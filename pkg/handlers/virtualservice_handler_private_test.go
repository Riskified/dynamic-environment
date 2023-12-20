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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	istionetwork "istio.io/client-go/pkg/apis/networking/v1alpha3"
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
