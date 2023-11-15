package v1alpha1_test

import (
	"encoding/json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
)

var _ = Describe("Life Cycle Status", func() {
	DescribeTable(
		"string representation of life cycle status",
		func(status riskifiedv1alpha1.LifeCycleStatus, s string) {
			rs := riskifiedv1alpha1.ResourceStatus{Status: status}
			Expect(rs.Status.String()).To(Equal(s))
		},
		Entry("unknown status", riskifiedv1alpha1.Unknown, "unknown"),
		Entry("initializing status", riskifiedv1alpha1.Initializing, "initializing"),
		Entry("running status", riskifiedv1alpha1.Running, "running"),
		Entry("failed status", riskifiedv1alpha1.Failed, "failed"),
		Entry("missing (not running)", riskifiedv1alpha1.Missing, "missing"),
		Entry("updating status", riskifiedv1alpha1.Updating, "updating"),
		Entry("ignored missing destination rule", riskifiedv1alpha1.IgnoredMissingDR, "ignored-missing-destination-rule"),
		Entry("ignored missing virtual service", riskifiedv1alpha1.IgnoredMissingVS, "ignored-missing-virtual-service"),
	)

	It("invalid status produces unknown", func() {
		rs := riskifiedv1alpha1.ResourceStatus{Status: "invalid"}
		Expect(rs.Status.String()).To(Equal("unknown"))
	})

	Context("Converting to/from Json", func() {
		It("converts invalid status to unknown when converting to json", func() {
			rs := riskifiedv1alpha1.ResourceStatus{
				Name:      "name",
				Namespace: "namespace",
				Status:    "invalid",
			}
			result, err := json.Marshal(rs)
			Expect(err).To(BeNil())
			Expect(string(result)).To(ContainSubstring("unknown"))
		})

		It("converts invalid status to unknown when converting to json", func() {
			data := []byte(`{"name":"name","namespace":"namespace","status":"invalid"}`)
			result := &riskifiedv1alpha1.ResourceStatus{}
			err := json.Unmarshal(data, result)
			Expect(err).To(BeNil())
			Expect(result.Status).To(Equal(riskifiedv1alpha1.Unknown))
		})
	})
})

var _ = Describe("Global Ready Status", func() {

	It("Default status is processing", func() {
		st := riskifiedv1alpha1.DynamicEnvStatus{
			State: "invalid",
		}
		Expect(st.State.String()).To(Equal("processing"))
	})

	Context("converting to/from JSON", func() {
		It("converts invalid state to processing when converting to JSON", func() {
			st := &riskifiedv1alpha1.DynamicEnvStatus{
				State: "invalid",
			}
			result, err := json.Marshal(st)
			Expect(err).To(BeNil())
			Expect(result).To(ContainSubstring("processing"))
		})

		It("converts invalid state to processing when converting from JSON", func() {
			data := []byte(`{"state":"processing"}`)
			result := &riskifiedv1alpha1.DynamicEnvStatus{}
			err := json.Unmarshal(data, result)
			Expect(err).To(BeNil())
			Expect(result.State).To(Equal(riskifiedv1alpha1.Processing))
		})

		DescribeTable(
			"converts valid values correctly",
			func(status riskifiedv1alpha1.GlobalReadyStatus, s string) {
				st := &riskifiedv1alpha1.DynamicEnvStatus{
					State: status,
				}
				data, err := json.Marshal(st)
				Expect(err).To(BeNil())
				Expect(data).To(ContainSubstring(s))
				st2 := &riskifiedv1alpha1.DynamicEnvStatus{}
				err = json.Unmarshal(data, st2)
				Expect(err).To(BeNil())
				Expect(st2.State).To(Equal(status))
			},
			Entry("degraded status", riskifiedv1alpha1.Degraded, "degraded"),
			Entry("processing status", riskifiedv1alpha1.Processing, "processing"),
			Entry("ready status", riskifiedv1alpha1.Ready, "ready"),
		)
	})

	DescribeTable(
		"IsFailedStatus",
		func(s riskifiedv1alpha1.LifeCycleStatus, expected bool) {
			Expect(s.IsFailedStatus()).To(Equal(expected))
		},
		Entry("initializing is not failed", riskifiedv1alpha1.Initializing, false),
		Entry("updating is not failed", riskifiedv1alpha1.Updating, false),
		Entry("running is not failed", riskifiedv1alpha1.Running, false),
		Entry("unknown is not failed", riskifiedv1alpha1.Unknown, false),
		Entry("ignored missing DR is not failed", riskifiedv1alpha1.IgnoredMissingDR, false),
		Entry("ignored missing VS is not failed", riskifiedv1alpha1.IgnoredMissingVS, false),
		Entry("missing is failed", riskifiedv1alpha1.Missing, true),
		Entry("failed is failed", riskifiedv1alpha1.Failed, true),
	)
})
