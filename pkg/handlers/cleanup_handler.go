package handlers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/pkg/helpers"
	"github.com/riskified/dynamic-environment/pkg/names"
	"github.com/riskified/dynamic-environment/pkg/watches"
	istioapi "istio.io/api/networking/v1alpha3"
	istionetwork "istio.io/client-go/pkg/apis/networking/v1alpha3"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

// CleanupHandler is a handler that deals with removing DynamicEnv resources.
type CleanupHandler struct {
	client.Client
	Ctx context.Context
	Log logr.Logger
	DE  *riskifiedv1alpha1.DynamicEnv
}

type SubsetsAndConsumersMap = map[string]riskifiedv1alpha1.SubsetOrConsumer

// Checks for subsets and consumers that should be removed because of spec update.
func (ch *CleanupHandler) CheckForRemovedSubsetsAndConsumers(subsetsAndConsumers []riskifiedv1alpha1.SubsetType) SubsetsAndConsumersMap {
	var allSubsetsAndConsumers = make(map[string]riskifiedv1alpha1.SubsetOrConsumer)
	for _, st := range subsetsAndConsumers {
		subsetName := helpers.MKSubsetName(st.Subset)
		allSubsetsAndConsumers[subsetName] = st.Type
	}
	var keys []string
	for k := range allSubsetsAndConsumers {
		keys = append(keys, k)
	}
	var removed = make(map[string]riskifiedv1alpha1.SubsetOrConsumer)
	for name := range ch.DE.Status.ConsumersStatus {
		if !slices.Contains(keys, name) {
			removed[name] = riskifiedv1alpha1.CONSUMER
		}
	}
	for name := range ch.DE.Status.SubsetsStatus {
		if !slices.Contains(keys, name) {
			removed[name] = riskifiedv1alpha1.SUBSET
		}
	}
	if len(removed) > 0 {
		ch.Log.V(1).Info("Found subsets/consumers to be cleaned up", "subsets/consumers", removed)
	}
	return removed
}

// Removes the provided subsets and consumers. Returns whether there are deletions in progress and an error if occurred.
// You can use the output of CheckForRemovedSubsetsAndConsumers as an input.
func (ch *CleanupHandler) RemoveSubsetsAndConsumers(subsetsAndConsumers SubsetsAndConsumersMap) (inProgress bool, _ error) {
	for name, typ := range subsetsAndConsumers {
		if typ == riskifiedv1alpha1.CONSUMER {
			processing, err := ch.removeConsumer(name)
			if err != nil {
				ch.Log.Error(err, "delete removed consumer")
				return processing, fmt.Errorf("delete removed consumer: %w", err)
			}
			if processing {
				inProgress = true
			}
		} else {
			processing, err := ch.removeSubset(name)
			if err != nil {
				ch.Log.Error(err, "delete removed subset")
				return processing, fmt.Errorf("delete removed subset: %w", err)
			}
			if processing {
				inProgress = true
			}
		}
	}

	return inProgress, nil
}

