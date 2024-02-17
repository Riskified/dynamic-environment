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

package handlers_test

import (
	"context"
	"fmt"
	"github.com/riskified/dynamic-environment/pkg/helpers"
	"github.com/riskified/dynamic-environment/pkg/model"
	"github.com/riskified/dynamic-environment/pkg/names"
	"io"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/pkg/handlers"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func mkDynamicEnvFromYamlFile(fileName string) (de riskifiedv1alpha1.DynamicEnv, err error) {
	sourceFile, err := os.Open(fileName)
	if err != nil {
		return de, fmt.Errorf("error opening fixture: %w", err)
	}
	data, err := io.ReadAll(sourceFile)
	if err != nil {
		return de, fmt.Errorf("error reading data from file: %w", err)
	}
	if err := yaml.UnmarshalStrict(data, &de); err != nil {
		return de, fmt.Errorf("error strict unmarshaling fixture: %w", err)
	}
	return de, nil
}

var _ = Describe("DeploymentHandler", func() {
	Context("GetStatus", func() {

		mkExpected := func(h handlers.DeploymentHandler, s riskifiedv1alpha1.LifeCycleStatus) riskifiedv1alpha1.ResourceStatus {
			return riskifiedv1alpha1.ResourceStatus{
				Name:      h.UniqueName,
				Namespace: h.Subset.Namespace,
				Status:    s,
			}
		}

		mkDeploymentHandler := func(name, ns string, c client.Client) handlers.DeploymentHandler {
			return handlers.DeploymentHandler{
				Client:     c,
				UniqueName: name,
				Subset: riskifiedv1alpha1.Subset{
					Namespace: ns,
				},
				VersionLabel: names.DefaultVersionLabel,
				Log:          helpers.MkLogger("TestsLogger"),
			}
		}

		Context("When Deployment does not exist", func() {
			It("Returns missing life cycle status", func() {
				mc := struct{ MockClient }{}
				mc.getMethod = func(context.Context, types.NamespacedName, client.Object, ...client.GetOption) error {
					return errors.NewNotFound(schema.GroupResource{}, "error")
				}
				handler := mkDeploymentHandler("unique", "my-namespace", mc)
				expected := mkExpected(handler, riskifiedv1alpha1.Missing)
				result, _ := handler.GetStatus(context.Background())
				Expect(result).To(Equal(expected))
			})

		})

		Context("When Deployment is available", func() {
			It("reports status as running if processing and reason is NewReplicaSetAvailable", func() {
				mc := struct{ MockClient }{}
				mc.getMethod = func(_ context.Context, _ types.NamespacedName, o client.Object, _ ...client.GetOption) error {
					t := o.(*appsv1.Deployment)
					t.Status.AvailableReplicas = 1
					t.Status.Conditions = []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentProgressing,
							Status: v1.ConditionTrue,
							Reason: "NewReplicaSetAvailable",
						},
					}
					return nil
				}
				handler := mkDeploymentHandler("unique", "my-namespace", mc)
				expected := mkExpected(handler, riskifiedv1alpha1.Running)
				result, _ := handler.GetStatus(context.Background())
				Expect(result).To(Equal(expected))
			})

			It("reports status as initializing if processing=true and reason is not NewReplicaSetAvailable", func() {
				mc := struct{ MockClient }{}
				mc.getMethod = func(_ context.Context, _ types.NamespacedName, o client.Object, _ ...client.GetOption) error {
					t := o.(*appsv1.Deployment)
					t.Status.AvailableReplicas = 1
					t.Status.Conditions = []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentProgressing,
							Status: v1.ConditionTrue,
						},
					}
					return nil
				}
				handler := mkDeploymentHandler("unique", "my-namespace", mc)
				expected := mkExpected(handler, riskifiedv1alpha1.Initializing)
				result, _ := handler.GetStatus(context.Background())
				Expect(result).To(Equal(expected))
			})
		})

		Context("When Deployment is not available", func() {
			It("reports status as initializing if Processing=True", func() {
				mc := struct{ MockClient }{}
				mc.getMethod = func(_ context.Context, _ types.NamespacedName, o client.Object, _ ...client.GetOption) error {
					t := o.(*appsv1.Deployment)
					t.Status.Conditions = []appsv1.DeploymentCondition{
						{
							Type:    appsv1.DeploymentProgressing,
							Status:  v1.ConditionTrue,
							Reason:  "NewReplicaSetCreated",
							Message: "Created new replica Set ...",
						},
						{
							Type:    appsv1.DeploymentAvailable,
							Status:  v1.ConditionFalse,
							Reason:  "MinimumReplicasUnavailable",
							Message: "Deployment does not have minimum ...",
						},
					}
					return nil
				}
				handler := mkDeploymentHandler("unavailable1", "my-namespace", mc)
				expected := mkExpected(handler, riskifiedv1alpha1.Initializing)
				result, _ := handler.GetStatus(context.Background())
				Expect(result).To(Equal(expected))
			})

			It("reports status as failure if Processing=False", func() {
				mc := struct{ MockClient }{}
				mc.getMethod = func(_ context.Context, _ types.NamespacedName, o client.Object, _ ...client.GetOption) error {
					t := o.(*appsv1.Deployment)
					t.Status.Conditions = []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentProgressing,
							Status: v1.ConditionFalse,
						},
					}
					return nil
				}
				handler := mkDeploymentHandler("unique", "my-namespace", mc)
				expected := mkExpected(handler, riskifiedv1alpha1.Failed)
				result, _ := handler.GetStatus(context.Background())
				Expect(result).To(Equal(expected))
			})

			// TODO: Do we need to test of ReplicaFailure condition?
		})
	})

	Context("UpdateIfRequired", func() {

		mkDeploymentHandler := func(uniqueName string, de riskifiedv1alpha1.DynamicEnv, c client.Client) handlers.DeploymentHandler {
			return handlers.DeploymentHandler{
				Client:       c,
				UniqueName:   uniqueName,
				VersionLabel: names.DefaultVersionLabel,
				StatusManager: &model.StatusManager{
					Client:     c,
					DynamicEnv: &de,
				},
				Log: ctrl.Log,
			}
		}

		It("Matches correctly hashes of unmodified subsets", func() {
			de, err := mkDynamicEnvFromYamlFile("fixtures/simple-dynamicenv.yaml")
			if err != nil {
				Fail(err.Error())
			}
			mc := struct{ MockClient }{}
			handler := mkDeploymentHandler("details-default-dynamicenv-sample", de, mc)
			handler.SubsetName = fmt.Sprintf("%s/%s", de.Spec.Subsets[0].Namespace, de.Spec.Subsets[0].Name)
			subset := de.Spec.Subsets[0].DeepCopy()
			handler.Subset = *subset
			errorResult := handler.UpdateIfRequired(context.Background())
			Expect(errorResult).To(BeNil())
			Expect(handler.Updating).To(BeFalse())
		})
	})
})
