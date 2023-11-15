package handlers

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/pkg/helpers"
	"github.com/riskified/dynamic-environment/pkg/watches"
	istioapi "istio.io/api/networking/v1alpha3"
	istionetwork "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// A handler for managing VirtualService manipulations.
type VirtualServiceHandler struct {
	client.Client
	UniqueName     string
	UniqueVersion  string
	Namespace      string
	RoutePrefix    string
	ServiceHosts   []string
	DefaultVersion string
	DynamicEnv     *riskifiedv1alpha1.DynamicEnv
	StatusHandler  *DynamicEnvStatusHandler
	Log            logr.Logger
	Ctx            context.Context

	activeHosts []string
}

// Fetches related Virtual Services and manipulates them accordingly.
func (h *VirtualServiceHandler) Handle() error {
	for _, serviceHost := range h.ServiceHosts {
		services, err := h.locateVirtualServicesByServiceHost(serviceHost)
		if err != nil {
			return err
		}
		var atLeastOneVSPerHostnameExists = false
		for _, service := range services {
			if err := h.updateVirtualService(serviceHost, service); err != nil {
				h.Log.Info("error updating virtual service", "service-host", serviceHost, "error", err.Error())
				continue
			}
			atLeastOneVSPerHostnameExists = true
		}
		if atLeastOneVSPerHostnameExists {
			h.activeHosts = append(h.activeHosts, serviceHost)
		}
	}

	if len(h.activeHosts) == 0 {
		return fmt.Errorf("could not find even one virtual service that handles subset %q", h.GetSubset())
	}
	return nil
}

func (h *VirtualServiceHandler) GetStatus() ([]riskifiedv1alpha1.ResourceStatus, error) {
	var services []*istionetwork.VirtualService
	var statuses []riskifiedv1alpha1.ResourceStatus
	for _, serviceHost := range h.ServiceHosts {
		result, err := h.locateVirtualServicesByServiceHost(serviceHost)
		if err != nil {
			h.Log.Error(err, "While running GetStatus in VirtualServiceHandler")
			return []riskifiedv1alpha1.ResourceStatus{}, err
		}
		services = append(services, result...)
	}
	for _, service := range services {
		s := h.getStatusForService(service)
		statuses = append(statuses, s)
	}

	return statuses, nil
}

func (h *VirtualServiceHandler) ApplyStatus(statuses []riskifiedv1alpha1.ResourceStatus) error {
	for _, rs := range statuses {
		if err := h.StatusHandler.AddVirtualServiceStatusEntry(h.UniqueName, rs); err != nil {
			return err
		}
	}
	return nil
}

func (h *VirtualServiceHandler) GetSubset() string {
	return h.UniqueName
}

func (h *VirtualServiceHandler) GetHosts() []string {
	return h.activeHosts
}

func (h *VirtualServiceHandler) locateVirtualServicesByServiceHost(serviceHost string) ([]*istionetwork.VirtualService, error) {
	virtualServices := istionetwork.VirtualServiceList{}
	var result []*istionetwork.VirtualService
	if err := h.List(h.Ctx, &virtualServices); err != nil {
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
					found, err := h.extractServiceFromDelegate(d, virtualServices)
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
		if h.hasMatchingHostAndSubset(serviceHost, r.Route, service.Namespace) {
			useSelf = true
		}
	}
	return useSelf, delegated
}

func (h *VirtualServiceHandler) extractServiceFromDelegate(delegate *istioapi.Delegate, services istionetwork.VirtualServiceList) (*istionetwork.VirtualService, error) {
	for _, s := range services.Items {
		if delegate.Name == s.Name && delegate.Namespace == s.Namespace {
			return s, nil
		}
	}
	msg := fmt.Sprintf("Wierd, Couldn't find a service with name: %s, namespace: %s, in the service list", delegate.Name, delegate.Namespace)
	if err := h.StatusHandler.AddGlobalVirtualServiceError(h.UniqueName, msg); err != nil {
		h.Log.Error(err, "failed to write the following message to status: "+msg)
	}
	h.Log.Info(msg)
	s := &istionetwork.VirtualService{}
	searchName := types.NamespacedName{Name: delegate.Name, Namespace: delegate.Namespace}
	if err := h.Client.Get(h.Ctx, searchName, s); err != nil {
		if errors.IsNotFound(err) {
			msg := fmt.Sprintf("Delegate (%s/%s) not found", delegate.Namespace, delegate.Name)
			h.Log.V(0).Info(msg)
			if err = h.StatusHandler.AddGlobalVirtualServiceError(h.UniqueName, msg); err != nil {
				h.Log.Error(err, "Error writing virtual service status regarding delegate", "delegate", delegate)
			}
			return nil, nil
		} else {
			return nil, fmt.Errorf("error while loading delegate '%s/%s': %w", delegate.Namespace, delegate.Name, err)
		}
	}
	return s, nil
}

