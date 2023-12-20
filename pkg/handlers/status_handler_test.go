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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"

	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	"github.com/riskified/dynamic-environment/pkg/handlers"
)

var _ = Describe("SyncStatusResources", func() {
	defaultResources := []riskifiedv1alpha1.ResourceStatus{
		{
			Name:      "event1",
			Namespace: "ns",
			Status:    riskifiedv1alpha1.Unknown,
		},
		{
			Name:      "event2",
			Namespace: "ns",
			Status:    riskifiedv1alpha1.Unknown,
		},
	}

	DescribeTable(
		"correctly handles status sync",
		func(
			s riskifiedv1alpha1.ResourceStatus,
			resources []riskifiedv1alpha1.ResourceStatus,
			modified bool,
			expected []riskifiedv1alpha1.ResourceStatus,
		) {
			m, result := handlers.SyncStatusResources(s, resources)
			Expect(result).To(Equal(expected))
			Expect(m).To(Equal(modified))
		},
		Entry(
			"when status already synced",
			riskifiedv1alpha1.ResourceStatus{
				Name:      "event1",
				Namespace: "ns",
				Status:    riskifiedv1alpha1.Unknown,
			},
			defaultResources,
			false,
			defaultResources,
		),
		Entry(
			"when provided resource exists by name but status is different",
			riskifiedv1alpha1.ResourceStatus{
				Name:      "event1",
				Namespace: "ns",
				Status:    riskifiedv1alpha1.Initializing,
			},
			defaultResources,
			true,
			[]riskifiedv1alpha1.ResourceStatus{
				{
					Name:      "event1",
					Namespace: "ns",
					Status:    riskifiedv1alpha1.Initializing,
				},
				{
					Name:      "event2",
					Namespace: "ns",
					Status:    riskifiedv1alpha1.Unknown,
				},
			},
		),
		Entry(
			"when provided status does not exists",
			riskifiedv1alpha1.ResourceStatus{
				Name:      "event2",
				Namespace: "ns",
				Status:    riskifiedv1alpha1.Running,
			},
			[]riskifiedv1alpha1.ResourceStatus{
				{
					Name:      "event1",
					Namespace: "ns",
					Status:    riskifiedv1alpha1.Initializing,
				},
			},
			true,
			[]riskifiedv1alpha1.ResourceStatus{
				{
					Name:      "event1",
					Namespace: "ns",
					Status:    riskifiedv1alpha1.Initializing,
				},
				{
					Name:      "event2",
					Namespace: "ns",
					Status:    riskifiedv1alpha1.Running,
				},
			},
		),
		Entry(
			"when current resources is empty",
			riskifiedv1alpha1.ResourceStatus{
				Name:      "event1",
				Namespace: "ns",
				Status:    riskifiedv1alpha1.Initializing,
			},
			nil,
			true,
			[]riskifiedv1alpha1.ResourceStatus{
				{
					Name:      "event1",
					Namespace: "ns",
					Status:    riskifiedv1alpha1.Initializing,
				},
			},
		),
	)
	// do something
})

