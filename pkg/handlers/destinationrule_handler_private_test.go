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

// IMPORTANT: While it's generally bad practice to test private functions, I do it here to avoid overly specified tests
// from the outside. If these tests fail (e.g., we changed implementation) delete the failing tests (make sure you test
// the new functionality :) ).

import (
	"context"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	istioapi "istio.io/api/networking/v1alpha3"
	istionetwork "istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Accepting alternative version label and version", func() {
	It("Handles custom version label correctly", func() {
		versionLabel := "version-label"
		version := "custom-version-value"
		mc := struct{ MockClient }{}
		serviceName := "service-name"
		mc.listMethod = func(_ context.Context, drs client.ObjectList, _ ...client.ListOption) error {
			drs.(*istionetwork.DestinationRuleList).Items = []*istionetwork.DestinationRule{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service",
						Namespace: "namespace",
					},
					Spec: istioapi.DestinationRule{
						Host: serviceName,
						Subsets: []*istioapi.Subset{
							{
								Name: versionLabel,
								Labels: map[string]string{
									versionLabel: version,
								},
							},
						},
					},
				},
			}
			return nil
		}
		h := DestinationRuleHandler{
			Client:         mc,
			UniqueName:     "unique-name",
			UniqueVersion:  "unique-version",
			Namespace:      "namespace",
			VersionLabel:   versionLabel,
			DefaultVersion: version,
			StatusManager:  nil,
			ServiceHosts:   []string{serviceName},
			Owner:          types.NamespacedName{},
			Log:            logr.Logger{},
		}
		dr, err := h.generateOverridingDestinationRule(context.Background(), serviceName)
		Expect(err).To(BeNil())
		Expect(dr).NotTo(BeNil())
	})
})
