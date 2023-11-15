package helpers_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/riskified/dynamic-environment/pkg/helpers"
	v1 "k8s.io/api/core/v1"
)

var _ = Describe("Dynamic Environment Controller", func() {

	Context("Kubernetes Helpers", func() {
		Context("MergeEnvVars", func() {

			var (
				notOverridden = v1.EnvVar{Name: "ENV_FIVE", Value: "original_value5"}
				overrides     = []v1.EnvVar{
					{Name: "ENV_ONE", Value: "override_value1"},
					{Name: "ENV_TWO", Value: "override_value2"},
					{Name: "ENV_THREE", Value: "override_value3"},
					{Name: "ENV_FOUR", Value: "override_value4"},
				}
				current = []v1.EnvVar{
					{Name: "ENV_ONE", Value: "original_value1"},
					{Name: "ENV_TWO", Value: "original_value2"},
					{Name: "ENV_THREE", Value: "original_value3"},
					notOverridden,
				}
			)

			It("overrides with supplied environment variables", func() {
				r := helpers.MergeEnvVars(current, overrides)
				Expect(r).To(HaveLen(5), "Seems that not all environment variables were merged")
				for _, item := range overrides {
					Expect(helpers.EnvVarContains(r, item)).To(BeTrue(), fmt.Sprintf("override %v does not exist", item))
				}
			})

			It("retains values that are not overridden", func() {
				r := helpers.MergeEnvVars(current, overrides)
				Expect(r).To(HaveLen(5), "Seems that not all environment variables were merged")
				Expect(helpers.EnvVarContains(r, notOverridden)).To(BeTrue(), "should keep original value")
			})
		})
	})
})
