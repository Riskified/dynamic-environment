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
	"fmt"
	"k8s.io/utils/strings/slices"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var dynamicenvlog = logf.Log.WithName("dynamicenv-resource")

func (de *DynamicEnv) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(de).
		Complete()
}

//+kubebuilder:webhook:path=/validate-riskified-com-v1alpha1-dynamicenv,mutating=false,failurePolicy=fail,sideEffects=None,groups=riskified.com,resources=dynamicenvs,verbs=create;update,versions=v1alpha1,name=vdynamicenv.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &DynamicEnv{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (de *DynamicEnv) ValidateCreate() (admission.Warnings, error) {
	dynamicenvlog.Info("validate create", "name", de.Name)

	if err := de.validateIstioMatchAnyOf(); err != nil {
		return nil, err
	}
	if err := de.validateSubsetsProperties(); err != nil {
		return nil, err
	}

	return nil, de.validateStringMatchOneOf()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (de *DynamicEnv) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	dynamicenvlog.Info("validate update", "name", de.Name)

	if err := de.validateIstioMatchImmutable(old); err != nil {
		return nil, err
	}
	if err := de.validateSubsetsProperties(); err != nil {
		return nil, err
	}
	return nil, de.validatePartialUpdateSubsets(old)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (de *DynamicEnv) ValidateDelete() (admission.Warnings, error) {
	dynamicenvlog.Info("validate delete", "name", de.Name)

	return nil, nil
}

// validateStringMatchOneOf must validate one and only one schema (oneOf) of StringMatch is defined
func (de *DynamicEnv) validateStringMatchOneOf() error {
	var allErrs field.ErrorList
	for _, m := range de.Spec.IstioMatches {
		for header, match := range m.Headers {
			fields := reflect.ValueOf(match)
			var alternatives int
			for i := 0; i < fields.NumField(); i++ {
				if fields.Field(i).Interface() != "" {
					alternatives++
				}
			}

			if alternatives != 1 {
				errDetail := fmt.Sprintf("spec.istioMatch.headers.%s must validate one and only one schema (oneOf). Found %d valid alternatives", header, alternatives)
				path := field.NewPath("spec").Child("istioMatch").Child("headers").Child(header)
				allErrs = append(allErrs, field.Invalid(path, "header", errDetail))
			}
		}
	}

	if len(allErrs) != 0 {
		return errors.NewInvalid(
			schema.GroupKind{Group: "dynamicenv.riskified.com", Kind: "DynamicEnv"},
			de.Name, allErrs)
	}

	return nil
}

// validateIstioMatchAnyOf validates at least one field from IstioMatch is defined
func (de *DynamicEnv) validateIstioMatchAnyOf() error {
	errDetail := "empty IstioMatch is invalid for DynamicEnv"
	if len(de.Spec.IstioMatches) == 0 {
		return field.Invalid(field.NewPath("spec"), de.Spec.IstioMatches, errDetail)
	}
	for _, match := range de.Spec.IstioMatches {
		if len(match.Headers) == 0 && len(match.SourceLabels) == 0 {
			return field.Invalid(field.NewPath("spec"), de.Spec.IstioMatches, errDetail)
		}
	}

	return nil
}

// Validates certain aspects of the subset. Should be used both on creation and update.
func (de *DynamicEnv) validateSubsetsProperties() error {
	subsets := append(de.Spec.Subsets, de.Spec.Consumers...)
	for _, s := range subsets {
		if s.Replicas != nil && *s.Replicas == 0 {
			msg := "It's illegal to use 0 replicas!"
			return field.Invalid(field.NewPath("spec"), de.Spec, msg)
		}
		if len(s.Containers) == 0 && len(s.InitContainers) == 0 {
			msg := "At least a single container or init-container must be specified"
			return field.Invalid(field.NewPath("spec").Key(s.Name), s, msg)
		}
		if !validateUniqueContainerNames(s.Containers) {
			msg := "It seems that not all container names are unique"
			return field.Invalid(field.NewPath("spec").Child("Subsets").Key(s.Name).Child("Containers"), s.Containers, msg)
		}
		if !validateUniqueContainerNames(s.InitContainers) {
			msg := "It seems that not all container names are unique"
			return field.Invalid(field.NewPath("spec").Child("Subsets").Key(s.Name).Child("initContainers"), s.InitContainers, msg)
		}
	}
	if err := validateNoDuplicateDeployment(subsets); err != nil {
		msg := "It seems that the following deployment appears in more then one subset/consumer: " + err.Error()
		return field.Invalid(field.NewPath("spec"), de.Spec, msg)
	}
	return nil
}

