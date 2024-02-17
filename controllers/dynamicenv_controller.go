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

package controllers

import (
	"context"
	"fmt"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/pkg/handlers"
	"github.com/riskified/dynamic-environment/pkg/helpers"
	"github.com/riskified/dynamic-environment/pkg/model"
	"github.com/riskified/dynamic-environment/pkg/names"
	"github.com/riskified/dynamic-environment/pkg/watches"
	istionetwork "istio.io/client-go/pkg/apis/networking/v1alpha3"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sort"
)

var log = helpers.MkLogger("DynamicEnvReconciler")

// DynamicEnvReconciler reconciles a DynamicEnv object
type DynamicEnvReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	VersionLabel   string
	DefaultVersion string
	LabelsToRemove []string
}

type ReconcileLoopStatus struct {
	returnError      model.NonMaskingError
	cleanupError     error
	cleanupProgress  bool
	subsetMessages   map[string]riskifiedv1alpha1.SubsetMessages
	consumerMessages map[string][]string
	// Non-ready consumers and subsets
	nonReadyCS map[string]bool
}

type processSubsetResponse struct {
	msgs              riskifiedv1alpha1.SubsetMessages
	deploymentHandler handlers.SRHandler
	mrHandlers        []handlers.MRHandler
	noCommonHost      bool
}

