package model

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

// CleanupManager is a handler that deals with removing DynamicEnv resources.
type CleanupManager struct {
	client.Client
	Ctx context.Context
	Log logr.Logger
	DE  *riskifiedv1alpha1.DynamicEnv
}

type SubsetsAndConsumersMap = map[string]riskifiedv1alpha1.SubsetOrConsumer

// Checks for subsets and consumers that should be removed because of spec update.
func (cm *CleanupManager) CheckForRemovedSubsetsAndConsumers(subsetsAndConsumers []SubsetType) SubsetsAndConsumersMap {
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
	for name := range cm.DE.Status.ConsumersStatus {
		if !slices.Contains(keys, name) {
			removed[name] = riskifiedv1alpha1.CONSUMER
		}
	}
	for name := range cm.DE.Status.SubsetsStatus {
		if !slices.Contains(keys, name) {
			removed[name] = riskifiedv1alpha1.SUBSET
		}
	}
	if len(removed) > 0 {
		cm.Log.V(1).Info("Found subsets/consumers to be cleaned up", "subsets/consumers", removed)
	}
	return removed
}

// Removes the provided subsets and consumers. Returns whether there are deletions in progress and an error if occurred.
// You can use the output of CheckForRemovedSubsetsAndConsumers as an input.
func (cm *CleanupManager) RemoveSubsetsAndConsumers(subsetsAndConsumers SubsetsAndConsumersMap) (inProgress bool, _ error) {
	for name, typ := range subsetsAndConsumers {
		if typ == riskifiedv1alpha1.CONSUMER {
			processing, err := cm.removeConsumer(name)
			if err != nil {
				cm.Log.Error(err, "delete removed consumer")
				return processing, fmt.Errorf("delete removed consumer: %w", err)
			}
			if processing {
				inProgress = true
			}
		} else {
			processing, err := cm.removeSubset(name)
			if err != nil {
				cm.Log.Error(err, "delete removed subset")
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
func (cm *CleanupManager) DeleteAllResources() (ctrl.Result, error) {
	cm.Log.Info("Dynamic Env marked for deletion, cleaning up ...")
	if slices.Contains(cm.DE.Finalizers, names.DeleteDeployments) {
		count, err := cm.deleteAllDeployments()
		if err != nil {
			cm.Log.Error(err, "cleanup all resources")
			return ctrl.Result{}, err
		}
		if count == 0 {
			if err := cm.deleteFinalizer(names.DeleteDeployments); err != nil {
				cm.Log.Error(err, "removing DeleteDeployments finalizer")
				return ctrl.Result{}, err
			}
		}
	}
	if slices.Contains(cm.DE.Finalizers, names.DeleteDestinationRules) {
		count, err := cm.deleteAllDestinationRules()
		if err != nil {
			cm.Log.Error(err, "cleanup all resources")
			return ctrl.Result{}, err
		}
		if count == 0 {
			if err := cm.deleteFinalizer(names.DeleteDestinationRules); err != nil {
				cm.Log.Error(err, "removing DeleteDestinationRules finalizer")
				return ctrl.Result{}, err
			}
		}
	}
	if slices.Contains(cm.DE.Finalizers, names.CleanupVirtualServices) {
		if err := cm.cleanupVirtualServices(); err != nil {
			cm.Log.Error(err, "cleanup all resources")
			return ctrl.Result{}, err
		}
		if err := cm.deleteFinalizer(names.CleanupVirtualServices); err != nil {
			cm.Log.Error(err, "error removing CleanupVirtualServices finalizer")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (cm *CleanupManager) deleteFinalizer(finalizer string) error {
	cm.Log.Info("Deleting finalizer from dynamic env", "finalizer", finalizer)
	remainingFinalizers := helpers.RemoveItemFromStringSlice(finalizer, cm.DE.Finalizers)
	cm.DE.Finalizers = remainingFinalizers
	if err := cm.Update(cm.Ctx, cm.DE); err != nil {
		return fmt.Errorf("error removing %s finalizer: %w", finalizer, err)
	}
	return nil
}

// Deletes all deployments created by this controller.
// Returns the number of identified resources that are not yet deleted (and error if happened).
func (cm *CleanupManager) deleteAllDeployments() (int, error) {
	var deployments []riskifiedv1alpha1.ResourceStatus
	var runningCount int
	for _, s := range cm.DE.Status.SubsetsStatus {
		deployments = append(deployments, s.Deployment)
	}
	for _, c := range cm.DE.Status.ConsumersStatus {
		deployments = append(deployments, c.ResourceStatus)
	}
	for _, item := range deployments {
		found, err := cm.deleteDeployment(item)
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
func (cm *CleanupManager) deleteAllDestinationRules() (int, error) {
	var drs []riskifiedv1alpha1.ResourceStatus
	var runningCount int
	for _, s := range cm.DE.Status.SubsetsStatus {
		drs = append(drs, s.DestinationRules...)
	}
	for _, item := range drs {
		cm.Log.Info("Deleting destination rule ...", "destinationRule", item)
		found, err := cm.deleteDestinationRule(item)
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
func (cm *CleanupManager) cleanupVirtualServices() error {
	var vss []riskifiedv1alpha1.ResourceStatus
	for _, s := range cm.DE.Status.SubsetsStatus {
		vss = append(vss, s.VirtualServices...)
	}
	for _, item := range vss {
		if err := cm.cleanupVirtualService(item); err != nil {
			return fmt.Errorf("cleaning up all virtual services: %w", err)
		}
	}
	return nil
}

func (cm *CleanupManager) cleanupVirtualService(vs riskifiedv1alpha1.ResourceStatus) error {
	version := helpers.UniqueDynamicEnvName(types.NamespacedName{Name: cm.DE.Name, Namespace: cm.DE.Namespace})
	cm.Log.Info("Cleaning up Virtual Service ...", "virtual-service", vs)
	found := istionetwork.VirtualService{}
	if err := cm.Get(cm.Ctx, types.NamespacedName{Name: vs.Name, Namespace: vs.Namespace}, &found); err != nil {
		if errors.IsNotFound(err) {
			cm.Log.V(1).Info("Cleanup: Didn't find virtual service. Probably deleted", "virtual-service", vs)
			return nil
		} else {
			cm.Log.Error(err, "error searching for virtual service during cleanup", "virtual-service", vs)
			return err
		}
	}
	var newRoutes []*istioapi.HTTPRoute
	for _, route := range found.Spec.Http {
		if strings.HasPrefix(route.Name, helpers.CalculateVirtualServicePrefix(version, "")) {
			cm.Log.V(1).Info("Found route to cleanup", "route", route)
			continue
		}
		newRoutes = append(newRoutes, route)
	}
	found.Spec.Http = newRoutes
	watches.RemoveFromAnnotation(types.NamespacedName{Name: cm.DE.Name, Namespace: cm.DE.Namespace}, &found)
	if err := cm.Update(cm.Ctx, &found); err != nil {
		cm.Log.Error(err, "error updating virtual service after cleanup", "virtual-service", found.Name)
		return err
	}
	return nil
}

func (cm *CleanupManager) removeConsumer(name string) (processing bool, err error) {
	st, ok := cm.DE.Status.ConsumersStatus[name]
	if ok {
		found, err := cm.deleteDeployment(st.ResourceStatus)
		if err != nil {
			return true, fmt.Errorf("remove consumer: %w", err)
		}
		if found {
			status := cm.DE.Status.ConsumersStatus[name]
			status.Status = riskifiedv1alpha1.Removing
			cm.DE.Status.ConsumersStatus[name] = status
			return true, cm.Status().Update(cm.Ctx, cm.DE)
		} else {
			delete(cm.DE.Status.ConsumersStatus, name)
			return false, cm.Status().Update(cm.Ctx, cm.DE)
		}
	}
	cm.Log.V(1).Info("Consumer removal finished", "consumer", name)
	return false, nil
}

func (cm *CleanupManager) removeSubset(name string) (processing bool, _ error) {
	st, ok := cm.DE.Status.SubsetsStatus[name]
	exists := false
	if ok {
		found, err := cm.deleteDeployment(st.Deployment)
		if err != nil {
			return true, fmt.Errorf("removing subset: %w", err)
		}
		if found {
			exists = found
		}
		for _, dr := range st.DestinationRules {
			found, err := cm.deleteDestinationRule(dr)
			if err != nil {
				return true, fmt.Errorf("removing subset: %w", err)
			}
			if found {
				exists = found
			}
		}
		for _, vs := range st.VirtualServices {
			if err := cm.cleanupVirtualService(vs); err != nil {
				return true, fmt.Errorf("removing subset: %w", err)
			}
		}
	}
	if exists {
		status := cm.DE.Status.SubsetsStatus[name]
		status.Deployment.Status = riskifiedv1alpha1.Removing
		cm.DE.Status.SubsetsStatus[name] = status
		return true, cm.Status().Update(cm.Ctx, cm.DE)
	} else {
		delete(cm.DE.Status.SubsetsStatus, name)
		return false, cm.Status().Update(cm.Ctx, cm.DE)
	}
}

func (cm *CleanupManager) deleteDeployment(deployment riskifiedv1alpha1.ResourceStatus) (found bool, _ error) {
	dep := appsv1.Deployment{}
	if err := cm.Get(cm.Ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, &dep); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("fetching deployment for deletion: %w", err)
	}
	if err := cm.Delete(cm.Ctx, &dep); err != nil {
		return true, fmt.Errorf("deleting deployment: %w", err)
	}
	return true, nil
}

func (cm *CleanupManager) deleteDestinationRule(dr riskifiedv1alpha1.ResourceStatus) (found bool, _ error) {
	toDelete := istionetwork.DestinationRule{}
	if err := cm.Get(cm.Ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &toDelete); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("fetching destination rule for deletion: %w", err)
	}
	if err := cm.Delete(cm.Ctx, &toDelete); err != nil {
		return true, fmt.Errorf("deleting destination rule: %w", err)
	}
	return true, nil
}
