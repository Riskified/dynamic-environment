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

// IMPORTANT: While it's generally bad practice to test private functions, I do it here in order to
// avoid overly specified tests from the outside. If these test fails (e.g. we changed
// implementation) just delete the failing tests (of-course, make sure you test the new
// functionality :) ).

import (
	"encoding/json"
	"fmt"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Accepting alternative version label", func() {
	It("Handles custom version label correctly", func() {
		uniqueVersion := "unique-version"
		versionLabel := "version-label"
		deploymentData := `{
			  "apiVersion": "apps/v1",
			  "kind": "Deployment",
			  "metadata": {
				"name": "details",
				"namespace": "simple-test",
				"labels": {
				  "app": "details",
				  "version-label": "shared",
				  "version": "shared"
				}
			  },
			  "spec": {
				"replicas": 1,
				"selector": {
				  "matchLabels": {
					"app": "details",
					"version-label": "shared",
					"version": "shared"
				  }
				},
				"template": {
				  "metadata": {
					"labels": {
					  "app": "details",
					  "version-label": "shared",
					  "version": "shared"
					}
				  },
				  "spec": {
					"serviceAccountName": "bookinfo-details",
					"containers": [
					  {
						"name": "details",
						"image": "docker.io/istio/examples-bookinfo-details-v1:1.16.2",
						"imagePullPolicy": "IfNotPresent",
						"ports": [
						  {
							"containerPort": 9080
						  }
						],
						"securityContext": {
						  "runAsUser": 1000
						}
					  }
					]
				  }
				}
			  }
			}`
		deployment := v1.Deployment{}
		if err := json.Unmarshal([]byte(deploymentData), &deployment); err != nil {
			Fail(fmt.Sprintf("Could not parse deployment json: %s", err))
		}
		h := DeploymentHandler{
			Client:         nil,
			UniqueName:     "unique",
			UniqueVersion:  uniqueVersion,
			Owner:          types.NamespacedName{},
			BaseDeployment: &deployment,
			DeploymentType: 0,
			VersionLabel:   versionLabel,
			StatusManager:  nil,
			Matches:        nil,
			Updating:       false,
			Subset:         riskifiedv1alpha1.Subset{},
			Log:            logr.Logger{},
			Ctx:            nil,
		}
		result, err := h.createOverridingDeployment()
		Expect(err).To(BeNil())
		Expect(result.ObjectMeta.Labels[versionLabel]).To(Equal(uniqueVersion))
	})
})

var _ = Describe("Removing apecific labels from deployment", func() {

	deploymentData := `{
		  "apiVersion": "apps/v1",
		  "kind": "Deployment",
		  "metadata": {
			"name": "details",
			"namespace": "simple-test",
			"labels": {
			  "app": "details",
			  "version-label": "shared",
			  "version": "shared",
			  "argocd.argoproj.io/instance": "value",
			  "another/value-to-remove": "other-value"
			}
		  },
		  "spec": {
			"replicas": 1,
			"selector": {
			  "matchLabels": {
				"app": "details",
				"version-label": "shared",
				"version": "shared"
			  }
			},
			"template": {
			  "metadata": {
				"labels": {
				  "app": "details",
				  "version-label": "shared",
				  "version": "shared"
				}
			  },
			  "spec": {
				"serviceAccountName": "bookinfo-details",
				"containers": [
				  {
					"name": "details",
					"image": "docker.io/istio/examples-bookinfo-details-v1:1.16.2",
					"imagePullPolicy": "IfNotPresent",
					"ports": [
					  {
						"containerPort": 9080
					  }
					],
					"securityContext": {
					  "runAsUser": 1000
					}
				  }
				]
			  }
			}
		  }
		}`
	deployment := v1.Deployment{}
	if err := json.Unmarshal([]byte(deploymentData), &deployment); err != nil {
		Fail(fmt.Sprintf("Could not parse deployment json: %s", err))
	}

	getDefaultHanlder := func() DeploymentHandler {
		return DeploymentHandler{
			Client:         nil,
			UniqueName:     "unique",
			UniqueVersion:  "unique-version",
			Owner:          types.NamespacedName{},
			BaseDeployment: &deployment,
			DeploymentType: 0,
			LabelsToRemove: []string{},
			VersionLabel:   "version-label",
			StatusManager:  nil,
			Matches:        nil,
			Updating:       false,
			Subset:         riskifiedv1alpha1.Subset{},
			Log:            logr.Logger{},
			Ctx:            nil,
		}
	}

	Context("With no global labels to remove", func() {
		It("duplicates without removing labels", func() {
			handler := getDefaultHanlder()
			result, err := handler.createOverridingDeployment()
			Expect(err).To(BeNil())
			Expect(result.ObjectMeta.Labels).To(HaveKey("argocd.argoproj.io/instance"))
			Expect(result.ObjectMeta.Labels).To(HaveKey("another/value-to-remove"))
		})
	})

	Context("With globally configured labels to remove", func() {
		It("remove the configured labels", func() {
			handler := getDefaultHanlder()
			handler.LabelsToRemove = []string{
				"argocd.argoproj.io/instance",
				"another/value-to-remove",
				"yet-another/label-to-remove",
			}
			result, err := handler.createOverridingDeployment()
			Expect(err).To(BeNil())
			Expect(result.ObjectMeta.Labels).To(Not(HaveKey("argocd.argoproj.io/instance")))
			Expect(result.ObjectMeta.Labels).To(Not(HaveKey("another/value-to-remove")))
		})
	})
})
