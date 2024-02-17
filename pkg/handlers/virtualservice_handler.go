/*
Copyright 2023 Riskified Ltd

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handlers

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/pkg/helpers"
	"github.com/riskified/dynamic-environment/pkg/model"
	"github.com/riskified/dynamic-environment/pkg/watches"
	istioapi "istio.io/api/networking/v1alpha3"
	istionetwork "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// A handler for managing VirtualService manipulations.
type VirtualServiceHandler struct {
	client.Client
	Identifier    types.NamespacedName
	UniqueName    string
	UniqueVersion string
	Namespace     string
	// The name of the subset/consumer as it appears in the Status map
	SubsetName     string
	RoutePrefix    string
	ServiceHosts   []string
	DefaultVersion string
	Matches        []riskifiedv1alpha1.IstioMatch
	StatusManager  *model.StatusManager
	Log            logr.Logger

	activeHosts     []string
	virtualServices map[string][]types.NamespacedName
}

// Initializes VirtualServiceHandler with provided and default values
func NewVirtualServiceHandler(
	subsetData model.DynamicEnvReconcileData,
	serviceHosts []string,
	defaultVersion string,
	client client.Client,
) *VirtualServiceHandler {
	uniqueVersion := helpers.UniqueDynamicEnvName(subsetData.Identifier)
	uniqueName := helpers.MkSubsetUniqueName(subsetData.Subset.Name, uniqueVersion)
	return &VirtualServiceHandler{
		Client:         client,
		Identifier:     subsetData.Identifier,
		UniqueName:     uniqueName,
		UniqueVersion:  uniqueVersion,
		Namespace:      subsetData.Subset.Namespace,
		SubsetName:     helpers.MKSubsetName(subsetData.Subset),
		RoutePrefix:    helpers.CalculateVirtualServicePrefix(uniqueVersion, subsetData.Subset.Name),
		ServiceHosts:   serviceHosts,
		DefaultVersion: defaultVersion,
		Matches:        subsetData.Matches,
		StatusManager:  subsetData.StatusManager,
		Log:            helpers.MkLogger("VirtualServiceHandler", "resource", helpers.MkResourceName(subsetData.Identifier)),
	}
}

// Fetches related Virtual Services and manipulates them accordingly.
func (h *VirtualServiceHandler) Handle(ctx context.Context) error {
	h.virtualServices = make(map[string][]types.NamespacedName)
	for _, serviceHost := range h.ServiceHosts {
		services, err := h.locateVirtualServicesByServiceHost(ctx, serviceHost)
		if err != nil {
			return err
		}
		h.virtualServices[serviceHost] = toNamespacedName(services)
		var atLeastOneVirtualServicePerHostnameExists = false
		for _, service := range services {
			if err := h.updateVirtualService(ctx, service, serviceHost); err != nil {
				h.Log.Info("error updating virtual service", "service-host", serviceHost, "error", err.Error())
				continue
			}
			atLeastOneVirtualServicePerHostnameExists = true
		}
		if atLeastOneVirtualServicePerHostnameExists {
			h.activeHosts = append(h.activeHosts, serviceHost)
		}
	}

	if len(h.activeHosts) == 0 {
		return fmt.Errorf("could not find even one virtual service that handles subset %q", h.GetSubset())
	}
	return nil
}

func (h *VirtualServiceHandler) GetStatus(ctx context.Context) ([]riskifiedv1alpha1.ResourceStatus, error) {
	var services []*istionetwork.VirtualService
	var statuses []riskifiedv1alpha1.ResourceStatus
	for _, serviceHost := range h.ServiceHosts {
		s, ok := h.virtualServices[serviceHost]
		if ok {
			for _, nn := range s {
				found := &istionetwork.VirtualService{}
				if err := h.Get(ctx, nn, found); err != nil {
					h.Log.Error(err, "Fetching virtual service for status", "name", nn)
					return nil, fmt.Errorf("fetching virtual service for status: %w", err)
				}
				services = append(services, found)
			}
		}
	}
	for _, service := range services {
		s := h.getStatusForService(service)
		statuses = append(statuses, s)
	}

	return statuses, nil
}

func (h *VirtualServiceHandler) ApplyStatus(statuses []riskifiedv1alpha1.ResourceStatus) error {
	for _, rs := range statuses {
		if err := h.StatusManager.AddVirtualServiceStatusEntry(h.SubsetName, rs); err != nil {
			return err
		}
	}
	return nil
}

func (h *VirtualServiceHandler) GetSubset() string {
	return h.SubsetName
}

func (h *VirtualServiceHandler) GetHosts() []string {
	return h.activeHosts
}

func (h *VirtualServiceHandler) locateVirtualServicesByServiceHost(ctx context.Context, serviceHost string) ([]*istionetwork.VirtualService, error) {
	virtualServices := istionetwork.VirtualServiceList{}
	var result []*istionetwork.VirtualService
	if err := h.List(ctx, &virtualServices); err != nil {
		return nil, fmt.Errorf("error locating virtual services for app host '%s': %w", h.ServiceHosts, err)
	}
	for idx, service := range virtualServices.Items {
		for _, host := range service.Spec.Hosts {
			if helpers.MatchNamespacedHost(serviceHost, h.Namespace, host, service.Namespace) {
				useSelf, delegated := h.resolveVirtualServices(serviceHost, service)
				if useSelf {
					result = append(result, virtualServices.Items[idx])
				}
				for _, d := range delegated {
					found, err := h.extractServiceFromDelegate(ctx, d, virtualServices)
					if err != nil {
						return result, err
					}
					if found != nil {
						result = append(result, found)
					}
				}
			}
		}
	}
	h.Log.Info(fmt.Sprintf("Found %d virtual services matching '%s'", len(result), h.ServiceHosts))
	return result, nil
}

// Parses the provided virtual service and return a list of delegates (if exists) and itself if
// contains non-delegate HTTPRoutes.
func (h *VirtualServiceHandler) resolveVirtualServices(serviceHost string, service *istionetwork.VirtualService) (
	useSelf bool, delegated []*istioapi.Delegate,
) {
	for _, r := range service.Spec.Http {
		if r.Delegate != nil {
			delegated = append(delegated, r.Delegate)
		}
		if h.routeDestinationsMatchesServiceHost(serviceHost, r.Route, service.Namespace) {
			useSelf = true
		}
	}
	return useSelf, delegated
}

func (h *VirtualServiceHandler) extractServiceFromDelegate(ctx context.Context, delegate *istioapi.Delegate, services istionetwork.VirtualServiceList) (*istionetwork.VirtualService, error) {
	for _, s := range services.Items {
		if delegate.Name == s.Name && delegate.Namespace == s.Namespace {
			return s, nil
		}
	}
	msg := fmt.Sprintf("Wierd, Couldn't find a service with name: %s, namespace: %s, in the service list", delegate.Name, delegate.Namespace)
	if err := h.StatusManager.AddGlobalVirtualServiceError(h.SubsetName, msg); err != nil {
		h.Log.Error(err, "failed to write the following message to status: "+msg)
	}
	h.Log.Info(msg)
	s := &istionetwork.VirtualService{}
	searchName := types.NamespacedName{Name: delegate.Name, Namespace: delegate.Namespace}
	if err := h.Client.Get(ctx, searchName, s); err != nil {
		if errors.IsNotFound(err) {
			msg := fmt.Sprintf("Delegate (%s/%s) not found", delegate.Namespace, delegate.Name)
			h.Log.V(0).Info(msg)
			if err = h.StatusManager.AddGlobalVirtualServiceError(h.SubsetName, msg); err != nil {
				h.Log.Error(err, "Error writing virtual service status regarding delegate", "delegate", delegate)
				if !errors.IsConflict(err) {
					return nil, fmt.Errorf("failed to write global virtual service error: %w", err)
				}
			}
			return nil, nil
		} else {
			return nil, fmt.Errorf("error while loading delegate '%s/%s': %w", delegate.Namespace, delegate.Name, err)
		}
	}
	return s, nil
}

func (h *VirtualServiceHandler) updateVirtualService(ctx context.Context, service *istionetwork.VirtualService, serviceHost string) error {
	h.Log.Info("Updating virtual service to route to version", "virtual-service", service.Name, "version", h.UniqueVersion)
	prefix := h.RoutePrefix

	// TODO: This is currently a shortcoming. We'll see whether there's a need to enable that (a single virtual service
	//       that serves multiple service hosts).
	if routesConfiguredWithDynamicEnvRoutes(service.Spec.Http, prefix) {
		h.Log.Info("Skipping virtual service that already contains dynamic-environment routes", "virtual-service", service.Name)
		return nil
	}

	newStatus := riskifiedv1alpha1.ResourceStatus{Name: service.Name, Namespace: service.Namespace}
	if err := h.StatusManager.AddVirtualServiceStatusEntry(h.SubsetName, newStatus); err != nil {
		h.Log.Error(err, "error updating state for virtual service")
		return err
	}

	var newRoutes []*istioapi.HTTPRoute
	for _, r := range service.Spec.Http {
		if strings.HasPrefix(r.Name, prefix) {
			// We never use our old definitions, we're always acting from scratch
			continue
		}
		if h.routeDestinationsMatchesServiceHost(serviceHost, r.Route, service.Namespace) {
			for _, match := range h.Matches {
				ourSubsetRoute := r.DeepCopy()
				if err := h.populateRouteWithSubsetData(ourSubsetRoute, serviceHost, match, service.Namespace); err != nil {
					h.Log.Error(err, "Adopting rule for dynamic environment")
					return err
				}
				newRoutes = append(newRoutes, ourSubsetRoute)
			}
		}
		newRoutes = append(newRoutes, r)
	}

	service.Spec.Http = newRoutes
	watches.AddToAnnotation(h.Identifier, service)
	if err := h.Update(ctx, service); err != nil {
		h.Log.Error(err, "Error updating VirtualService with our updated rules")
		return err
	}

	return nil
}

func (h *VirtualServiceHandler) getStatusForService(vs *istionetwork.VirtualService) riskifiedv1alpha1.ResourceStatus {
	genStatus := func(s riskifiedv1alpha1.LifeCycleStatus) riskifiedv1alpha1.ResourceStatus {
		return riskifiedv1alpha1.ResourceStatus{
			Name:      vs.Name,
			Namespace: vs.Namespace,
			Status:    s,
		}
	}

	if routesConfiguredWithDynamicEnvRoutes(vs.Spec.Http, h.RoutePrefix) {
		return genStatus(riskifiedv1alpha1.Running)
	} else {
		return genStatus(riskifiedv1alpha1.IgnoredMissingVS)
	}
}

// Populates the provided HTTPRoute with provided (and configured) data.
// Note: the `inNamespace` parameter is the namespace to search for if the destination is not fully
// qualified (e.g.the namespace which contains the virtual service we're updating).
func (h *VirtualServiceHandler) populateRouteWithSubsetData(route *istioapi.HTTPRoute, serviceHost string, match riskifiedv1alpha1.IstioMatch, inNamespace string) error {
	newRouteDestinations := h.filterRouteDestinationByHost(route.Route, serviceHost, inNamespace)
	if len(newRouteDestinations) == 0 {
		return IgnoredMissing{}
	}
	for _, rd := range newRouteDestinations {
		rd.Destination.Subset = h.UniqueVersion
		rd.Weight = 0
		if rd.Headers == nil {
			rd.Headers = &istioapi.Headers{}
		}
		safeApplyResponseHeaders(rd.Headers, "x-dynamic-env", h.UniqueName)
	}
	route.Route = newRouteDestinations

	if len(route.Match) == 0 {
		route.Match = []*istioapi.HTTPMatchRequest{{}}
	}
	for _, m := range route.Match {
		if len(match.Headers) > 0 && m.Headers == nil {
			m.Headers = make(map[string]*istioapi.StringMatch)
		}
		if len(match.SourceLabels) > 0 && m.SourceLabels == nil {
			m.SourceLabels = make(map[string]string)
		}
		for k, v := range match.Headers {
			if v.Exact != "" {
				m.Headers[k] = &istioapi.StringMatch{MatchType: &istioapi.StringMatch_Exact{Exact: v.Exact}}
			} else if v.Prefix != "" {
				m.Headers[k] = &istioapi.StringMatch{MatchType: &istioapi.StringMatch_Prefix{Prefix: v.Prefix}}
			} else {
				m.Headers[k] = &istioapi.StringMatch{MatchType: &istioapi.StringMatch_Regex{Regex: v.Regex}}
			}
		}
		for k, v := range match.SourceLabels {
			m.SourceLabels[k] = v
		}
	}
	newRuleHash := helpers.AsSha256(route)
	route.Name = fmt.Sprintf("%s-%s", h.RoutePrefix, helpers.Shorten(newRuleHash, 10))

	return nil
}

// A helper to check whether any of the provided routes are already populated with DynamicEnv data.
func routesConfiguredWithDynamicEnvRoutes(routes []*istioapi.HTTPRoute, prefix string) bool {
	for _, r := range routes {
		if strings.HasPrefix(r.Name, prefix) {
			return true
		}
	}
	return false
}

// A helper to check if any of the provided route-destinations has a serviceHost and namespace matching the provided
// ones. The `inNamespace` parameter is the namespace to search if the `host` parameter is not fully qualified (e.g., the
// namespace of the virtual service).
func (h *VirtualServiceHandler) routeDestinationsMatchesServiceHost(serviceHost string, routes []*istioapi.HTTPRouteDestination, inNamespace string) bool {
	for _, route := range routes {
		dest := route.Destination
		if helpers.MatchNamespacedHost(serviceHost, h.Namespace, dest.Host, inNamespace) && dest.Subset == h.DefaultVersion {
			return true
		}
	}
	return false
}

// Select only the route-destinations that match the provided hostname and namespace.
// Note: the `inNamespace` parameter is the namespace to search for if the destination is not fully qualified (e.g.the
// namespace which contains the virtual service we're processing).
func (h *VirtualServiceHandler) filterRouteDestinationByHost(destinations []*istioapi.HTTPRouteDestination, serviceHost string, inNamespace string) []*istioapi.HTTPRouteDestination {
	var newDestinations []*istioapi.HTTPRouteDestination

	for _, d := range destinations {
		if helpers.MatchNamespacedHost(serviceHost, h.Namespace, d.Destination.Host, inNamespace) && d.Destination.Subset == h.DefaultVersion {
			newDestinations = append(newDestinations, d)
		}
	}

	return newDestinations
}

func safeApplyResponseHeaders(headers *istioapi.Headers, key, value string) {
	if headers.Response == nil {
		headers.Response = &istioapi.Headers_HeaderOperations{}
	}
	currentResponseSet := headers.Response.GetAdd()
	if currentResponseSet == nil {
		currentResponseSet = make(map[string]string)
	}
	currentResponseSet[key] = value
	headers.Response.Add = currentResponseSet
}

func toNamespacedName(services []*istionetwork.VirtualService) []types.NamespacedName {
	var nns []types.NamespacedName
	for _, s := range services {
		nns = append(nns, types.NamespacedName{Name: s.Name, Namespace: s.Namespace})
	}
	return nns
}
