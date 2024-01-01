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

	"github.com/riskified/dynamic-environment/extensions"

	"github.com/go-logr/logr"
	"github.com/mitchellh/hashstructure/v2"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/pkg/helpers"
	"github.com/riskified/dynamic-environment/pkg/names"
	"github.com/riskified/dynamic-environment/pkg/watches"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// A handler for managing Deployments manipulations.
type DeploymentHandler struct {
	client.Client
	// The unique name of the deployment we need to handle
	UniqueName string
	// The unique version of the deployment we need to handle
	UniqueVersion string
	// The name of the subset/consumer as it appears in the Status map
	SubsetName string
	// THe owner of the deployment we need to handle (e.g. to configure watches)
	Owner types.NamespacedName
	// The deployment we should use as base
	BaseDeployment *appsv1.Deployment
	// Is it a consumer or subset
	DeploymentType riskifiedv1alpha1.SubsetOrConsumer
	// A list of labels to be removed from the overriding deployment
	LabelsToRemove []string
	// The version label to use
	VersionLabel string
	// Status handler (to be able to update status)
	StatusHandler *DynamicEnvStatusHandler
	// The DynamicEnv matchers
	Matches []riskifiedv1alpha1.IstioMatch
	// An indicator whether the current deployment is updating (as opposed to initializing).
	Updating bool
	// The subset that contains the data of our deployment
	Subset riskifiedv1alpha1.Subset
	Log    logr.Logger
	Ctx    context.Context
}

// Handles creation and manipulation of related Deployments.
func (h *DeploymentHandler) Handle() error {
	subset := h.Subset
	sDeployment := &appsv1.Deployment{}
	searchName := types.NamespacedName{Name: h.UniqueName, Namespace: subset.Namespace}
	err := h.Get(h.Ctx, searchName, sDeployment)
	if err != nil && errors.IsNotFound(err) {
		if err := h.setStatus(riskifiedv1alpha1.Initializing); err != nil {
			return fmt.Errorf("failed to update status (prior to adding deployment: %s): %w", h.UniqueName, err)
		}
		sDeployment, err2 := h.createOverridingDeployment()
		if err2 != nil {
			return err2
		}
		h.Log.Info("Deploying newly created overriding deployment", "namespace", subset.Namespace, "name", subset.Name)
		err2 = h.Create(h.Ctx, sDeployment)
		if err2 != nil {
			h.Log.Error(err, "Error deploying", "namespace", subset.Namespace, "name", subset.Name)
			return err2
		}
		hash, err := hashstructure.Hash(h.Subset, hashstructure.FormatV2, nil)
		if err != nil {
			return fmt.Errorf("calculating hash for %q: %w", h.UniqueName, err)
		}
		if err := h.StatusHandler.ApplyHash(h.SubsetName, hash, h.DeploymentType); err != nil {
			return fmt.Errorf("setting subset hash on deployment creation %q: %w", h.UniqueName, err)
		}

		return nil
	} else if err != nil {
		h.Log.Error(err, "Failed to get matching deployment for subset")
		return err
	}
	return h.UpdateIfRequired()
}

func (h *DeploymentHandler) GetStatus() (riskifiedv1alpha1.ResourceStatus, error) {

	genStatus := func(s riskifiedv1alpha1.LifeCycleStatus) riskifiedv1alpha1.ResourceStatus {
		return riskifiedv1alpha1.ResourceStatus{
			Name:      h.UniqueName,
			Namespace: h.Subset.Namespace,
			Status:    s,
		}
	}

	deployment := &appsv1.Deployment{}
	searchName := types.NamespacedName{Name: h.UniqueName, Namespace: h.Subset.Namespace}
	if err := h.Get(h.Ctx, searchName, deployment); err != nil {
		if errors.IsNotFound(err) {
			return genStatus(riskifiedv1alpha1.Missing), nil
		} else {
			h.Log.Error(err, "error searching for deployment in getStatus", "name", h.UniqueName, "namespace", h.Subset.Namespace)
			e := fmt.Errorf("error searching for deployment: %w", err)
			return riskifiedv1alpha1.ResourceStatus{}, e
		}
	}
	h.Log.V(1).Info("status found for deployment", "deployment", searchName, "status", deployment.Status)

	found, processing := getMostRecentConditionForType(appsv1.DeploymentProgressing, deployment.Status.Conditions)
	if !found {
		return genStatus(riskifiedv1alpha1.Unknown), nil
	}
	if processing.Status == v1.ConditionTrue {
		if processing.Reason == "NewReplicaSetAvailable" {
			return genStatus(riskifiedv1alpha1.Running), nil
		} else {
			var status riskifiedv1alpha1.LifeCycleStatus
			if h.Updating {
				status = riskifiedv1alpha1.Updating
			} else {
				status = riskifiedv1alpha1.Initializing
			}
			return genStatus(status), nil
		}
	} else {
		return genStatus(riskifiedv1alpha1.Failed), nil
	}
}

func (h *DeploymentHandler) ApplyStatus(rs riskifiedv1alpha1.ResourceStatus) error {
	return h.StatusHandler.AddDeploymentStatusEntry(h.SubsetName, rs, h.DeploymentType)
}

func (h *DeploymentHandler) GetSubset() string {
	return h.SubsetName
}