// DeleteAllResources handles the removal of all resources created by the controller + the finalizers.
func (ch *CleanupHandler) DeleteAllResources() (ctrl.Result, error) {
	ch.Log.Info("Dynamic Env marked for deletion, cleaning up ...")
	if slices.Contains(ch.DE.Finalizers, names.DeleteDeployments) {
		count, err := ch.deleteAllDeployments()
		if err != nil {
			ch.Log.Error(err, "cleanup all resources")
			return ctrl.Result{}, err
		}
		if count == 0 {
			if err := ch.deleteFinalizer(names.DeleteDeployments); err != nil {
				ch.Log.Error(err, "removing DeleteDeployments finalizer")
				return ctrl.Result{}, err
			}
		}
	}
	if slices.Contains(ch.DE.Finalizers, names.DeleteDestinationRules) {
		count, err := ch.deleteAllDestinationRules()
		if err != nil {
			ch.Log.Error(err, "cleanup all resources")
			return ctrl.Result{}, err
		}
		if count == 0 {
			if err := ch.deleteFinalizer(names.DeleteDestinationRules); err != nil {
				ch.Log.Error(err, "removing DeleteDestinationRules finalizer")
				return ctrl.Result{}, err
			}
		}
	}
	if slices.Contains(ch.DE.Finalizers, names.CleanupVirtualServices) {
		if err := ch.cleanupVirtualServices(); err != nil {
			ch.Log.Error(err, "cleanup all resources")
			return ctrl.Result{}, err
		}
		if err := ch.deleteFinalizer(names.CleanupVirtualServices); err != nil {
			ch.Log.Error(err, "error removing CleanupVirtualServices finalizer")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (ch *CleanupHandler) deleteFinalizer(finalizer string) error {
	ch.Log.Info("Deleting finalizer from dynamic env", "finalizer", finalizer)
	remainingFinalizers := helpers.RemoveItemFromStringSlice(finalizer, ch.DE.Finalizers)
	ch.DE.Finalizers = remainingFinalizers
	if err := ch.Update(ch.Ctx, ch.DE); err != nil {
		return fmt.Errorf("error removing %s finalizer: %w", finalizer, err)
	}
	return nil
}

// Deletes all deployments created by this controller.
// Returns the number of identified resources that are not yet deleted (and error if happened).
func (ch *CleanupHandler) deleteAllDeployments() (int, error) {
	var deployments []riskifiedv1alpha1.ResourceStatus
	var runningCount int
	for _, s := range ch.DE.Status.SubsetsStatus {
		deployments = append(deployments, s.Deployment)
	}
	for _, c := range ch.DE.Status.ConsumersStatus {
		deployments = append(deployments, c.ResourceStatus)
	}
	for _, item := range deployments {
		found, err := ch.deleteDeployment(item)
		if err != nil {
			return runningCount, fmt.Errorf("deleting all deployments: %w", err)
		}
		if found {
			runningCount += 1
		}
	}
	return runningCount, nil
}

// Deletes all the destination-rules created by the controller.
// Returns the number of deletions in progress and error.
func (ch *CleanupHandler) deleteAllDestinationRules() (int, error) {
	var drs []riskifiedv1alpha1.ResourceStatus
	var runningCount int
	for _, s := range ch.DE.Status.SubsetsStatus {
		drs = append(drs, s.DestinationRules...)
	}
	for _, item := range drs {
		ch.Log.Info("Deleting destination rule ...", "destinationRule", item)
		found, err := ch.deleteDestinationRule(item)
		if err != nil {
			return runningCount, fmt.Errorf("deleting destination rule: %w", err)
		}
		if found {
			runningCount += 1
		}
	}
	return runningCount, nil
}

// Deletes all modified virtual services.
func (ch *CleanupHandler) cleanupVirtualServices() error {
	var vss []riskifiedv1alpha1.ResourceStatus
	for _, s := range ch.DE.Status.SubsetsStatus {
		vss = append(vss, s.VirtualServices...)
	}
	for _, item := range vss {
		if err := ch.cleanupVirtualService(item); err != nil {
			return fmt.Errorf("cleaning up all virtual services: %w", err)
		}
	}
	return nil
}

func (ch *CleanupHandler) cleanupVirtualService(vs riskifiedv1alpha1.ResourceStatus) error {
	version := helpers.UniqueDynamicEnvName(ch.DE)
	ch.Log.Info("Cleaning up Virtual Service ...", "virtual-service", vs)
	found := istionetwork.VirtualService{}
	if err := ch.Get(ch.Ctx, types.NamespacedName{Name: vs.Name, Namespace: vs.Namespace}, &found); err != nil {
		if errors.IsNotFound(err) {
			ch.Log.V(1).Info("Cleanup: Didn't find virtual service. Probably deleted", "virtual-service", vs)
			return nil
		} else {
			ch.Log.Error(err, "error searching for virtual service during cleanup", "virtual-service", vs)
			return err
		}
	}
	var newRoutes []*istioapi.HTTPRoute
	for _, route := range found.Spec.Http {
		if strings.HasPrefix(route.Name, helpers.CalculateVirtualServicePrefix(version, "")) {
			ch.Log.V(1).Info("Found route to cleanup", "route", route)
			continue
		}
		newRoutes = append(newRoutes, route)
	}
	found.Spec.Http = newRoutes
	watches.RemoveFromAnnotation(types.NamespacedName{Name: ch.DE.Name, Namespace: ch.DE.Namespace}, &found)
	if err := ch.Update(ch.Ctx, &found); err != nil {
		ch.Log.Error(err, "error updating virtual service after cleanup", "virtual-service", found.Name)
		return err
	}
	return nil
}

func (ch *CleanupHandler) removeConsumer(name string) (processing bool, err error) {
	st, ok := ch.DE.Status.ConsumersStatus[name]
	if ok {
		found, err := ch.deleteDeployment(st.ResourceStatus)
		if err != nil {
			return true, fmt.Errorf("remove consumer: %w", err)
		}
		if found {
			status := ch.DE.Status.ConsumersStatus[name]
			status.Status = riskifiedv1alpha1.Removing
			ch.DE.Status.ConsumersStatus[name] = status
			return true, ch.Status().Update(ch.Ctx, ch.DE)
		} else {
			delete(ch.DE.Status.ConsumersStatus, name)
			return false, ch.Status().Update(ch.Ctx, ch.DE)
		}
	}
	ch.Log.V(1).Info("Consumer removal finished", "consumer", name)
	return false, nil
}

func (ch *CleanupHandler) removeSubset(name string) (processing bool, _ error) {
	st, ok := ch.DE.Status.SubsetsStatus[name]
	exists := false
	if ok {
		found, err := ch.deleteDeployment(st.Deployment)
		if err != nil {
			return true, fmt.Errorf("removing subset: %w", err)
		}
		if found {
			exists = found
		}
		for _, dr := range st.DestinationRules {
			found, err := ch.deleteDestinationRule(dr)
			if err != nil {
				return true, fmt.Errorf("removing subset: %w", err)
			}
			if found {
				exists = found
			}
		}
		for _, vs := range st.VirtualServices {
			if err := ch.cleanupVirtualService(vs); err != nil {
				return true, fmt.Errorf("removing subset: %w", err)
			}
		}
	}
	if exists {
		status := ch.DE.Status.SubsetsStatus[name]
		status.Deployment.Status = riskifiedv1alpha1.Removing
		ch.DE.Status.SubsetsStatus[name] = status
		return true, ch.Status().Update(ch.Ctx, ch.DE)
	} else {
		delete(ch.DE.Status.SubsetsStatus, name)
		return false, ch.Status().Update(ch.Ctx, ch.DE)
	}
}

func (ch *CleanupHandler) deleteDeployment(deployment riskifiedv1alpha1.ResourceStatus) (found bool, _ error) {
	dep := appsv1.Deployment{}
	if err := ch.Get(ch.Ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, &dep); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("fetching deployment for deletion: %w", err)
	}
	if err := ch.Delete(ch.Ctx, &dep); err != nil {
		return true, fmt.Errorf("deleting deployment: %w", err)
	}
	return true, nil
}

func (ch *CleanupHandler) deleteDestinationRule(dr riskifiedv1alpha1.ResourceStatus) (found bool, _ error) {
	toDelete := istionetwork.DestinationRule{}
	if err := ch.Get(ch.Ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &toDelete); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("fetching destination rule for deletion: %w", err)
	}
	if err := ch.Delete(ch.Ctx, &toDelete); err != nil {
		return true, fmt.Errorf("deleting destination rule: %w", err)
	}
	return true, nil
}
