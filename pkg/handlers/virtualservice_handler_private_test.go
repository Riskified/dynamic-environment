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
	"context"
	"encoding/json"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/pkg/helpers"
	"istio.io/api/networking/v1alpha3"
	istionetwork "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	istioapi "istio.io/api/networking/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("VirtualService with Delegates", func() {

	const HOST1 = "my-host"
	//const HOST2 = "my-other-host"

	mkVirtualServiceHandler := func(name, ns string, c client.Client) VirtualServiceHandler {
		return VirtualServiceHandler{
			Client:         c,
			UniqueName:     name,
			Namespace:      ns,
			DefaultVersion: "shared",
			Log:            ctrl.Log,
		}
	}

	DescribeTable(
		"Resolving VirtualServices and Delegates",
		func(
			serviceHost string,
			data string,
			expectedDelegates []*istioapi.Delegate,
			expectedSelf bool,
		) {
			mc := struct{ client.Client }{}
			handler := mkVirtualServiceHandler("handler", "my-ns", &mc)
			handler.ServiceHosts = []string{serviceHost}
			vs := istionetwork.VirtualService{}
			if err := json.Unmarshal([]byte(data), &vs); err != nil {
				Fail(fmt.Sprintf("Error unmarshaling data: %s", err))
			}
			s, ds := handler.resolveVirtualServices(serviceHost, &vs)
			Expect(ds).To(HaveLen(len(expectedDelegates)))
			for idx, delegate := range expectedDelegates {
				Expect(delegate.Name).To(Equal(ds[idx].Name), "delegate name of index %d (%s) not equal to expected (%s)", idx, ds[idx].Name, delegate.Name)
				Expect(delegate.Namespace).To(Equal(ds[idx].Namespace), "delegate ns of index %d (%s) not equal to expected (%s)", idx, ds[idx].Namespace, delegate.Namespace)
			}
			Expect(s).To(Equal(expectedSelf))
		},
		Entry(
			"virtual service without delegates",
			HOST1,
			`{
			  "apiVersion": "networking.istio.io/v1alpha3",
			  "kind": "VirtualService",
			  "metadata": {
				"name": "my-name",
				"namespace": "my-ns"
			  },
			  "spec": {
				"hosts": [
				  "my-host"
				],
				"http": [
				  {
					"route": [
					  {
						"destination": {
						  "host": "my-host",
						  "subset": "shared"
						}
					  }
					]
				  }
				]
			  }
			}`,
			nil,
			true,
		),
		Entry(
			"virtual service with only delegates",
			HOST1,
			`{
			  "apiVersion": "networking.istio.io/v1alpha3",
			  "kind": "VirtualService",
			  "metadata": {
				"name": "my-name",
				"namespace": "my-ns"
			  },
			  "spec": {
				"hosts": [
				  "my-host",
				  "my-other-host"
				],
				"http": [
				  {
					"match": [
					  {
						"uri": null,
						"prefix": "/reviews"
					  }
					],
					"delegate": {
					  "name": "vs1",
					  "namespace": "my-ns"
					}
				  },
				  {
					"match": [
					  {
						"uri": null,
						"prefix": "/details"
					  }
					],
					"delegate": {
					  "name": "vs2",
					  "namespace": "my-ns"
					}
				  }
				]
			  }
			}`,
			[]*istioapi.Delegate{
				{
					Name:      "vs1",
					Namespace: "my-ns",
				},
				{
					Name:      "vs2",
					Namespace: "my-ns",
				},
			},
			false,
		),
		Entry(
			"virtual service with both delegates and local routes",
			HOST1,
			`{
			  "apiVersion": "networking.istio.io/v1alpha3",
			  "kind": "VirtualService",
			  "metadata": {
				"name": "my-name",
				"namespace": "my-ns"
			  },
			  "spec": {
				"hosts": [
				  "my-host",
				  "my-other-host"
				],
				"http": [
				  {
					"match": [
					  {
						"uri": null,
						"prefix": "/reviews"
					  }
					],
					"delegate": {
					  "name": "vs1",
					  "namespace": "my-ns"
					}
				  },
				  {
					"match": [
					  {
						"uri": null,
						"prefix": "/details"
					  }
					],
					"delegate": {
					  "name": "vs2",
					  "namespace": "my-ns"
					}
				  },
				  {
					"route": [
					  {
						"destination": {
						  "host": "my-host",
						  "subset": "shared"
						}
					  }
					]
				  }
				]
			  }
			}`,
			[]*istioapi.Delegate{
				{
					Name:      "vs1",
					Namespace: "my-ns",
				},
				{
					Name:      "vs2",
					Namespace: "my-ns",
				},
			},
			true,
		),
	)
})

var _ = Describe("VirtualServiceHandler", func() {
	Context("GetStatus", func() {
		mkVirtualServiceHandler := func(name, ns, version, prefix string, c client.Client) VirtualServiceHandler {
			return VirtualServiceHandler{
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
			// TODO: How do we behave here? It should probably return error...
			Skip("We need to think this over")
			mc := struct{ MockClient }{}
			handler := mkVirtualServiceHandler("unique", "ns", "_version", "_prefix", mc)
			var expected []riskifiedv1alpha1.ResourceStatus
			result, err := handler.GetStatus()
			Expect(err).To(BeNil())
			Expect(result).To(Equal(expected))
		})

		It("returns 'ignored-missing-virtual-service' for virtual services that does not contain our routes", func() {
			serviceName := "my-service-host"
			mc := struct{ MockClient }{}
			mc.getMethod = func(_ context.Context, nn types.NamespacedName, o client.Object, _ ...client.GetOption) error {
				if nn.Name == "service" && nn.Namespace == "ns" {
					t := o.(*istionetwork.VirtualService)
					t.Name = "service"
					t.Namespace = "ns"
					t.Spec.Hosts = []string{serviceName}
					t.Spec.Http = []*v1alpha3.HTTPRoute{
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
					return nil
				} else {
					return errors.NewNotFound(schema.GroupResource{}, "test did not request correct name")
				}
			}
			version := "unique-version"
			prefix := helpers.CalculateVirtualServicePrefix(version, "subset")
			handler := mkVirtualServiceHandler("unique", "ns", version, prefix, mc)
			handler.ServiceHosts = []string{"my-service-host"}
			handler.UniqueVersion = "unique-version"
			vss := []types.NamespacedName{
				{Name: "service", Namespace: "ns"},
			}
			handler.virtualServices = make(map[string][]types.NamespacedName)
			handler.virtualServices["my-service-host"] = vss
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
			mc.getMethod = func(_ context.Context, nn types.NamespacedName, o client.Object, _ ...client.GetOption) error {
				if nn.Name == "service" && nn.Namespace == "ns" {
					t := o.(*istionetwork.VirtualService)
					t.Name = "service"
					t.Namespace = "ns"
					t.Spec.Hosts = []string{serviceName}
					t.Spec.Http = []*v1alpha3.HTTPRoute{
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
					return nil
				} else {
					return errors.NewNotFound(schema.GroupResource{}, "test did not request correct name")
				}
			}
			handler := mkVirtualServiceHandler("unique", "ns", uniqueVersion, prefix, mc)
			handler.ServiceHosts = []string{"my-service-host"}
			handler.UniqueVersion = uniqueVersion
			vss := []types.NamespacedName{
				{Name: "service", Namespace: "ns"},
			}
			handler.virtualServices = make(map[string][]types.NamespacedName)
			handler.virtualServices["my-service-host"] = vss
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