func (h *DeploymentHandler) UpdateIfRequired() error {
	h.Log.V(1).Info("Checking whether it's required to update subset", "subset namespace", h.Subset.Namespace, "subset name", h.Subset.Name)
	s := h.Subset
	var existingHash uint64
	if h.DeploymentType == riskifiedv1alpha1.CONSUMER {
		existingHash = h.StatusHandler.GetHashForConsumer(h.SubsetName)
	} else {
		existingHash = h.StatusHandler.GetHashForSubset(h.SubsetName)
	}
	hash, err := hashstructure.Hash(h.Subset, hashstructure.FormatV2, nil)
	if err != nil {
		return fmt.Errorf("could not calculate hash of subset '%s/%s': %w", s.Namespace, s.Name, err)
	}
	if existingHash != hash {
		h.Updating = true
		h.Log.V(1).Info("Deployment update required", "subset", h.UniqueName)
		newDeployment, err := h.createOverridingDeployment()
		if err != nil {
			return err
		}
		if err := h.setStatus(riskifiedv1alpha1.Updating); err != nil {
			return fmt.Errorf("failed to update status (prior to update deployment: %s): %w", h.UniqueName, err)
		}
		if err := h.Update(h.Ctx, newDeployment); err != nil {
			h.Log.Error(err, "Updating Subset", "subset full name", h.UniqueName)
			return fmt.Errorf("error updating subset %s: %w", h.UniqueName, err)
		}
		if err := h.StatusHandler.ApplyHash(h.SubsetName, hash, h.DeploymentType); err != nil {
			return fmt.Errorf("setting subset hash for '%s': %w", h.UniqueName, err)
		}
	}
	return nil
}

func (h *DeploymentHandler) createOverridingDeployment() (*appsv1.Deployment, error) {
	oldMeta := h.BaseDeployment.ObjectMeta.DeepCopy()
	newSpec := h.BaseDeployment.Spec.DeepCopy()
	oldMeta.Labels[h.VersionLabel] = h.UniqueVersion
	oldMeta.Labels[names.DynamicEnvLabel] = "true"
	for _, l := range h.LabelsToRemove {
		delete(oldMeta.Labels, l)
	}
	newSpec.Selector.MatchLabels[h.VersionLabel] = h.UniqueVersion
	var replicas int32 = 1
	if h.Subset.Replicas != nil {
		replicas = *h.Subset.Replicas
	}
	*newSpec.Replicas = replicas
	template := newSpec.Template
	template.ObjectMeta.Labels[h.VersionLabel] = h.UniqueVersion
	template.ObjectMeta.Labels[names.DynamicEnvLabel] = "true"
	for k, v := range h.Subset.PodLabels {
		template.ObjectMeta.Labels[k] = v
	}

	// Main container overrides
	for _, c := range h.Subset.Containers {
		container, idx, err := createContainerTemplate(template.Spec.Containers, c)
		if err != nil {
			newError := fmt.Errorf("processing containers in subset %s: %w", h.Subset.Name, err)
			return &appsv1.Deployment{}, newError
		}
		template.Spec.Containers[idx] = container
	}

	// Init container overrides
	for _, ic := range h.Subset.InitContainers {
		initContainer, idx, err := createContainerTemplate(template.Spec.InitContainers, ic)
		if err != nil {
			newError := fmt.Errorf("processing init-containers in subset %s: %w", h.Subset.Name, err)
			return &appsv1.Deployment{}, newError
		}
		template.Spec.InitContainers[idx] = initContainer
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.UniqueName,
			Namespace: h.Subset.Namespace,
			Labels:    oldMeta.Labels,
		},
		Spec: *newSpec,
	}

	depData := extensions.DeploymentExtensionData{
		BaseDeployment: h.BaseDeployment,
		Matches:        h.Matches,
		Subset:         h.Subset,
	}

	if err := extensions.ExtendOverridingDeployment(dep, depData); err != nil {
		return dep, err
	}

	watches.AddToAnnotation(h.Owner, dep)
	return dep, nil
}

func (h *DeploymentHandler) setStatus(status riskifiedv1alpha1.LifeCycleStatus) error {
	currentState := riskifiedv1alpha1.ResourceStatus{
		Name:      h.UniqueName,
		Namespace: h.Subset.Namespace,
		Status:    status,
	}
	if err := h.StatusHandler.AddDeploymentStatusEntry(h.SubsetName, currentState, h.DeploymentType); err != nil {
		return err
	}
	return nil
}

func createContainerTemplate(containers []v1.Container, containerOverrides riskifiedv1alpha1.ContainerOverrides) (v1.Container, int, error) {
	idx, err := findContainerIndex(&containers, containerOverrides.ContainerName)
	if err != nil {
		return v1.Container{}, 0, err
	}
	container := containers[idx]
	if containerOverrides.Image != "" {
		container.Image = containerOverrides.Image
	}
	if len(containerOverrides.Command) > 0 {
		container.Command = containerOverrides.Command
	}
	container.Env = helpers.MergeEnvVars(container.Env, containerOverrides.Env)
	return container, idx, nil
}

// Returns the container with the specified name. Fails if not found
func findContainerIndex(containers *[]v1.Container, defaultName string) (int, error) {
	if len(*containers) < 1 {
		// Todo: How do we need to treat this situation? Is it even possible to have empty container list?
		//goland:noinspection GoErrorStringFormat
		return 0, fmt.Errorf("how did we get empty container list?")
	}

	for idx, c := range *containers {
		if c.Name == defaultName {
			return idx, nil
		}
	}
	return 0, fmt.Errorf("container name %s does'nt exist", defaultName)
}

func getMostRecentConditionForType(typ appsv1.DeploymentConditionType, conditions []appsv1.DeploymentCondition) (found bool, c appsv1.DeploymentCondition) {
	var result appsv1.DeploymentCondition
	for _, c := range conditions {
		if c.Type == typ {
			if found {
				if result.LastUpdateTime.Before(&c.LastUpdateTime) {
					result = c
				}
			} else {
				found = true
				result = c
			}
		}
	}
	return found, result
}