func (h *VirtualServiceHandler) updateVirtualService(serviceHost string, service *istionetwork.VirtualService) error {
	h.Log.Info("Updating virtual service to route to version", "virtual-service", service.Name, "version", h.UniqueVersion)
	owner := types.NamespacedName{Name: h.DynamicEnv.Name, Namespace: h.DynamicEnv.Namespace}
	prefix := h.RoutePrefix

	// TODO: Might not be relevant in multiple services
	if containsDynamicEnvRoutes(service.Spec.Http, prefix) {
		h.Log.Info("Skipping virtual service that already contains dynamic-environment routes", "virtual-service", service.Name)
		return nil
	}

	// Add this service to status
	newStatus := riskifiedv1alpha1.ResourceStatus{Name: service.Name, Namespace: service.Namespace}
	if err := h.StatusHandler.AddVirtualServiceStatusEntry(h.UniqueName, newStatus); err != nil {
		h.Log.Error(err, "error updating state for virtual service")
		return err
	}

	var newRoutes []*istioapi.HTTPRoute
	for _, r := range service.Spec.Http {
		if strings.HasPrefix(r.Name, prefix) {
			// We never use our old definitions, we're always acting from scratch
			// TODO: Not relevant in multiple services
			continue
		}
		if h.hasMatchingHostAndSubset(serviceHost, r.Route, service.Namespace) {
			for _, match := range h.DynamicEnv.Spec.IstioMatches {
				ourSubsetRoute := r.DeepCopy()
				if err := h.updateRouteForSubset(serviceHost, ourSubsetRoute, match, service.Namespace); err != nil {
					h.Log.Error(err, "Adopting rule for dynamic environment")
					return err
				}
				newRoutes = append(newRoutes, ourSubsetRoute)
			}
		}
		newRoutes = append(newRoutes, r)
	}

	service.Spec.Http = newRoutes
	watches.AddToAnnotation(owner, service)
	if err := h.Update(h.Ctx, service); err != nil {
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

	// TODO: is this even possible? we validate existing routes when selecting virtual services...
	if containsDynamicEnvRoutes(vs.Spec.Http, h.RoutePrefix) {
		return genStatus(riskifiedv1alpha1.Running)
	} else {
		return genStatus(riskifiedv1alpha1.IgnoredMissingVS)
	}
}

// Note: the `inNamespace` parameter is the namespace to search for if the destination is not fully
// qualified (e.g.the namespace which contains the virtual service we're updating).
func (h *VirtualServiceHandler) updateRouteForSubset(serviceHost string, route *istioapi.HTTPRoute, match riskifiedv1alpha1.IstioMatch, inNamespace string) error {
	newDestinations := h.filterDestinationByHostAndSubset(serviceHost, route.Route, inNamespace)
	if len(newDestinations) == 0 {
		return IgnoredMissing{}
	}
	for _, d := range newDestinations {
		d.Destination.Subset = h.UniqueVersion
		d.Weight = 0
		if d.Headers == nil {
			d.Headers = &istioapi.Headers{}
		}
		safeApplyResponseHeaders(d.Headers, "x-dynamic-env", h.UniqueName)
	}
	route.Route = newDestinations

	if len(route.Match) == 0 {
		// if we're handling default route than match is empty, however we need at
		// least one match for merging out rules into
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

func containsDynamicEnvRoutes(routes []*istioapi.HTTPRoute, prefix string) bool {
	for _, r := range routes {
		if strings.HasPrefix(r.Name, prefix) {
			return true
		}
	}
	return false
}

// A helper to check if any of the provided routes has the serviceHost and namespace configured in
// our handler. The `inNamespace` parameter is the namespace to search if the `host` parameter is
// not fully qualified (e.g. the namespace of the virtual service).
func (h *VirtualServiceHandler) hasMatchingHostAndSubset(serviceHost string, routes []*istioapi.HTTPRouteDestination, inNamespace string) bool {
	for _, route := range routes {
		dest := route.Destination
		if helpers.MatchNamespacedHost(serviceHost, h.Namespace, dest.Host, inNamespace) && dest.Subset == h.DefaultVersion {
			return true
		}
	}
	return false
}

// Note: the `inNamespace` parameter is the namespace to search for if the destination is not fully
// qualified (e.g.the namespace which contains the virtual service we're processing).
func (h *VirtualServiceHandler) filterDestinationByHostAndSubset(serviceHost string, destinations []*istioapi.HTTPRouteDestination, inNamespace string) []*istioapi.HTTPRouteDestination {
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
