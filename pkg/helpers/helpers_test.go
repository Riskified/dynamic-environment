package helpers

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
)

func TestHelpers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Helpers Tests")
}

var _ = Describe("Helpers Tests", func() {
	Context("SerializationMatchExactHeaders", func() {

		var (
			multiple = map[string]riskifiedv1alpha1.StringMatch{
				"first-header":  {Exact: "value"},
				"anotherheader": {Exact: "another-value"},
			}
			empty = map[string]riskifiedv1alpha1.StringMatch{}
			mixed = map[string]riskifiedv1alpha1.StringMatch{
				"first-header":  {Exact: "value"},
				"anotherheader": {Prefix: "another-value"},
				"withregexp":    {Prefix: "a-z"},
			}
		)

		It("Concatenates multiple exact headers", func() {
			got := SerializeIstioMatchExactHeaders(multiple)
			possibleResults := []string{
				"first-header:value|anotherheader:another-value",
				"anotherheader:another-value|first-header:value",
			}
			Expect(possibleResults).To(ContainElement(got))
		})

		It("Returns empty string if empty headers provided", func() {
			got := SerializeIstioMatchExactHeaders(empty)
			Expect(got).To(BeEmpty())
		})

		It("Only serializes exact match and ignores other headers", func() {
			got := SerializeIstioMatchExactHeaders(mixed)
			Expect(got).To(Equal("first-header:value"))
		})
	})
})
