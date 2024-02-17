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

package model

import (
	"context"

	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// A handler for managing `DynamicEnv` Status.
type StatusManager struct {
	client.Client
	Ctx        context.Context
	DynamicEnv *riskifiedv1alpha1.DynamicEnv
}

// Adds (or updates) a status entry to the *Deployments* status section (if not
// exists).
func (sm *StatusManager) AddDeploymentStatusEntry(subset string, newStatus riskifiedv1alpha1.ResourceStatus, tpe riskifiedv1alpha1.SubsetOrConsumer) error {
	if tpe == riskifiedv1alpha1.CONSUMER {
		return sm.addConsumerDeploymentStatusEntry(subset, newStatus)
	}
	return sm.addSubsetDeploymentStatusEntry(subset, newStatus)
}

// Add a status entry to the *DestinationRules* status section (if not exists).
func (sm *StatusManager) AddDestinationRuleStatusEntry(subset string, newStatus riskifiedv1alpha1.ResourceStatus) error {
	currentStatus := sm.safeGetSubsetsStatus(subset)
	modified, newStatuses := SyncStatusResources(newStatus, currentStatus.DestinationRules)
	if modified {
		currentStatus.DestinationRules = newStatuses
		sm.DynamicEnv.Status.SubsetsStatus[subset] = currentStatus
		return sm.Status().Update(sm.Ctx, sm.DynamicEnv)
	}
	return nil
}

// Add a status entry to the *VirtualServices* status section (if not exists).
func (sm *StatusManager) AddVirtualServiceStatusEntry(subset string, newStatus riskifiedv1alpha1.ResourceStatus) error {
	currentStatus := sm.safeGetSubsetsStatus(subset)
	modified, newStatuses := SyncStatusResources(newStatus, currentStatus.VirtualServices)
	if modified {
		currentStatus.VirtualServices = newStatuses
		sm.DynamicEnv.Status.SubsetsStatus[subset] = currentStatus
		return sm.Status().Update(sm.Ctx, sm.DynamicEnv)
	}
	return nil
}

func (sm *StatusManager) AddGlobalVirtualServiceError(subset, msg string) error {
	currentStatus := sm.safeGetSubsetsStatus(subset)
	statusErrors := SafeGetSubsetErrors(currentStatus)
	currentErrors := statusErrors.VirtualServices
	currentErrors = SyncGlobalErrors(msg, currentErrors)
	statusErrors.VirtualServices = currentErrors
	currentStatus.Errors = &statusErrors
	sm.DynamicEnv.Status.SubsetsStatus[subset] = currentStatus
	return sm.Status().Update(sm.Ctx, sm.DynamicEnv)
}

func (sm *StatusManager) SetGlobalState(state riskifiedv1alpha1.GlobalReadyStatus, totalCount int, notReadyCount int) error {
	sm.DynamicEnv.Status.State = state
	sm.DynamicEnv.Status.TotalCount = totalCount
	sm.DynamicEnv.Status.TotalReady = totalCount - notReadyCount
	return sm.Status().Update(sm.Ctx, sm.DynamicEnv)
}

func (sm *StatusManager) addSubsetDeploymentStatusEntry(subset string, newStatus riskifiedv1alpha1.ResourceStatus) error {
	currentStatus := sm.safeGetSubsetsStatus(subset)
	if !currentStatus.Deployment.IsEqual(newStatus) {
		currentStatus.Deployment = newStatus
		sm.DynamicEnv.Status.SubsetsStatus[subset] = currentStatus
		return sm.Status().Update(sm.Ctx, sm.DynamicEnv)
	}
	return nil
}

func (sm *StatusManager) addConsumerDeploymentStatusEntry(subset string, newStatus riskifiedv1alpha1.ResourceStatus) error {
	currentStatus := sm.safeGetConsumersStatus(subset)
	if !currentStatus.IsEqual(newStatus) {
		currentStatus.Name = newStatus.Name
		currentStatus.Namespace = newStatus.Namespace
		currentStatus.Status = newStatus.Status
		sm.DynamicEnv.Status.ConsumersStatus[subset] = currentStatus
		return sm.Status().Update(sm.Ctx, sm.DynamicEnv)
	}
	return nil
}

func (sm *StatusManager) safeGetSubsetsStatus(subset string) riskifiedv1alpha1.SubsetStatus {
	if sm.DynamicEnv.Status.SubsetsStatus == nil {
		sm.DynamicEnv.Status.SubsetsStatus = make(map[string]riskifiedv1alpha1.SubsetStatus)
	}
	return sm.DynamicEnv.Status.SubsetsStatus[subset]
}

func (sm *StatusManager) safeGetConsumersStatus(subset string) riskifiedv1alpha1.ConsumerStatus {
	if sm.DynamicEnv.Status.ConsumersStatus == nil {
		sm.DynamicEnv.Status.ConsumersStatus = make(map[string]riskifiedv1alpha1.ConsumerStatus)
	}
	return sm.DynamicEnv.Status.ConsumersStatus[subset]
}

func SafeGetSubsetErrors(status riskifiedv1alpha1.SubsetStatus) riskifiedv1alpha1.SubsetErrors {
	if status.Errors != nil {
		return *status.Errors
	}
	return riskifiedv1alpha1.SubsetErrors{}
}

func (sm *StatusManager) SyncSubsetMessagesToStatus(messages map[string]riskifiedv1alpha1.SubsetMessages) {
	for sbst, msgs := range messages {
		statuses := sm.DynamicEnv.Status.SubsetsStatus
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
		sm.DynamicEnv.Status.SubsetsStatus = statuses
	}
}

func (sm *StatusManager) SyncConsumerMessagesToStatus(messages map[string][]string) {
	for subject, msgs := range messages {
		statuses := sm.DynamicEnv.Status.ConsumersStatus
		if statuses == nil {
			statuses = make(map[string]riskifiedv1alpha1.ConsumerStatus)
		}
		cs := statuses[subject]
		for _, msg := range msgs {
			cs.Errors = SyncGlobalErrors(msg, cs.Errors)
		}
		statuses[subject] = cs
		sm.DynamicEnv.Status.ConsumersStatus = statuses
	}
}

func (sm *StatusManager) GetHashForSubset(name string) uint64 {
	subsetStatus, ok := sm.DynamicEnv.Status.SubsetsStatus[name]
	if !ok {
		return 0
	}
	return uint64(subsetStatus.Hash)
}

func (sm *StatusManager) GetHashForConsumer(name string) uint64 {
	consumerStatus, ok := sm.DynamicEnv.Status.ConsumersStatus[name]
	if !ok {
		return 0
	}
	return uint64(consumerStatus.Hash)
}

func (sm *StatusManager) ApplyHash(name string, hash uint64, tpe riskifiedv1alpha1.SubsetOrConsumer) error {
	if tpe == riskifiedv1alpha1.CONSUMER {
		return sm.setHashForConsumer(name, hash)
	} else {
		return sm.setHashForSubset(name, hash)
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
func (sm *StatusManager) setHashForSubset(name string, hash uint64) error {
	subsetStatus, ok := sm.DynamicEnv.Status.SubsetsStatus[name]
	if !ok {
		subsetStatus = riskifiedv1alpha1.SubsetStatus{}
	}
	subsetStatus.Hash = int64(hash)
	sm.DynamicEnv.Status.SubsetsStatus[name] = subsetStatus
	return sm.Status().Update(sm.Ctx, sm.DynamicEnv)
}

func (sm *StatusManager) setHashForConsumer(name string, hash uint64) error {
	if sm.DynamicEnv.Status.ConsumersStatus == nil {
		sm.DynamicEnv.Status.ConsumersStatus = make(map[string]riskifiedv1alpha1.ConsumerStatus)
	}
	consumerStatus, ok := sm.DynamicEnv.Status.ConsumersStatus[name]
	if !ok {
		consumerStatus = riskifiedv1alpha1.ConsumerStatus{}
	}
	consumerStatus.Hash = int64(hash)
	sm.DynamicEnv.Status.ConsumersStatus[name] = consumerStatus
	return sm.Status().Update(sm.Ctx, sm.DynamicEnv)
}
