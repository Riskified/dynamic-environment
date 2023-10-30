/*
Copyright 2021.

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

package controllers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	istioapi "istio.io/api/networking/v1alpha3"
	istionetwork "istio.io/client-go/pkg/apis/networking/v1alpha3"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/pkg/handlers"
	"github.com/riskified/dynamic-environment/pkg/helpers"
	"github.com/riskified/dynamic-environment/pkg/names"
	"github.com/riskified/dynamic-environment/pkg/watches"
)

// DynamicEnvReconciler reconciles a DynamicEnv object
type DynamicEnvReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	VersionLabel   string
	DefaultVersion string
	LabelsToRemove []string
}

type ReconcileLoopStatus struct {
	returnError      error
	subsetMessages   map[string]riskifiedv1alpha1.SubsetMessages
	consumerMessages map[string][]string
	// Non ready consumers and subsets
	nonReadyCS map[string]bool
}

type SubsetType struct {
	Type   riskifiedv1alpha1.SubsetOrConsumer
	Subset riskifiedv1alpha1.Subset
}

func (rls *ReconcileLoopStatus) setErrorIfNotMasking(err error) {
	if rls.returnError == nil {
		rls.returnError = err
	}
}

func (rls *ReconcileLoopStatus) addDeploymentMessage(subset string, tpe riskifiedv1alpha1.SubsetOrConsumer, format string, a ...interface{}) {
	if tpe == riskifiedv1alpha1.CONSUMER {
		msg := fmt.Sprintf(format, a...)
		rls.consumerMessages[subset] = append(rls.consumerMessages[subset], msg)
	} else {
		rls.subsetMessages[subset] = rls.subsetMessages[subset].AppendDeploymentMsg(format, a...)
	}
}

//+kubebuilder:rbac:groups=riskified.com,resources=dynamicenvs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=riskified.com,resources=dynamicenvs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=riskified.com,resources=dynamicenvs/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=networking.istio.io,resources=*,verbs=*
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch
// TODO: shrink istio permissions if possible.

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DynamicEnv object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *DynamicEnvReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	log.V(1).Info("Entered Reconcile Loop")
	rls := ReconcileLoopStatus{
		subsetMessages:   make(map[string]riskifiedv1alpha1.SubsetMessages),
		consumerMessages: make(map[string][]string),
		nonReadyCS:       make(map[string]bool),
	}

	dynamicEnv := &riskifiedv1alpha1.DynamicEnv{}
	err := r.Client.Get(ctx, req.NamespacedName, dynamicEnv)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("DynamicEnv resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch DynamicEnv")
		return ctrl.Result{}, fmt.Errorf("failed to fetch dynamic environment: %w", err)
	}

	uniqueVersion := helpers.UniqueDynamicEnvName(dynamicEnv.Name, dynamicEnv.Namespace)

	if markedForDeletion(dynamicEnv) {
		return r.cleanDynamicEnvResources(ctx, dynamicEnv, uniqueVersion)
	}

	if err := r.addFinalizersIfRequired(ctx, dynamicEnv); err != nil {
		log.Error(err, "Error adding finalizers")
		return ctrl.Result{}, err
	}

	owner := types.NamespacedName{Name: dynamicEnv.Name, Namespace: dynamicEnv.Namespace}
	var deploymentHandlers []handlers.SRHandler
	var mrHandlers []handlers.MRHandler
	nonReadyExists := false
	degradedExists := false

	statusHandler := handlers.DynamicEnvStatusHandler{
		Client:     r.Client,
		Ctx:        ctx,
		DynamicEnv: dynamicEnv,
	}

	subsetsAndConsumers := mergeSubsetsAndConsumers(dynamicEnv.Spec.Subsets, dynamicEnv.Spec.Consumers)

	for _, st := range subsetsAndConsumers {
		s := st.Subset
		uniqueName := s.Name + "-" + uniqueVersion
		defaultVersionForSubset := r.DefaultVersion
		if s.DefaultVersion != "" {
			defaultVersionForSubset = s.DefaultVersion
		}

		baseDeployment := &appsv1.Deployment{}
		err := r.Get(ctx, types.NamespacedName{Name: s.Name, Namespace: s.Namespace}, baseDeployment)
		if err != nil {
			log.Error(err, "couldn't find the deployment we need to override", "deployment-name", s.Name, "namespace", s.Namespace)
			msg := fmt.Sprintf("couldn't find the deployment we need to override (name: %s, ns: %s)", s.Name, s.Namespace)
			rls.returnError = fmt.Errorf("%s: %w", msg, err)
			rls.addDeploymentMessage(uniqueName, st.Type, "%s, %v", msg, err)
			rls.nonReadyCS[uniqueName] = true
			break
		}

		deploymentHandler := handlers.DeploymentHandler{
			Client:         r.Client,
			UniqueName:     uniqueName,
			UniqueVersion:  uniqueVersion,
			Owner:          owner,
			BaseDeployment: baseDeployment,
			DeploymentType: st.Type,
			LabelsToRemove: r.LabelsToRemove,
			VersionLabel:   r.VersionLabel,
			StatusHandler:  &statusHandler,
			Matches:        dynamicEnv.Spec.IstioMatches,
			Subset:         s,
			Log:            log,
			Ctx:            ctx,
		}
		deploymentHandlers = append(deploymentHandlers, &deploymentHandler)
		if err := deploymentHandler.Handle(); err != nil {
			rls.returnError = err
			rls.addDeploymentMessage(uniqueName, st.Type, err.Error())
			break
		}

		if st.Type == riskifiedv1alpha1.SUBSET {
			serviceHosts, err := r.locateMatchingServiceHostnames(ctx, s.Namespace, baseDeployment.Spec.Template.ObjectMeta.Labels)
			if err != nil {
				msg := fmt.Sprintf("locating service hostname for deployment '%s'", baseDeployment.Name)
				rls.returnError = fmt.Errorf("%s: %w", msg, err)
				rls.subsetMessages[uniqueName] = rls.subsetMessages[uniqueName].AppendGlobalMsg("%s: %v", msg, err)
				break
			}

			destinationRuleHandler := handlers.DestinationRuleHandler{
				Client:         r.Client,
				UniqueName:     uniqueName,
				UniqueVersion:  uniqueVersion,
				Namespace:      s.Namespace,
				VersionLabel:   r.VersionLabel,
				DefaultVersion: defaultVersionForSubset,
				StatusHandler:  &statusHandler,
				ServiceHosts:   serviceHosts,
				Owner:          owner,
				Log:            log,
				Ctx:            ctx,
			}
			mrHandlers = append(mrHandlers, &destinationRuleHandler)
			if err := destinationRuleHandler.Handle(); err != nil {
				rls.returnError = err
				rls.subsetMessages[uniqueName] = rls.subsetMessages[uniqueName].AppendDestinationRuleMsg(err.Error())
				break
			}

			virtualServiceHandler := handlers.VirtualServiceHandler{
				Client:         r.Client,
				UniqueName:     uniqueName,
				UniqueVersion:  uniqueVersion,
				RoutePrefix:    helpers.CalculateVirtualServicePrefix(uniqueVersion, s.Name),
				Namespace:      s.Namespace,
				ServiceHosts:   serviceHosts,
				DefaultVersion: defaultVersionForSubset,
				DynamicEnv:     dynamicEnv,
				StatusHandler:  &statusHandler,
				Log:            log,
				Ctx:            ctx,
			}

			mrHandlers = append(mrHandlers, &virtualServiceHandler)
			if err := virtualServiceHandler.Handle(); err != nil {
				if errors.IsConflict(err) {
					ctrl.Log.V(1).Info("ignoring update error due to version conflict", "error", err)
				} else {
					log.Error(err, "error updating virtual service for subset", "subset", s.Name)
					msg := fmt.Sprintf("error updating virtual service for subset (%s)", uniqueName)
					rls.returnError = fmt.Errorf("%s: %w", msg, err)
					rls.subsetMessages[uniqueName] = rls.subsetMessages[uniqueName].AppendVirtualServiceMsg("%s: %s", msg, err)
				}
			}

			commonHostExists := helpers.CommonValueExists(destinationRuleHandler.GetHosts(), virtualServiceHandler.GetHosts())
			if !commonHostExists {
				degradedExists = true
				rls.nonReadyCS[uniqueName] = true
				rls.subsetMessages[uniqueName] = rls.subsetMessages[uniqueName].AppendGlobalMsg("Couldn't find common active service hostname across DestinationRules and VirtualServices")
			}
		}
	}

	for _, handler := range deploymentHandlers {
		newStatus, err := handler.GetStatus()
		if err != nil {
			rls.setErrorIfNotMasking(err)
			rls.subsetMessages[handler.GetSubset()] = rls.subsetMessages[handler.GetSubset()].AppendGlobalMsg("error fetching status: %s", err)
			continue
		}
		log.Info("Handler returned status", "status", newStatus)
		if err = handler.ApplyStatus(newStatus); err != nil {
			log.Error(err, "error updating status", "status", newStatus)
			rls.setErrorIfNotMasking(err)
			rls.subsetMessages[handler.GetSubset()] = rls.subsetMessages[handler.GetSubset()].AppendGlobalMsg("error updating status: %s", err)
		}
		if newStatus.Status != riskifiedv1alpha1.Running {
			nonReadyExists = true
			rls.nonReadyCS[handler.GetSubset()] = true
		}
		if newStatus.Status.IsFailedStatus() {
			degradedExists = true
		}
	}
	for _, handler := range mrHandlers {
		statuses, err := handler.GetStatus()
		if err != nil {
			rls.setErrorIfNotMasking(err)
			rls.subsetMessages[handler.GetSubset()] = rls.subsetMessages[handler.GetSubset()].AppendGlobalMsg("error fetching status: %s", err)
			continue
		}
		log.Info("MRHandler returned statuses", "statuses", statuses)
		if err = handler.ApplyStatus(statuses); err != nil {
			log.Error(err, "error updating status", "statuses", statuses)
			rls.setErrorIfNotMasking(err)
			rls.subsetMessages[handler.GetSubset()] = rls.subsetMessages[handler.GetSubset()].AppendGlobalMsg("error updating status: %s", err)
		}
		for _, s := range statuses {
			if s.Status.IsFailedStatus() {
				rls.nonReadyCS[handler.GetSubset()] = true
				degradedExists = true
			}
		}
	}

	globalState := riskifiedv1alpha1.Processing
	if !nonReadyExists {
		globalState = riskifiedv1alpha1.Ready
	}
	if degradedExists {
		globalState = riskifiedv1alpha1.Degraded
	}
	if rls.returnError != nil {
		globalState = riskifiedv1alpha1.Degraded
	}

	statusHandler.SyncSubsetMessagesToStatus(rls.subsetMessages)
	statusHandler.SyncConsumerMessagesToStatus(rls.consumerMessages)
	if err := statusHandler.SetGlobalState(globalState, len(subsetsAndConsumers), len(rls.nonReadyCS)); err != nil {
		log.Error(err, "error setting global state", "global-state", globalState, "subsetMessages", rls.subsetMessages)
		rls.setErrorIfNotMasking(err)
	}

	if nonReadyExists && rls.returnError == nil {
		// Currently we don't get updates on resource's status changes, so we need to requeue.
		log.V(1).Info("Requeue because of non running status")
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}
	return ctrl.Result{}, rls.returnError
}

func (r *DynamicEnvReconciler) locateMatchingServiceHostnames(ctx context.Context, namespace string, ls labels.Set) (serviceHosts []string, err error) {

	services := v1.ServiceList{}
	var matchingServices []v1.Service
	if err := r.List(ctx, &services, client.InNamespace(namespace)); err != nil {
		return serviceHosts, fmt.Errorf("error fetching services list for namespace %s: %w", namespace, err)
	}
	for _, service := range services.Items {
		var matcher labels.Set = service.Spec.Selector
		selector, err := matcher.AsValidatedSelector()
		if err != nil {
			return serviceHosts, fmt.Errorf("error converting service selector (from: %v): %w", matcher, err)
		}
		if selector.Matches(ls) {
			matchingServices = append(matchingServices, service)
		}
	}

	switch len(matchingServices) {
	case 0:
		return serviceHosts, fmt.Errorf("couldn't find service with matching labels: (%v) in namespace: %s", ls, namespace)
	default:
		for _, sh := range matchingServices {
			serviceHosts = append(serviceHosts, sh.Name)
		}
		// This is mainly to be able to test multiple services in Kuttl, but since I'm not expecting more than a few
		// elements the effect is eligible.
		sort.Strings(serviceHosts)
		return serviceHosts, nil
	}
}

func (r *DynamicEnvReconciler) addFinalizersIfRequired(ctx context.Context, de *riskifiedv1alpha1.DynamicEnv) error {
	if len(de.ObjectMeta.Finalizers) == 0 {
		de.ObjectMeta.Finalizers = []string{names.DeleteDeployments, names.DeleteDestinationRules, names.CleanupVirtualServices}
		if err := r.Update(ctx, de); err != nil {
			return fmt.Errorf("could not update finalizers: %w", err)
		}
	}
	return nil
}

func (r *DynamicEnvReconciler) cleanDynamicEnvResources(ctx context.Context, de *riskifiedv1alpha1.DynamicEnv, version string) (ctrl.Result, error) {
	var resources int
	log := ctrllog.FromContext(ctx)
	log.Info("Dynamic Env marked for deletion, cleaning up ...")
	if helpers.StringSliceContains(names.DeleteDeployments, de.Finalizers) {
		count, err := r.cleanupDeployments(ctx, de)
		if err != nil {
			log.Error(err, "error removing cleanupDeployments finalizer")
			return ctrl.Result{}, err
		}
		resources += count
	}
	if helpers.StringSliceContains(names.DeleteDestinationRules, de.Finalizers) {
		count, err := r.cleanupDestinationRules(ctx, de)
		if err != nil {
			log.Error(err, "error removing DeleteDestinationRules finalizer")
			return ctrl.Result{}, err
		}
		resources += count
	}
	if helpers.StringSliceContains(names.CleanupVirtualServices, de.Finalizers) {
		if err := r.cleanupVirtualServices(ctx, de, version); err != nil {
			log.Error(err, "error removing CleanupVirtualServices finalizer")
			return ctrl.Result{}, err
		}
	}

	if resources > 0 {
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

// Deletes created deployments on controller deletion and deletes the finalizer once all the deployments are deleted.
// Returns the number of identified resources that are not yet deleted (and error if happened).
func (r *DynamicEnvReconciler) cleanupDeployments(ctx context.Context, de *riskifiedv1alpha1.DynamicEnv) (int, error) {
	var deployments []riskifiedv1alpha1.ResourceStatus
	var runningCount int
	for _, s := range de.Status.SubsetsStatus {
		deployments = append(deployments, s.Deployment)
	}
	for _, c := range de.Status.ConsumersStatus {
		deployments = append(deployments, c.ResourceStatus)
	}
	for _, item := range deployments {
		ctrl.Log.Info("Cleaning up deployment ...", "deployment", item)
		found := appsv1.Deployment{}
		if err := r.Get(ctx, types.NamespacedName{Name: item.Name, Namespace: item.Namespace}, &found); err != nil { // if not found assume deleted
			if errors.IsNotFound(err) {
				continue
			}
			return runningCount, fmt.Errorf("error searching for deployment (%v): %w", item, err)
		}
		runningCount += 1
		if err := r.Delete(ctx, &found); err != nil {
			return runningCount, fmt.Errorf("error deleting deployment %v: %w", found, err)
		}
	}
	if runningCount > 0 {
		return runningCount, nil
	}
	return runningCount, r.deleteFinalizer(ctx, names.DeleteDeployments, de)
}

// Deletes created destination rules on controller deletion. Deletes the finalizer once all DRs are deleted. Returns the
// number of running DRs and error.
func (r *DynamicEnvReconciler) cleanupDestinationRules(ctx context.Context, de *riskifiedv1alpha1.DynamicEnv) (int, error) {
	var drs []riskifiedv1alpha1.ResourceStatus
	var runningCount int
	for _, s := range de.Status.SubsetsStatus {
		drs = append(drs, s.DestinationRules...)
	}
	for _, item := range drs {
		ctrl.Log.Info("Cleaning up destination rule ...", "destinationRule", item)
		found := istionetwork.DestinationRule{}
		if err := r.Get(ctx, types.NamespacedName{Name: item.Name, Namespace: item.Namespace}, &found); err != nil {
			if errors.IsNotFound(err) { // if not found assume deleted
				continue
			}
			return runningCount, fmt.Errorf("error searching for destination rule (%v): %w", item, err)
		}
		runningCount += 1
		if err := r.Delete(ctx, &found); err != nil {
			return runningCount, fmt.Errorf("error deleting destination rule %v: %w", &found, err)
		}
	}
	return runningCount, r.deleteFinalizer(ctx, names.DeleteDestinationRules, de)
}

func (r *DynamicEnvReconciler) cleanupVirtualServices(ctx context.Context, de *riskifiedv1alpha1.DynamicEnv, version string) error {
	vss := collectVirtualServices(de)
	for _, item := range vss {
		ctrl.Log.Info("Cleaning up Virtual Service ...", "virtual-service", item)
		found := istionetwork.VirtualService{}
		if err := r.Get(ctx, types.NamespacedName{Name: item.Name, Namespace: item.Namespace}, &found); err != nil {
			if errors.IsNotFound(err) {
				ctrl.Log.Info("Cleanup: Didn't find virtual service. Probably deleted", "virtual-service", item)
				continue
			}
			ctrl.Log.Error(err, "error searching for virtual service during cleanup", "virtual-service", item)
			return err
		}
		var newRoutes []*istioapi.HTTPRoute
		for _, route := range found.Spec.Http {
			if strings.HasPrefix(route.Name, helpers.CalculateVirtualServicePrefix(version, "")) {
				ctrl.Log.V(1).Info("Found route to cleanup", "route", route)
				continue
			}
			newRoutes = append(newRoutes, route)
		}
		found.Spec.Http = newRoutes
		watches.RemoveFromAnnotation(types.NamespacedName{Name: de.Name, Namespace: de.Namespace}, &found)
		if err := r.Update(ctx, &found); err != nil {
			ctrl.Log.Error(err, "error updating virtual service after cleanup", "virtual-service", found.Name)
			return err
		}
	}
	return r.deleteFinalizer(ctx, names.CleanupVirtualServices, de)
}

func (r *DynamicEnvReconciler) deleteFinalizer(ctx context.Context, finalizer string, de *riskifiedv1alpha1.DynamicEnv) error {
	ctrl.Log.Info("Deleting finalizer from dynamic env", "finalizer", finalizer)
	remainingFinalizers := helpers.RemoveItemFromStringSlice(finalizer, de.Finalizers)
	de.Finalizers = remainingFinalizers
	if err := r.Update(ctx, de); err != nil {
		return fmt.Errorf("error removing %s finalizer: %w", finalizer, err)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DynamicEnvReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&riskifiedv1alpha1.DynamicEnv{}).
		Watches(&source.Kind{Type: &appsv1.Deployment{}}, &watches.EnqueueRequestForAnnotation{}).
		Watches(&source.Kind{Type: &istionetwork.DestinationRule{}}, &watches.EnqueueRequestForAnnotation{}).
		Watches(&source.Kind{Type: &istionetwork.VirtualService{}}, &watches.EnqueueRequestForAnnotation{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func mergeSubsetsAndConsumers(subsets, consumers []riskifiedv1alpha1.Subset) []SubsetType {
	var result []SubsetType
	for _, s := range subsets {
		st := SubsetType{
			Type:   riskifiedv1alpha1.SUBSET,
			Subset: s,
		}
		result = append(result, st)
	}
	for _, s := range consumers {
		st := SubsetType{
			Type:   riskifiedv1alpha1.CONSUMER,
			Subset: s,
		}
		result = append(result, st)
	}
	return result
}

func collectVirtualServices(de *riskifiedv1alpha1.DynamicEnv) []riskifiedv1alpha1.ResourceStatus {
	var result []riskifiedv1alpha1.ResourceStatus
	for _, s := range de.Status.SubsetsStatus {
		result = append(result, s.VirtualServices...)
	}
	return result
}

func markedForDeletion(de *riskifiedv1alpha1.DynamicEnv) bool {
	return de.DeletionTimestamp != nil
}
