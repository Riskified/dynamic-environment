package model

import (
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// A data structure that contains a lot of the state that is passed from `Reconcile` to various handlers / processors.
// Used to reduce clutter.
type DynamicEnvReconcileData struct {
	// The name/namespace of the current custom resource.
	Identifier types.NamespacedName
	// The deployment we're creating subsets for.
	BaseDeployment *appsv1.Deployment
	// The current subset/consumer we're processing
	Subset riskifiedv1alpha1.Subset
	// StatusManager
	StatusManager *StatusManager
	// The matches used for this resource
	Matches []riskifiedv1alpha1.IstioMatch
}

// A struct to prevent masking one error with different (less important) one
type NonMaskingError struct {
	error error
}

// Only set the error if error is nil
func (err *NonMaskingError) SetIfNotMasking(e error) {
	if err.error == nil {
		err.error = e
	}
}

// Set error regardless on current error
func (err *NonMaskingError) ForceError(e error) {
	err.error = e
}

func (err *NonMaskingError) Get() error {
	return err.error
}

func (err *NonMaskingError) IsNil() bool {
	return err.error == nil
}

type SubsetType struct {
	Type   riskifiedv1alpha1.SubsetOrConsumer
	Subset riskifiedv1alpha1.Subset
}
