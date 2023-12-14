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

	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// A handler for managing `DynamicEnv` Status.
type DynamicEnvStatusHandler struct {
	client.Client
	Ctx        context.Context
	DynamicEnv *riskifiedv1alpha1.DynamicEnv
}

// Adds (or updates) a status entry to the *Deployments* status section (if not
// exists).
func (h *DynamicEnvStatusHandler) AddDeploymentStatusEntry(subset string, newStatus riskifiedv1alpha1.ResourceStatus, tpe riskifiedv1alpha1.SubsetOrConsumer) error {
	if tpe == riskifiedv1alpha1.CONSUMER {
		return h.addConsumerDeploymentStatusEntry(subset, newStatus)
	}
	return h.addSubsetDeploymentStatusEntry(subset, newStatus)
}

// Add a status entry to the *DestinationRules* status section (if not exists).
func (h *DynamicEnvStatusHandler) AddDestinationRuleStatusEntry(subset string, newStatus riskifiedv1alpha1.ResourceStatus) error {
	currentStatus := h.safeGetSubsetsStatus(subset)
	modified, newStatuses := SyncStatusResources(newStatus, currentStatus.DestinationRules)
	if modified {
		currentStatus.DestinationRules = newStatuses
		h.DynamicEnv.Status.SubsetsStatus[subset] = currentStatus
		return h.Status().Update(h.Ctx, h.DynamicEnv)
	}
	return nil
}

// Add a status entry to the *VirtualServices* status section (if not exists).
func (h *DynamicEnvStatusHandler) AddVirtualServiceStatusEntry(subset string, newStatus riskifiedv1alpha1.ResourceStatus) error {
	currentStatus := h.safeGetSubsetsStatus(subset)
	modified, newStatuses := SyncStatusResources(newStatus, currentStatus.VirtualServices)
	if modified {
		currentStatus.VirtualServices = newStatuses
		h.DynamicEnv.Status.SubsetsStatus[subset] = currentStatus
		return h.Status().Update(h.Ctx, h.DynamicEnv)
	}
	return nil
}

func (h *DynamicEnvStatusHandler) AddGlobalVirtualServiceError(subset, msg string) error {
	currentStatus := h.safeGetSubsetsStatus(subset)
	statusErrors := SafeGetSubsetErrors(currentStatus)
	currentErrors := statusErrors.VirtualServices
	currentErrors = SyncGlobalErrors(msg, currentErrors)
	statusErrors.VirtualServices = currentErrors
	currentStatus.Errors = &statusErrors
	h.DynamicEnv.Status.SubsetsStatus[subset] = currentStatus
	return h.Status().Update(h.Ctx, h.DynamicEnv)
}

func (h *DynamicEnvStatusHandler) SetGlobalState(state riskifiedv1alpha1.GlobalReadyStatus, totalCount int, notReadyCount int) error {
	h.DynamicEnv.Status.State = state
	h.DynamicEnv.Status.TotalCount = totalCount
	h.DynamicEnv.Status.TotalReady = totalCount - notReadyCount
	return h.Status().Update(h.Ctx, h.DynamicEnv)
}

func (h *DynamicEnvStatusHandler) addSubsetDeploymentStatusEntry(subset string, newStatus riskifiedv1alpha1.ResourceStatus) error {
	currentStatus := h.safeGetSubsetsStatus(subset)
	if !currentStatus.Deployment.IsEqual(newStatus) {
		currentStatus.Deployment = newStatus
		h.DynamicEnv.Status.SubsetsStatus[subset] = currentStatus
		return h.Status().Update(h.Ctx, h.DynamicEnv)
	}
	return nil
}

func (h *DynamicEnvStatusHandler) addConsumerDeploymentStatusEntry(subset string, newStatus riskifiedv1alpha1.ResourceStatus) error {
	currentStatus := h.safeGetConsumersStatus(subset)
	if !currentStatus.IsEqual(newStatus) {
		currentStatus.Name = newStatus.Name
		currentStatus.Namespace = newStatus.Namespace
		currentStatus.Status = newStatus.Status
		h.DynamicEnv.Status.ConsumersStatus[subset] = currentStatus
		return h.Status().Update(h.Ctx, h.DynamicEnv)
	}
	return nil
}

func (h *DynamicEnvStatusHandler) safeGetSubsetsStatus(subset string) riskifiedv1alpha1.SubsetStatus {
	if h.DynamicEnv.Status.SubsetsStatus == nil {
		h.DynamicEnv.Status.SubsetsStatus = make(map[string]riskifiedv1alpha1.SubsetStatus)
	}
	return h.DynamicEnv.Status.SubsetsStatus[subset]
}

func (h *DynamicEnvStatusHandler) safeGetConsumersStatus(subset string) riskifiedv1alpha1.ConsumerStatus {
	if h.DynamicEnv.Status.ConsumersStatus == nil {
		h.DynamicEnv.Status.ConsumersStatus = make(map[string]riskifiedv1alpha1.ConsumerStatus)
	}
	return h.DynamicEnv.Status.ConsumersStatus[subset]
}