type processStatusesResponse struct {
	msgs     map[string][]string
	degraded bool
	nonReady map[string]bool
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
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *DynamicEnvReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

	resourceName := fmt.Sprintf("%s/%s", dynamicEnv.Namespace, dynamicEnv.Name)
	log := log.WithValues("resource", resourceName)

	cleanupManager := model.CleanupManager{
		Client: r.Client,
		Ctx:    ctx,
		Log:    helpers.MkLogger("CleanupManager", "resource", resourceName),
		DE:     dynamicEnv,
	}

	if markedForDeletion(dynamicEnv) {
		return cleanupManager.DeleteAllResources()
	}

	if err := r.addFinalizersIfRequired(ctx, dynamicEnv); err != nil {
		log.Error(err, "Error adding finalizers")
		return ctrl.Result{}, err
	}

	var deploymentHandlers []handlers.SRHandler
	var mrHandlers []handlers.MRHandler
	nonReadyExists := false
	degradedExists := false

	statusManager := model.StatusManager{
		Client:     r.Client,
		Ctx:        ctx,
		DynamicEnv: dynamicEnv,
	}

	subsetsAndConsumers := mergeSubsetsAndConsumers(dynamicEnv.Spec.Subsets, dynamicEnv.Spec.Consumers)

	toRemove := cleanupManager.CheckForRemovedSubsetsAndConsumers(subsetsAndConsumers)
	if len(toRemove) > 0 {
		inProgress, err := cleanupManager.RemoveSubsetsAndConsumers(toRemove)
		if err != nil {
			rls.cleanupError = err
		}
		rls.cleanupProgress = inProgress
	}

	for _, st := range subsetsAndConsumers {
		s := st.Subset
		defaultVersionForSubset := r.DefaultVersion
		if s.DefaultVersion != "" {
			defaultVersionForSubset = s.DefaultVersion
		}

		subsetName := helpers.MKSubsetName(s)
		baseDeployment := &appsv1.Deployment{}
		err := r.Get(ctx, types.NamespacedName{Name: s.Name, Namespace: s.Namespace}, baseDeployment)
		if err != nil {
			log.Error(err, "couldn't find the deployment we need to override", "deployment-name", s.Name, "namespace", s.Namespace)
			msg := fmt.Sprintf("couldn't find the deployment we need to override (name: %s, ns: %s)", s.Name, s.Namespace)
			rls.returnError.ForceError(fmt.Errorf("%s: %w", msg, err))
			rls.addDeploymentMessage(subsetName, st.Type, "%s, %v", msg, err)
			rls.nonReadyCS[subsetName] = true
			break
		}

		subsetData := model.DynamicEnvReconcileData{
			Identifier:     types.NamespacedName{Namespace: dynamicEnv.Namespace, Name: dynamicEnv.Name},
			BaseDeployment: baseDeployment,
			Subset:         s,
			StatusManager:  &statusManager,
			Matches:        dynamicEnv.Spec.IstioMatches,
		}

		if st.Type == riskifiedv1alpha1.CONSUMER {
			handler, err := r.processConsumer(ctx, subsetData)
			deploymentHandlers = append(deploymentHandlers, handler)
			if err != nil {
				rls.returnError.ForceError(err)
				rls.addDeploymentMessage(subsetName, riskifiedv1alpha1.CONSUMER, err.Error())
			}
		}

		if st.Type == riskifiedv1alpha1.SUBSET {
			response, err := r.processSubset(ctx, subsetData, defaultVersionForSubset)
			deploymentHandlers = append(deploymentHandlers, response.deploymentHandler)
			mrHandlers = append(mrHandlers, response.mrHandlers...)
			rls.subsetMessages[subsetName] = response.msgs
			if response.noCommonHost {
				degradedExists = true
				rls.nonReadyCS[subsetName] = true
			}
			if err != nil {
				rls.returnError.ForceError(err)
			}
		}
	}

	if rls.cleanupError != nil {
		// While the error occurred previously, these errors are less important than the main subsets loop, so we apply
		// them now only if not masking.
		// TODO: This seem to originally be a bug - TEST!!
		rls.returnError.SetIfNotMasking(rls.cleanupError)
	}

	response, err := r.processStatuses(ctx, deploymentHandlers, mrHandlers)
	if err != nil {
		rls.returnError.SetIfNotMasking(err)
	}
	if len(response.nonReady) > 0 {
		nonReadyExists = true
	}
	for k, v := range response.nonReady {
		rls.nonReadyCS[k] = v
	}

	if rls.cleanupProgress {
		nonReadyExists = true
	}
	globalState := riskifiedv1alpha1.Processing
	if !nonReadyExists {
		globalState = riskifiedv1alpha1.Ready
	}
	if degradedExists {
		globalState = riskifiedv1alpha1.Degraded
	}
	if !rls.returnError.IsNil() {
		globalState = riskifiedv1alpha1.Degraded
	}

	statusManager.SyncSubsetMessagesToStatus(rls.subsetMessages)
	statusManager.SyncConsumerMessagesToStatus(rls.consumerMessages)
	if err := statusManager.SetGlobalState(globalState, len(subsetsAndConsumers), len(rls.nonReadyCS)); err != nil {
		// TODO: do we need this manual checking now that we're handling conflicts globally?
		if errors.IsConflict(err) {
			log.Info("Ignoring global status update error due to conflict")
		} else {
			log.Error(err, "error setting global state", "global-state", globalState, "subsetMessages", rls.subsetMessages)
			rls.returnError.SetIfNotMasking(err)
		}
	}

	if nonReadyExists && rls.returnError.IsNil() {
		// Currently we don't get updates on resource's status changes, so we need to requeue.
		log.V(1).Info("Requeue because of non running status")
		return ctrl.Result{Requeue: true}, nil
	}
	if errors.IsConflict(rls.returnError.Get()) {
		log.Info("Skipping conflict error")
		// TODO: Do we really need to requeue? Because of the conflict it should run again anyway...
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, rls.returnError.Get()
}

func (r *DynamicEnvReconciler) processConsumer(
	ctx context.Context,
	subsetData model.DynamicEnvReconcileData,
) (handlers.SRHandler, error) {
	deploymentHandler := handlers.NewDeploymentHandler(subsetData, r.Client, riskifiedv1alpha1.CONSUMER, r.LabelsToRemove, r.VersionLabel)
	if err := deploymentHandler.Handle(ctx); err != nil {
		return deploymentHandler, err
	}
	return deploymentHandler, nil
}

func (r *DynamicEnvReconciler) processSubset(
	ctx context.Context,
	subsetData model.DynamicEnvReconcileData,
	defaultVersionForSubset string,
) (response processSubsetResponse, _ error) {
	s := subsetData.Subset
	uniqueVersion := helpers.UniqueDynamicEnvName(subsetData.Identifier)
	uniqueName := helpers.MkSubsetUniqueName(s.Name, uniqueVersion)
	deploymentHandler := handlers.NewDeploymentHandler(subsetData, r.Client, riskifiedv1alpha1.SUBSET, r.LabelsToRemove, r.VersionLabel)
	response.deploymentHandler = deploymentHandler
	if err := deploymentHandler.Handle(ctx); err != nil {
		response.msgs = response.msgs.AppendDeploymentMsg(err.Error())
		return response, err
	}

	serviceHosts, err := r.locateMatchingServiceHostnames(ctx, s.Namespace, subsetData.BaseDeployment.Spec.Template.ObjectMeta.Labels)
	if err != nil {
		msg := fmt.Sprintf("locating service hostname for deployment '%s'", subsetData.BaseDeployment.Name)
		response.msgs = response.msgs.AppendGlobalMsg("%s: %v", msg, err)
		return response, fmt.Errorf("%s: %w", msg, err)
	}

	destinationRuleHandler := handlers.NewDestinationRuleHandler(subsetData, r.VersionLabel, defaultVersionForSubset, serviceHosts, r.Client)
	response.mrHandlers = append(response.mrHandlers, destinationRuleHandler)
	if err := destinationRuleHandler.Handle(ctx); err != nil {
		response.msgs = response.msgs.AppendDestinationRuleMsg(err.Error())
		return response, err
	}

	virtualServiceHandler := handlers.NewVirtualServiceHandler(subsetData, serviceHosts, defaultVersionForSubset, r.Client)
	var returnError error // so we'll be able to test for commonHost even if an error occurred.
	response.mrHandlers = append(response.mrHandlers, virtualServiceHandler)
	if err := virtualServiceHandler.Handle(ctx); err != nil {
		if errors.IsConflict(err) {
			// TODO: Check the option for removal of this spacial clause. We already handle conflicts in the end of the reconcile loop.
			log.V(1).Info("ignoring update error due to version conflict", "error", err)
		} else {
			log.Error(err, "error updating virtual service for subset", "subset", s.Name)
			msg := fmt.Sprintf("error updating virtual service for subset (%s)", uniqueName)
			response.msgs = response.msgs.AppendVirtualServiceMsg("%s: %s", msg, err)
			returnError = fmt.Errorf("%s: %w", msg, err)
		}
	}

	commonHostExists := helpers.CommonValueExists(destinationRuleHandler.GetHosts(), virtualServiceHandler.GetHosts())
	if !commonHostExists {
		response.msgs = response.msgs.AppendGlobalMsg("Couldn't find common active service hostname across DestinationRules and VirtualServices")
		response.noCommonHost = true
	}

	return response, returnError
}

func (r *DynamicEnvReconciler) processStatuses(ctx context.Context, srhs []handlers.SRHandler, mrhs []handlers.MRHandler) (processStatusesResponse, error) {
	response := processStatusesResponse{
		msgs:     make(map[string][]string),
		nonReady: make(map[string]bool),
		degraded: false,
	}
	var returnError model.NonMaskingError

	for _, handler := range srhs {
		newStatus, err := handler.GetStatus(ctx)
		if err != nil {
			returnError.SetIfNotMasking(err)
			response.msgs[handler.GetSubset()] = append(response.msgs[handler.GetSubset()], fmt.Sprintf("error fetching status: %s", err))
			continue
		}
		log.Info("Handler returned status", "status", newStatus)
		if err = handler.ApplyStatus(newStatus); err != nil {
			log.Error(err, "error updating status", "status", newStatus)
			returnError.SetIfNotMasking(err)
			response.msgs[handler.GetSubset()] = append(response.msgs[handler.GetSubset()], fmt.Sprintf("error updating status: %s", err))
		}
		if newStatus.Status != riskifiedv1alpha1.Running {
			response.nonReady[handler.GetSubset()] = true
		}
		if newStatus.Status.IsFailedStatus() {
			response.degraded = true
		}
	}
	for _, handler := range mrhs {
		statuses, err := handler.GetStatus(ctx)
		if err != nil {
			msg := fmt.Sprintf("error fetching status: %s", err)
			returnError.SetIfNotMasking(err)
			response.msgs[handler.GetSubset()] = append(response.msgs[handler.GetSubset()], msg)
			continue
		}
		log.Info("MRHandler returned statuses", "statuses", statuses)
		if err = handler.ApplyStatus(statuses); err != nil {
			log.Error(err, "error updating status", "statuses", statuses)
			msg := fmt.Sprintf("error updating status: %s", err)
			returnError.SetIfNotMasking(err)
			response.msgs[handler.GetSubset()] = append(response.msgs[handler.GetSubset()], msg)
		}
		for _, s := range statuses {
			if s.Status.IsFailedStatus() {
				response.nonReady[handler.GetSubset()] = true
				response.degraded = true
			}
		}
	}
	return response, returnError.Get()
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
		//  elements, the effect is eligible.
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

func mergeSubsetsAndConsumers(subsets, consumers []riskifiedv1alpha1.Subset) []riskifiedv1alpha1.SubsetType {
	var result []riskifiedv1alpha1.SubsetType
	for _, s := range subsets {
		st := riskifiedv1alpha1.SubsetType{
			Type:   riskifiedv1alpha1.SUBSET,
			Subset: s,
		}
		result = append(result, st)
	}
	for _, s := range consumers {
		st := riskifiedv1alpha1.SubsetType{
			Type:   riskifiedv1alpha1.CONSUMER,
			Subset: s,
		}
		result = append(result, st)
	}
	return result
}

func markedForDeletion(de *riskifiedv1alpha1.DynamicEnv) bool {
	return de.DeletionTimestamp != nil
}
