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

package v1alpha1

import (
	"encoding/json"
	"fmt"
	"reflect"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type LifeCycleStatus string
type GlobalReadyStatus string
type SubsetOrConsumer int

const (
	// Various life cycle statuses
	Unknown          LifeCycleStatus = "unknown" // The default status
	Initializing     LifeCycleStatus = "initializing"
	Running          LifeCycleStatus = "running"
	Failed           LifeCycleStatus = "failed"
	Missing          LifeCycleStatus = "missing"
	Updating         LifeCycleStatus = "updating"
	IgnoredMissingDR LifeCycleStatus = "ignored-missing-destination-rule"
	IgnoredMissingVS LifeCycleStatus = "ignored-missing-virtual-service"

	// Statuses for the global readiness (argocd ready check)
	Degraded   GlobalReadyStatus = "degraded"
	Ready      GlobalReadyStatus = "ready"
	Processing GlobalReadyStatus = "processing"

	// Whether it's consumer or subset.
	SUBSET   SubsetOrConsumer = iota
	CONSUMER SubsetOrConsumer = iota
)

func (s LifeCycleStatus) String() string {
	defaultResult := string(Unknown)
	switch s {
	case Unknown:
		return defaultResult
	case Initializing:
		return string(Initializing)
	case Running:
		return string(Running)
	case Failed:
		return string(Failed)
	case Missing:
		return string(Missing)
	case Updating:
		return string(Updating)
	case IgnoredMissingDR:
		return string(IgnoredMissingDR)
	case IgnoredMissingVS:
		return string(IgnoredMissingVS)
	}
	return defaultResult
}

func ParseLifeCycleStatus(s string) LifeCycleStatus {
	switch s {
	case string(Initializing):
		return Initializing
	case string(Running):
		return Running
	case string(Failed):
		return Failed
	case string(Missing):
		return Missing
	case string(Updating):
		return Updating
	case string(IgnoredMissingDR):
		return IgnoredMissingDR
	case string(IgnoredMissingVS):
		return IgnoredMissingVS
	}
	return Unknown
}

func (s LifeCycleStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *LifeCycleStatus) UnmarshalJSON(data []byte) error {
	var status string
	if err := json.Unmarshal(data, &status); err != nil {
		return err
	}
	*s = ParseLifeCycleStatus(status)
	return nil
}

func (s *LifeCycleStatus) IsFailedStatus() bool {
	return *s == Missing || *s == Failed
}

func (s *GlobalReadyStatus) String() string {
	switch *s {
	case Degraded:
		return string(Degraded)
	case Processing:
		return string(Processing)
	case Ready:
		return string(Ready)
	}
	return string(Processing)
}

func ParseGlobalReadyStatus(s string) GlobalReadyStatus {
	switch s {
	case "degraded":
		return Degraded
	case "ready":
		return Ready
	}
	return Processing
}

func (s GlobalReadyStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *GlobalReadyStatus) UnmarshalJSON(data []byte) error {
	var status string
	if err := json.Unmarshal(data, &status); err != nil {
		return err
	}
	*s = ParseGlobalReadyStatus(status)
	return nil
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=dynamicenvs,scope=Namespaced,shortName=de
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="Status of the DynamicEnv"
// +kubebuilder:printcolumn:name="Desired",type="integer",JSONPath=".status.totalCount",description="displays desired subsets and consumers count"
// +kubebuilder:printcolumn:name="Current",type="integer",JSONPath=".status.totalReady",description="displays how many subsets and consumers are available"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DynamicEnv is the Schema for the dynamicenvs API
type DynamicEnv struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DynamicEnvSpec   `json:"spec,omitempty"`
	Status DynamicEnvStatus `json:"status,omitempty"`
}

// DynamicEnvSpec defines the desired state of DynamicEnv
type DynamicEnvSpec struct {
	// A list of matchers (partly corresponds to IstioMatch). Each match will have a rule of its
	// own (merged with existing rules) ordered by their order here.
	IstioMatches []IstioMatch `json:"istioMatches"`

	// Who should participate in the given dynamic environment
	Subsets []Subset `json:"subsets"`

	// Consumers are like subsets but for deployments that do not open a service but connect to external resources for
	// their work (e.g, offline workers). They are equivalent to subsets in the sense that they launch overriding
	// deployments with custom image and/or settings. However, since they are only consumers no virtual service or
	// destination route will be pointing to them.
	Consumers []Subset `json:"consumers,omitempty"`
}

// specifies a set of criterion to be met in order for the rule to be applied to the HTTP request
// This field is immutable after creation.
type IstioMatch struct {
	// Header values are case-sensitive and formatted as follows:<br/>
	// - `exact: "value"` for exact string match<br/>
	// - `prefix: "value"` for prefix-based match<br/>
	// - `regex: "value"` for RE2 style regex-based match (https://github.com/google/re2/wiki/Syntax).
	Headers map[string]StringMatch `json:"headers,omitempty"`

	// One or more labels that constrain the applicability of a rule to source (client) workloads
	// with the given labels.
	SourceLabels map[string]string `json:"sourceLabels,omitempty"`
}

// Describes how to match a given string in HTTP headers. Match is case-sensitive.
// one and only one of the fields needs to be defined (oneof)
type StringMatch struct {
	Exact  string `json:"exact,omitempty"`
	Prefix string `json:"prefix,omitempty"`
	Regex  string `json:"regex,omitempty"`
}

// Subsets defines how to generate subsets from existing Deployments
type Subset struct {
	// Deployment name (without namespace)
	Name string `json:"name"`

	// Namespace where the deployment is deployed
	Namespace string `json:"namespace"`

	// Labels to add to the pods of the deployment launched by this subset. Could be used in
	// conjunction with 'SourceLabels' in the `IstioMatches`.
	PodLabels map[string]string `json:"podLabels,omitempty"`

	// Number of deployment replicas. Default is 1. Note: 0 is *invalid*.
	Replicas *int32 `json:"replicas,omitempty"`

	// A list of container overrides (at least one of Containers or InitContainers must not be empty)
	Containers []ContainerOverrides `json:"containers,omitempty"`

	// A list of init container overrides (at least one of Containers or InitContainers must not be empty)
	InitContainers []ContainerOverrides `json:"initContainers,omitempty"`

	// Default version for this subset (if different then the global default version). This is the
	// version that will get the default route.
	DefaultVersion string `json:"defaultVersion,omitempty"`
}

// Defines the details of the container on which changes need to be made
// and the relevant overrides
type ContainerOverrides struct {
	// Container name to override in multiple containers' environment. If not
	// specified we will use the first container.
	ContainerName string `json:"containerName"`

	// Docker image name overridden to the desired subset
	// The Docker image found in the original deployment is used if this is not provided.
	// +optional
	Image string `json:"image,omitempty"`

	// Entrypoint array overridden to the desired subset
	// The docker image's ENTRYPOINT is used if this is not provided.
	// +optional
	Command []string `json:"command,omitempty"`

	// Additional environment variable to the given deployment
	// +optional
	Env []v1.EnvVar `json:"env,omitempty"`
}

// DynamicEnvStatus defines the observed state of DynamicEnv
type DynamicEnvStatus struct {
	// Represents the latest available observations of a deployment's current state.
	Conditions      []Condition               `json:"conditions,omitempty"`
	SubsetsStatus   map[string]SubsetStatus   `json:"subsetsStatus"`
	ConsumersStatus map[string]ConsumerStatus `json:"consumersStatus,omitempty"`
	State           GlobalReadyStatus         `json:"state,omitempty"`
	// desired subsets and consumers count
	TotalCount int `json:"totalCount,omitempty"`
	// number of available subsets and consumers
	TotalReady int `json:"totalReady"`
}

//+kubebuilder:object:root=true

// DynamicEnvList contains a list of DynamicEnv
type DynamicEnvList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamicEnv `json:"items"`
}

