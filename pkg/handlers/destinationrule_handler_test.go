package handlers_test

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/pkg/handlers"
	"io"
	istionetwork "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func destinationRuleFromYaml(fileName string) (dr *istionetwork.DestinationRule, err error) {
	sourceFile, err := os.Open(fileName)
	if err != nil {
		return dr, fmt.Errorf("loading fixture file %q: %w", fileName, err)
	}
	data, err := io.ReadAll(sourceFile)
	if err != nil {
		return dr, fmt.Errorf("error reading data from file: %w", err)
	}
	if err := yaml.UnmarshalStrict(data, &dr); err != nil {
		return dr, fmt.Errorf("error strict unmarshaling fixture: %w", err)
	}
	return dr, nil
}

var _ = Describe("DestinationRuleHandler", func() {
	Context("GetStatus", func() {
		Context("Not ignored", func() {
			It("returns 'missing' if destination rule not found", func() {
				mc := struct{ MockClient }{}
				mc.getMethod = func(context.Context, types.NamespacedName, client.Object, ...client.GetOption) error {
					return errors.NewNotFound(schema.GroupResource{}, "error")
				}
				handler := handlers.DestinationRuleHandler{
					Client:       mc,
					UniqueName:   "unique",
					Namespace:    "ns",
					ServiceHosts: []string{"service"},
				}
				expected := []riskifiedv1alpha1.ResourceStatus{
					{
						Name:      "unique-service",
						Namespace: "ns",
						Status:    riskifiedv1alpha1.Missing,
					},
				}
				result, err := handler.GetStatus()
				Expect(err).To(BeNil())
				Expect(result).To(Equal(expected))
			})
		})
	})

	Context("Handle", func() {
		Context("missing base destination rules", func() {
			It("returns without error if at least one base destination rule is found", func() {
				mc := struct{ MockClient }{}
				mc.listMethod = func(_ context.Context, o client.ObjectList, _ ...client.ListOption) error {
					drList := o.(*istionetwork.DestinationRuleList)
					dr, err := destinationRuleFromYaml("fixtures/destination-rule-with-unrelated-hostname.yaml")
					Expect(err).To(BeNil())
					drList.Items = []*istionetwork.DestinationRule{dr}
					return nil
				}
				mc.getMethod = func(context.Context, types.NamespacedName, client.Object, ...client.GetOption) error {
					return errors.NewNotFound(schema.GroupResource{}, "error")
				}
				handler := handlers.DestinationRuleHandler{
					Client:       mc,
					UniqueName:   "unique",
					Namespace:    "ns",
					ServiceHosts: []string{"details", "service2"},
					StatusHandler: &handlers.DynamicEnvStatusHandler{
						Client:     mc,
						Ctx:        context.Background(),
						DynamicEnv: &riskifiedv1alpha1.DynamicEnv{},
					},
					Log: ctrl.Log,
				}
				err := handler.Handle()
				Expect(err).To(BeNil())
			})

			It("returns error if destination rules for all hosts were missing", func() {
				mc := struct{ MockClient }{}
				mc.listMethod = func(_ context.Context, o client.ObjectList, _ ...client.ListOption) error {
					drList := o.(*istionetwork.DestinationRuleList)
					dr, err := destinationRuleFromYaml("fixtures/destination-rule-with-unrelated-hostname.yaml")
					Expect(err).To(BeNil())
					drList.Items = []*istionetwork.DestinationRule{dr}
					return nil
				}
				mc.getMethod = func(context.Context, types.NamespacedName, client.Object, ...client.GetOption) error {
					return errors.NewNotFound(schema.GroupResource{}, "error")
				}
				handler := handlers.DestinationRuleHandler{
					Client:       mc,
					UniqueName:   "unique",
					Namespace:    "ns",
					ServiceHosts: []string{"service1", "service2"},
					StatusHandler: &handlers.DynamicEnvStatusHandler{
						Client:     mc,
						Ctx:        context.Background(),
						DynamicEnv: &riskifiedv1alpha1.DynamicEnv{},
					},
					Log: ctrl.Log,
				}
				err := handler.Handle()
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("no base destination rules"))
			})
		})
	})
})
