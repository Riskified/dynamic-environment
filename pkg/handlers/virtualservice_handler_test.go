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

	ctrl "sigs.k8s.io/controller-runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/pkg/handlers"
	"github.com/riskified/dynamic-environment/pkg/helpers"
	"istio.io/api/networking/v1alpha3"
	istionetwork "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("VirtualServiceHandler", func() {
	Context("GetStatus", func() {
		mkVirtualServiceHandler := func(name, ns, version, prefix string, c client.Client) handlers.VirtualServiceHandler {
			return handlers.VirtualServiceHandler{
				Client:         c,
				UniqueName:     name,
				Namespace:      ns,
				UniqueVersion:  version,
				DefaultVersion: "shared",
				RoutePrefix:    prefix,
				Log:            ctrl.Log,
			}
		}

		It("returns empty result if no virtual service found", func() {
			mc := struct{ MockClient }{}
			mc.listMethod = func(context.Context, client.ObjectList, ...client.ListOption) error {
				return nil
			}
			handler := mkVirtualServiceHandler("unique", "ns", "_version", "_prefix", &mc)
			var expected []riskifiedv1alpha1.ResourceStatus
			result, err := handler.GetStatus()
			Expect(err).To(BeNil())
			Expect(result).To(Equal(expected))
		})

		It("returns 'ignored-missing-virtual-service' for virtual services that does not contain our routes", func() {
			serviceName := "my-service-host"
			mc := struct{ MockClient }{}
			mc.listMethod = func(_ context.Context, o client.ObjectList, _ ...client.ListOption) error {
				serviceResult := istionetwork.VirtualService{}
				serviceResult.Name = "service"
				serviceResult.Namespace = "ns"
				serviceResult.Spec.Hosts = []string{serviceName}
				serviceResult.Spec.Http = []*v1alpha3.HTTPRoute{
					{
						Route: []*v1alpha3.HTTPRouteDestination{
							{
								Destination: &v1alpha3.Destination{
									Host:   serviceName,
									Subset: "shared",
								},
							},
						},
					},
				}
				services := o.(*istionetwork.VirtualServiceList)
				services.Items = []*istionetwork.VirtualService{
					&serviceResult,
				}
				return nil
			}
			version := "unique-version"
			prefix := helpers.CalculateVirtualServicePrefix(version, "subset")
			handler := mkVirtualServiceHandler("unique", "ns", version, prefix, mc)
			handler.ServiceHosts = []string{"my-service-host"}
			handler.UniqueVersion = "unique-version"
			expected := []riskifiedv1alpha1.ResourceStatus{
				{
					Name:      "service",
					Namespace: "ns",
					Status:    riskifiedv1alpha1.IgnoredMissingVS,
				},
			}
			result, err := handler.GetStatus()
			Expect(err).To(BeNil())
			Expect(result).To(Equal(expected))
		})

		It("returns 'running' for every service that contains our routing", func() {
			serviceName := "my-service-host"
			uniqueVersion := "unique-version"
			prefix := helpers.CalculateVirtualServicePrefix(uniqueVersion, "subset")
			mc := struct{ MockClient }{}
			mc.listMethod = func(_ context.Context, o client.ObjectList, _ ...client.ListOption) error {
				serviceResult := istionetwork.VirtualService{}
				serviceResult.Name = "service"
				serviceResult.Namespace = "ns"
				serviceResult.Spec.Hosts = []string{serviceName}
				serviceResult.Spec.Http = []*v1alpha3.HTTPRoute{
					{
						Name: prefix + "abscdsdlkfj",
						Route: []*v1alpha3.HTTPRouteDestination{
							{
								Destination: &v1alpha3.Destination{
									Host:   serviceName,
									Subset: "shared",
								},
							},
						},
					},
				}
				services := o.(*istionetwork.VirtualServiceList)
				services.Items = []*istionetwork.VirtualService{
					&serviceResult,
				}
				return nil
			}
			handler := mkVirtualServiceHandler("unique", "ns", uniqueVersion, prefix, &mc)
			handler.ServiceHosts = []string{"my-service-host"}
			handler.UniqueVersion = uniqueVersion
			expected := []riskifiedv1alpha1.ResourceStatus{
				{
					Name:      "service",
					Namespace: "ns",
					Status:    riskifiedv1alpha1.Running,
				},
			}
			result, err := handler.GetStatus()
			Expect(err).To(BeNil())
			Expect(result).To(Equal(expected))
		})
	})
})
