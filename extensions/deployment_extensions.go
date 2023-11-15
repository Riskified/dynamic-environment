package extensions

import (
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
)

// DeploymentExtensionData contains handler context data to be passed to the extension methods that
// might be required by the extension method.
type DeploymentExtensionData struct {
	// The deployment we should use as base
	BaseDeployment *appsv1.Deployment
	// The DynamicEnv matchers
	Matches []riskifiedv1alpha1.IstioMatch
	// The subset that contains the data of our deployment
	Subset riskifiedv1alpha1.Subset
}

// ExtendOverridingDeployment is a function that could be used to customize the overriding
// deployment. Optionally using the `DeploymentExtensionData` data you can overwrite parts of the
// `deployment` ref.
func ExtendOverridingDeployment(deployment *appsv1.Deployment, data DeploymentExtensionData) error {

	return nil
}