func SafeGetSubsetErrors(status riskifiedv1alpha1.SubsetStatus) riskifiedv1alpha1.SubsetErrors {
	if status.Errors != nil {
		return *status.Errors
	}
	return riskifiedv1alpha1.SubsetErrors{}
}

func (h *DynamicEnvStatusHandler) SyncSubsetMessagesToStatus(messages map[string]riskifiedv1alpha1.SubsetMessages) {
	for sbst, msgs := range messages {
		statuses := h.DynamicEnv.Status.SubsetsStatus
		if statuses == nil {
			statuses = make(map[string]riskifiedv1alpha1.SubsetStatus)
		}
		cs := statuses[sbst]
		errors := SafeGetSubsetErrors(cs)
		for _, msg := range msgs.Deployment {
			errors.Deployment = SyncGlobalErrors(msg, errors.Deployment)
		}
		for _, msg := range msgs.DestinationRule {
			errors.DestinationRule = SyncGlobalErrors(msg, errors.DestinationRule)
		}
		for _, msg := range msgs.VirtualService {
			errors.VirtualServices = SyncGlobalErrors(msg, errors.VirtualServices)
		}
		for _, msg := range msgs.GlobalErrors {
			errors.Subset = SyncGlobalErrors(msg, errors.Subset)
		}
		cs.Errors = &errors
		statuses[sbst] = cs
		h.DynamicEnv.Status.SubsetsStatus = statuses
	}
}

func (h *DynamicEnvStatusHandler) SyncConsumerMessagesToStatus(messages map[string][]string) {
	for subject, msgs := range messages {
		statuses := h.DynamicEnv.Status.ConsumersStatus
		if statuses == nil {
			statuses = make(map[string]riskifiedv1alpha1.ConsumerStatus)
		}
		cs := statuses[subject]
		for _, msg := range msgs {
			cs.Errors = SyncGlobalErrors(msg, cs.Errors)
		}
		statuses[subject] = cs
		h.DynamicEnv.Status.ConsumersStatus = statuses
	}
}

func (h *DynamicEnvStatusHandler) GetHashForSubset(name string) uint64 {
	subsetStatus, ok := h.DynamicEnv.Status.SubsetsStatus[name]
	if !ok {
		return 0
	}
	return uint64(subsetStatus.Hash)
}

func (h *DynamicEnvStatusHandler) GetHashForConsumer(name string) uint64 {
	consumerStatus, ok := h.DynamicEnv.Status.ConsumersStatus[name]
	if !ok {
		return 0
	}
	return uint64(consumerStatus.Hash)
}

func (h *DynamicEnvStatusHandler) ApplyHash(name string, hash uint64, tpe riskifiedv1alpha1.SubsetOrConsumer) error {
	if tpe == riskifiedv1alpha1.CONSUMER {
		return h.setHashForConsumer(name, hash)
	} else {
		return h.setHashForSubset(name, hash)
	}
}

func SyncStatusResources(
	s riskifiedv1alpha1.ResourceStatus,
	currentStatuses []riskifiedv1alpha1.ResourceStatus,
) (modified bool, result []riskifiedv1alpha1.ResourceStatus) {
	exists := false
	for _, resource := range currentStatuses {
		newStatus := resource
		if resource.Name == s.Name && resource.Namespace == s.Namespace {
			exists = true
			if resource.Status != s.Status {
				modified = true
				newStatus = s
			}
		}
		result = append(result, newStatus)
	}
	if !exists {
		result = append(result, s)
		modified = true
	}
	return modified, result
}

func SyncGlobalErrors(msg string, errors []riskifiedv1alpha1.StatusError) []riskifiedv1alpha1.StatusError {
	var result []riskifiedv1alpha1.StatusError
	if len(errors) == 0 {
		result = append(result, riskifiedv1alpha1.NewStatusError(msg))
	} else {
		found := false
		for _, e := range errors {
			if e.Error == msg {
				found = true
				e.UpdateTime()
			}
			result = append(result, e)
		}
		if !found {
			result = append(result, riskifiedv1alpha1.NewStatusError(msg))
		}
	}
	return result
}
func (h *DynamicEnvStatusHandler) setHashForSubset(name string, hash uint64) error {
	subsetStatus, ok := h.DynamicEnv.Status.SubsetsStatus[name]
	if !ok {
		subsetStatus = riskifiedv1alpha1.SubsetStatus{}
	}
	subsetStatus.Hash = int64(hash)
	h.DynamicEnv.Status.SubsetsStatus[name] = subsetStatus
	return h.Status().Update(h.Ctx, h.DynamicEnv)
}

func (h *DynamicEnvStatusHandler) setHashForConsumer(name string, hash uint64) error {
	if h.DynamicEnv.Status.ConsumersStatus == nil {
		h.DynamicEnv.Status.ConsumersStatus = make(map[string]riskifiedv1alpha1.ConsumerStatus)
	}
	consumerStatus, ok := h.DynamicEnv.Status.ConsumersStatus[name]
	if !ok {
		consumerStatus = riskifiedv1alpha1.ConsumerStatus{}
	}
	consumerStatus.Hash = int64(hash)
	h.DynamicEnv.Status.ConsumersStatus[name] = consumerStatus
	return h.Status().Update(h.Ctx, h.DynamicEnv)
}