// validateIstioMatchImmutable validates IstioMatch is immutable after creation.
func (de *DynamicEnv) validateIstioMatchImmutable(old runtime.Object) error {
	oldIstioMatch := old.(*DynamicEnv).Spec.IstioMatches
	newIstioMatch := de.Spec.IstioMatches

	if !reflect.DeepEqual(newIstioMatch, oldIstioMatch) {
		errDetail := "spec.istioMatch field is immutable"
		return field.Invalid(field.NewPath("spec"), newIstioMatch, errDetail)
	}

	return nil
}

// validatePartialUpdateSubsets verifies that update only occurs within a subset. The name/namespace
// of the subsets should not be updated (e.g., should not delete or create new subsets).
func (de *DynamicEnv) validatePartialUpdateSubsets(old runtime.Object) error {
	oldSubsets := append(old.(*DynamicEnv).Spec.Subsets, old.(*DynamicEnv).Spec.Consumers...)
	newSubsets := append(de.Spec.Subsets, de.Spec.Consumers...)
	for _, subset := range oldSubsets {
		if err := validateSubsetModifications(subset, newSubsets); err != nil {
			return err
		}
	}
	return nil
}

func validateSubsetModifications(old Subset, subsets []Subset) error {
	for _, s := range subsets {
		if old.Name == s.Name && old.Namespace == s.Namespace {
			if err := compareContainers(old.Containers, s.Containers); err != nil {
				desc := fmt.Sprintf("couldn't find matching container to existing container: %s", err.Error())
				return field.Invalid(
					field.NewPath("spec").Child("Subsets").Key(s.Name).Child("Containers"),
					s.Containers, desc)
			}
			if err := compareContainers(old.InitContainers, s.InitContainers); err != nil {
				desc := fmt.Sprintf("couldn't find matching container to existing container: %s", err.Error())
				return field.Invalid(
					field.NewPath("spec").Child("Subsets").Key(s.Name).Child("initContainers"),
					s.InitContainers, desc)
			}
			if old.DefaultVersion != s.DefaultVersion {
				desc := fmt.Sprintf(
					"Changing default-version is not supported (modified '%s' to '%s')", old.DefaultVersion, s.DefaultVersion,
				)
				return field.Invalid(field.NewPath("spec").Child("Subsets"), subsets, desc)
			}
			break
		}
	}
	return nil
}

func compareContainers(oldCOntainers, newContainers []ContainerOverrides) error {
	if len(oldCOntainers) != len(newContainers) {
		return fmt.Errorf("number of new containers (%d) does not match the number of old containers (%d)",
			len(oldCOntainers), len(newContainers))
	}
	for _, c := range oldCOntainers {
		if err := findMatchingContainer(c, newContainers); err != nil {
			return err
		}
	}
	return nil
}

func findMatchingContainer(oldContainer ContainerOverrides, containers []ContainerOverrides) error {
	var foundMatching = false

	for _, c := range containers {
		if c.ContainerName == oldContainer.ContainerName {
			foundMatching = true
		}
	}

	if !foundMatching {
		return fmt.Errorf("%s", oldContainer.ContainerName)
	}
	return nil
}

func validateUniqueContainerNames(containers []ContainerOverrides) bool {
	// I really hate go for not providing helpers for doing such things
	m := make(map[string]bool)
	for _, c := range containers {
		m[c.ContainerName] = true
	}
	return len(m) == len(containers)
}

func validateNoDuplicateDeployment(subsets []Subset) error {
	var uniques []string
	for _, s := range subsets {
		combined := fmt.Sprintf("%s/%s", s.Namespace, s.Name)
		if slices.Contains(uniques, combined) {
			return fmt.Errorf("%s", combined)
		}
		uniques = append(uniques, combined)
	}
	return nil
}