var _ = Describe("SyncGlobalErrors", func() {

	findErrorIndex := func(msg string, errors []riskifiedv1alpha1.StatusError) int {
		for i, e := range errors {
			if e.Error == msg {
				return i
			}
		}
		panic("error not found in error list")
	}

	Context("Empty global error list", func() {
		It("Adds provided error", func() {
			now := metav1.Now()
			time.Sleep(200 * time.Microsecond) // force after
			var currentErrors []riskifiedv1alpha1.StatusError
			msg := "This is an error"
			result := handlers.SyncGlobalErrors(msg, currentErrors)
			Expect(result).To(HaveLen(1))
			e1 := result[0]
			Expect(e1.Error).Should(Equal(msg))
			Expect(e1.LastOccurrence.After(now.Time)).To(BeTrue(), "last occurrence should be after 'now'")
		})
	})

	Context("Existing global errors", func() {
		Context("Existing single matching error", func() {
			It("Updates the last occurrence", func() {
				msg := "This is an error"
				currentErrorTime := metav1.NewTime(time.Now().Add(-time.Minute * 10))
				current := []riskifiedv1alpha1.StatusError{
					{
						Error:          msg,
						LastOccurrence: currentErrorTime,
					},
				}
				result := handlers.SyncGlobalErrors(msg, current)
				Expect(result).To(HaveLen(1))
				e1 := result[0]
				Expect(e1.Error).Should(Equal(msg))
				Expect(e1.LastOccurrence.After(currentErrorTime.Time)).To(BeTrue(), "After update error time should be after original")
			})
		})

		Context("Existing multiple errors including matching error", func() {
			It("Updates the last occurrence of the matching error", func() {
				msg := "This is an error"
				currentErrorTime := metav1.NewTime(time.Now().Add(-time.Minute * 10))
				current := []riskifiedv1alpha1.StatusError{
					riskifiedv1alpha1.NewStatusError("Different error 1"),
					{
						Error:          msg,
						LastOccurrence: currentErrorTime,
					},
					riskifiedv1alpha1.NewStatusError("Different error 2"),
				}
				result := handlers.SyncGlobalErrors(msg, current)
				Expect(result).To(HaveLen(3))
				idx := findErrorIndex(msg, result)
				err := result[idx]
				Expect(err.LastOccurrence.After(currentErrorTime.Time)).To(BeTrue(), "After update error time should be after original")
			})
		})

		Context("Existing Errors without matching error", func() {
			It("Adds the provided error", func() {
				msg := "This is an error"
				current := []riskifiedv1alpha1.StatusError{
					riskifiedv1alpha1.NewStatusError("Different error 1"),
					riskifiedv1alpha1.NewStatusError("Different error 2"),
				}
				result := handlers.SyncGlobalErrors(msg, current)
				Expect(result).To(HaveLen(3))
				_ = findErrorIndex(msg, result)
				// If last call did not panic than we're ok.

			})
		})
	})
})