// SubsetStatus Contains aggregation of all resources status connected to set subset.
type SubsetStatus struct {
	// Status of the deployment that belongs to the subset
	Deployment ResourceStatus `json:"deployment,omitempty"`
	// Status of the destination-rule that belongs to the subset
	DestinationRules []ResourceStatus `json:"destinationRules,omitempty"`
	// Status of the virtual-service that belongs to the subset
	VirtualServices []ResourceStatus `json:"virtualServices,omitempty"`
	// A list of global errors related to subset resources
	Errors *SubsetErrors `json:"subsetErrors,omitempty"`
	// Hash of the current subset - for internal use
	Hash int64 `json:"hash,omitempty"`
}

// SubsetErrors contains all global errors related to set subset.
type SubsetErrors struct {
	// Subset's deployment global errors.
	Deployment []StatusError `json:"deployment,omitempty"`
	// Subset's destination-rule global errors.
	DestinationRule []StatusError `json:"destinationRule,omitempty"`
	// Subset's virtual-services global errors.
	VirtualServices []StatusError `json:"virtualServices,omitempty"`
	// Errors related to subset but not to any of the launched resources
	Subset []StatusError `json:"subset,omitempty"`
}

type ConsumerStatus struct {
	ResourceStatus `json:",inline"`
	// Hash of the current consumer - for internal use
	Hash int64 `json:"hash,omitempty"`
	// List of errors related to the consumer
	Errors []StatusError `json:"errors,omitempty"`
}