var _ = Describe("SyncSubsetMessagesToStatus", func() {
	st1msgDeploy := "A deployment message on subset1"
	st1msgVS := "A VS message on subset1"
	st1msgDR := "A DR message on subset1"
	st1msgErr := "A global error on subset1"
	st2msgDeploy := "A deployment message on subset2"
	st2msgVS := "A VS message on subset2"
	st2msgDR := "A DR message on subset2"
	st2msgErr := "A global error on subset2"
	nonEmptyStatus := map[string]riskifiedv1alpha1.SubsetStatus{
		"subset1": {
			Errors: &riskifiedv1alpha1.SubsetErrors{
				Deployment: []riskifiedv1alpha1.StatusError{
					{
						Error: "A deployment error on subset1",
					},
				},
				DestinationRule: []riskifiedv1alpha1.StatusError{
					{
						Error: "A DR error on subset1",
					},
				},
				VirtualServices: []riskifiedv1alpha1.StatusError{
					{
						Error: "A VS error on subset 1",
					},
				},
				Subset: []riskifiedv1alpha1.StatusError{
					{
						Error: "A subset error on subset 1",
					},
				},
			},
		},
		"subset2": {
			Errors: &riskifiedv1alpha1.SubsetErrors{
				Deployment: []riskifiedv1alpha1.StatusError{
					{
						Error: "A deployment error on subset2",
					},
				},
				DestinationRule: []riskifiedv1alpha1.StatusError{
					{
						Error: "A DR error on subset2",
					},
				},
				VirtualServices: []riskifiedv1alpha1.StatusError{
					{
						Error: "A VS error on subset 2",
					},
				},
				Subset: []riskifiedv1alpha1.StatusError{
					{
						Error: "A subset error on subset 2",
					},
				},
			},
		},
	}
	nonEmptyMessages := map[string]riskifiedv1alpha1.SubsetMessages{
		"subset1": {
			Deployment: []string{
				st1msgDeploy,
			},
			VirtualService: []string{
				st1msgVS,
			},
			DestinationRule: []string{
				st1msgDR,
			},
			GlobalErrors: []string{
				st1msgErr,
			},
		},
		"subset2": {
			Deployment: []string{
				st2msgDeploy,
			},
			VirtualService: []string{
				st2msgVS,
			},
			DestinationRule: []string{
				st2msgDR,
			},
			GlobalErrors: []string{
				st2msgErr,
			},
		},
	}

	mkStatusHandler := func() *handlers.DynamicEnvStatusHandler {
		return &handlers.DynamicEnvStatusHandler{
			DynamicEnv: &riskifiedv1alpha1.DynamicEnv{
				Status: riskifiedv1alpha1.DynamicEnvStatus{
					SubsetsStatus: make(map[string]riskifiedv1alpha1.SubsetStatus),
				},
			},
		}
	}

	statusErrors2Messages := func(errors []riskifiedv1alpha1.StatusError) (result []string) {
		for _, err := range errors {
			result = append(result, err.Error)
		}
		return result
	}

	Context("With empty status", func() {
		It("should include all messages", func() {
			h := mkStatusHandler()
			h.SyncSubsetMessagesToStatus(nonEmptyMessages)
			result := h.DynamicEnv.Status.SubsetsStatus
			Expect(result).To(HaveLen(2))
			errors1 := handlers.SafeGetSubsetErrors(result["subset1"])
			errors2 := handlers.SafeGetSubsetErrors(result["subset2"])
			Expect(errors1.Deployment[0].Error).To(Equal(st1msgDeploy))
			Expect(errors1.DestinationRule[0].Error).To(Equal(st1msgDR))
			Expect(errors1.VirtualServices[0].Error).To(Equal(st1msgVS))
			Expect(errors1.Subset[0].Error).To(Equal(st1msgErr))
			Expect(errors2.Deployment[0].Error).To(Equal(st2msgDeploy))
			Expect(errors2.DestinationRule[0].Error).To(Equal(st2msgDR))
			Expect(errors2.VirtualServices[0].Error).To(Equal(st2msgVS))
			Expect(errors2.Subset[0].Error).To(Equal(st2msgErr))
		})
	})

	Context("With no messages", func() {
		It("should leave status the same", func() {
			h := mkStatusHandler()
			h.DynamicEnv.Status.SubsetsStatus = nonEmptyStatus
			h.SyncSubsetMessagesToStatus(make(map[string]riskifiedv1alpha1.SubsetMessages))
			result := h.DynamicEnv.Status.SubsetsStatus
			Expect(result).To(Equal(nonEmptyStatus))
		})
	})

	Context("With both status and messages", func() {
		It("should merge messages", func() {
			h := mkStatusHandler()
			h.DynamicEnv.Status.SubsetsStatus = nonEmptyStatus
			h.SyncSubsetMessagesToStatus(nonEmptyMessages)
			subset1 := h.DynamicEnv.Status.SubsetsStatus["subset1"]
			Expect(subset1.Errors).ToNot(BeNil(), "expected error messages, received nil")
			Expect(subset1.Errors.Deployment).To(HaveLen(2))
			Expect(subset1.Errors.Deployment).To(ContainElement(nonEmptyStatus["subset1"].Errors.Deployment[0]))
			Expect(statusErrors2Messages(subset1.Errors.Deployment)).To(ContainElement(st1msgDeploy))
			Expect(subset1.Errors.DestinationRule).To(HaveLen(2))
			Expect(subset1.Errors.DestinationRule).To(ContainElement(nonEmptyStatus["subset1"].Errors.DestinationRule[0]))
			Expect(statusErrors2Messages(subset1.Errors.DestinationRule)).To(ContainElement(st1msgDR))
			Expect(subset1.Errors.VirtualServices).To(HaveLen(2))
			Expect(subset1.Errors.VirtualServices).To(ContainElement(nonEmptyStatus["subset1"].Errors.VirtualServices[0]))
			Expect(statusErrors2Messages(subset1.Errors.VirtualServices)).To(ContainElement(st1msgVS))
			Expect(subset1.Errors.Subset).To(HaveLen(2))
			Expect(subset1.Errors.Subset).To(ContainElement(nonEmptyStatus["subset1"].Errors.Subset[0]))
			Expect(statusErrors2Messages(subset1.Errors.Subset)).To(ContainElement(st1msgErr))
		})
	})
})