// ResourceStatus shows the status of each item created/edited by DynamicEnv
type ResourceStatus struct {
	// The name of the resource
	Name string `json:"name"`
	// The namespace where the resource is created
	Namespace string `json:"namespace"`
	// The life cycle status of the resource
	Status LifeCycleStatus `json:"status"`
}

func (rs ResourceStatus) IsEqual(other ResourceStatus) bool {
	return rs.Name == other.Name && rs.Namespace == other.Namespace && rs.Status == other.Status
}

// StatusError shows an error we want to display in the status with the last time it happened. This
// *does not* have to be the only time it happened. The idea is that a list of errors should only
// contain single occurrence of an error (just the last).
type StatusError struct {
	// The error message
	Error string `json:"error"`
	// THe last occurrence of the error
	LastOccurrence metav1.Time `json:"lastOccurrence"`
}

// SubsetMessages contains a list of messages (errors) that occurred during Reconcile loop. At the
// end of each loop these messages should be synced to the matching subset status.
type SubsetMessages struct {
	Deployment      []string
	DestinationRule []string
	VirtualService  []string
	GlobalErrors    []string
}

func NewStatusError(error string) StatusError {
	return StatusError{
		Error:          error,
		LastOccurrence: metav1.Now(),
	}
}

func (co ContainerOverrides) IsEmpty() bool {
	return reflect.DeepEqual(co, ContainerOverrides{})
}

func (se *StatusError) SameError(other StatusError) bool {
	return se.Error == other.Error
}

func (se *StatusError) UpdateTime() {
	se.LastOccurrence = metav1.Now()
}

func init() {
	SchemeBuilder.Register(&DynamicEnv{}, &DynamicEnvList{})
}

func (rls SubsetMessages) AppendDeploymentMsg(format string, a ...interface{}) SubsetMessages {
	msg := fmt.Sprintf(format, a...)
	rls.Deployment = append(rls.Deployment, msg)
	return rls
}

func (rls SubsetMessages) AppendDestinationRuleMsg(format string, a ...interface{}) SubsetMessages {
	msg := fmt.Sprintf(format, a...)
	rls.DestinationRule = append(rls.DestinationRule, msg)
	return rls
}

func (rls SubsetMessages) AppendVirtualServiceMsg(format string, a ...interface{}) SubsetMessages {
	msg := fmt.Sprintf(format, a...)
	rls.VirtualService = append(rls.VirtualService, msg)
	return rls
}

func (rls SubsetMessages) AppendGlobalMsg(format string, a ...interface{}) SubsetMessages {
	msg := fmt.Sprintf(format, a...)
	rls.GlobalErrors = append(rls.GlobalErrors, msg)
	return rls
}
