package v1alpha1

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	"sigs.k8s.io/yaml"
)

var zeroReplicas int32 = 0

func mkDynamicEnvFromYamlFile(fileName string) (de DynamicEnv, err error) {
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

var _ = Describe("Validating Webhook", func() {
	Context("Updating DynamicEnvironment Matchers", func() {
		base := DynamicEnv{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-de",
				Namespace: "default",
			},
			Spec: DynamicEnvSpec{
				IstioMatches: []IstioMatch{
					{
						Headers: map[string]StringMatch{
							"name": {
								Exact: "my_name",
							},
						},
					},
				},
			},
		}

		updated := DynamicEnv{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-de",
				Namespace: "default",
			},
			Spec: DynamicEnvSpec{
				IstioMatches: []IstioMatch{
					{
						Headers: map[string]StringMatch{
							"another-name": {
								Exact: "my_name",
							},
						},
					},
				},
			},
		}

		It("Does not allow to update matchers", func() {
			old := runtime.Object(&base)
			err := updated.ValidateUpdate(old)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("field is immutable"))
		})
	})

	Context("Creating new DynamicEnvironment", func() {

		var (
			multiStringMatch1 = DynamicEnv{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-de",
					Namespace: "default",
				},
				Spec: DynamicEnvSpec{
					IstioMatches: []IstioMatch{
						{
							Headers: map[string]StringMatch{
								"name": {
									Exact:  "my_name",
									Prefix: "my",
								},
							},
						},
					},
				},
			}

			multiStringMatch2 = DynamicEnv{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-de",
					Namespace: "default",
				},
				Spec: DynamicEnvSpec{
					IstioMatches: []IstioMatch{
						{
							Headers: map[string]StringMatch{
								"name": {
									Exact: "my_name",
								},
							},
						},
						{ // This item should fail
							Headers: map[string]StringMatch{
								"name": {
									Exact: "my_name",
									Regex: "my",
								},
							},
						},
					},
				},
			}
		)

		It("StringMatch only allows one of prefix/match/regex", func() {
			testCases := []DynamicEnv{
				multiStringMatch1,
				multiStringMatch2,
			}

			for ind, item := range testCases {
				By(fmt.Sprintf("multi stringMatch %d", ind))
				err := item.ValidateCreate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("oneOf"))
			}
		},
		)

		It("Requires at least one of headers or source label match", func() {
			noMatchDe := DynamicEnv{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-de",
					Namespace: "default",
				},
				Spec: DynamicEnvSpec{},
			}
			err := noMatchDe.ValidateCreate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("empty IstioMatch"))
		})

		It("Accepts multiple correct matchers", func() {
			de := DynamicEnv{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-de",
					Namespace: "default",
				},
				Spec: DynamicEnvSpec{
					IstioMatches: []IstioMatch{
						{
							Headers: map[string]StringMatch{
								"name": {
									Exact: "my_name",
								},
							},
						},
						{
							SourceLabels: map[string]string{
								"key": "value",
							},
						},
					},
				},
			}

			err := de.ValidateCreate()
			Expect(err).To(BeNil())
		})

		It("Accepts subsets with specified number of replicas", func() {
			var replicas int32 = 8
			de1 := DynamicEnv{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-de",
					Namespace: "default",
				},
				Spec: DynamicEnvSpec{
					IstioMatches: []IstioMatch{
						{
							Headers: map[string]StringMatch{
								"name": {
									Exact: "my_name",
								},
							},
						},
					},
					Subsets: []Subset{
						{
							Name:           "somename",
							Namespace:      "ns",
							Replicas:       &replicas,
							DefaultVersion: "version",
							Containers: []ContainerOverrides{
								{
									ContainerName: "a-container",
									Image:         "an-image:tag",
								},
							},
						},
					},
					Consumers: nil,
				},
			}
			de2 := DynamicEnv{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-de",
					Namespace: "default",
				},
				Spec: DynamicEnvSpec{
					IstioMatches: []IstioMatch{
						{
							Headers: map[string]StringMatch{
								"name": {
									Exact: "my_name",
								},
							},
						},
					},
					Consumers: []Subset{
						{
							Name:           "somename",
							Namespace:      "ns",
							Replicas:       &replicas,
							DefaultVersion: "version",
							Containers: []ContainerOverrides{
								{
									ContainerName: "a-container",
									Image:         "an-image:tag",
								},
							},
						},
					},
					Subsets: nil,
				},
			}

			err1 := de1.ValidateCreate()
			Expect(err1).To(BeNil(), "Should accept number of replicas in subsets")
			err2 := de2.ValidateCreate()
			Expect(err2).To(BeNil(), "Should accept number of replicas in consumers")
		})

		It("Create rejects deployments without containers or init-containers", func() {
			de, err := mkDynamicEnvFromYamlFile("fixtures/create-rejects-deployments-without-containers-or-init-containers.yaml")
			if err != nil {
				Fail(err.Error())
			}
			resultError := de.ValidateCreate()
			Expect(resultError).To(HaveOccurred())
			Expect(resultError.Error()).To(ContainSubstring("At least a single container or init-container"))
		})

		It("Create rejects multiple container within single subset with conflicting container names", func() {
			de, err := mkDynamicEnvFromYamlFile("fixtures/deployment-with-multiple-containers-with-conflicting-names.yaml")
			if err != nil {
				Fail(err.Error())
			}
			resultError := de.ValidateCreate()
			Expect(resultError).To(HaveOccurred())
			Expect(resultError.Error()).To(ContainSubstring("names are unique"))
		})

		DescribeTable("Create rejects invalid subset properties",
			func(de *DynamicEnv, errMsg string) {
				err := de.ValidateCreate()
				Expect(err).To(Not(BeNil()))
				Expect(err.Error()).To(ContainSubstring(errMsg))
			},
			Entry(
				"0 replicas in subset",
				&DynamicEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-de",
						Namespace: "default",
					},
					Spec: DynamicEnvSpec{
						IstioMatches: []IstioMatch{
							{
								Headers: map[string]StringMatch{
									"name": {
										Exact: "my_name",
									},
								},
							},
						},
						Subsets: []Subset{
							{
								Name:           "somename",
								Namespace:      "ns",
								Replicas:       &zeroReplicas,
								DefaultVersion: "version",
							},
						},
						Consumers: nil,
					},
				},
				"0 replicas",
			),
			Entry(
				"0 replicas in consumer",
				&DynamicEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-de",
						Namespace: "default",
					},
					Spec: DynamicEnvSpec{
						IstioMatches: []IstioMatch{
							{
								Headers: map[string]StringMatch{
									"name": {
										Exact: "my_name",
									},
								},
							},
						},
						Subsets: nil,
						Consumers: []Subset{
							{
								Name:           "somename",
								Namespace:      "ns",
								Replicas:       &zeroReplicas,
								DefaultVersion: "version",
							},
						},
					},
				},
				"0 replicas",
			),
		)

		DescribeTable(
			"it rejects empty matchers",
			func(match IstioMatch) {
				de := DynamicEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-de",
						Namespace: "default",
					},
					Spec: DynamicEnvSpec{
						IstioMatches: []IstioMatch{
							match,
						},
					},
				}

				err := de.ValidateCreate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Invalid value"))
			},
			Entry(
				"empty headers",
				IstioMatch{
					Headers: map[string]StringMatch{},
				},
			),
			Entry(
				"empty source labels",
				IstioMatch{
					SourceLabels: map[string]string{},
				},
			),
		)
	})

	Context("Updating subsets", func() {
		DescribeTable(
			"Allowed modifications",
			func(oldData, currentData string) {
				old, err := mkDynamicEnvFromYamlFile(oldData)
				if err != nil {
					Fail("Error decoding oldData: " + err.Error())
				}
				current, err := mkDynamicEnvFromYamlFile(currentData)
				if err != nil {
					Fail("Error decoding currentData")
				}
				errorResult := current.ValidateUpdate(runtime.Object(&old))
				Expect(errorResult).To(BeNil())
			},
			Entry(
				"Modified image",
				"fixtures/allowed-modifications-modified-image-old.yaml",
				"fixtures/allowed-modifications-modified-image-new.yaml",
			),
			Entry(
				"Modified order of subsets",
				"fixtures/allowed-modifications-modified-order-of-subsets-old.yaml",
				"fixtures/allowed-modifications-modified-order-of-subsets-new.yaml",
			),
			Entry(
				"Modifying number of replicas",
				"fixtures/allowed-modifications-modifying-number-of-replicas-old.yaml",
				"fixtures/allowed-modifications-modifying-number-of-replicas-new.yaml",
			),
		)

		DescribeTable(
			"Disallowed modifications",
			func(oldData, currentData, partialError string) {
				old, err := mkDynamicEnvFromYamlFile(oldData)
				if err != nil {
					Fail("Error decoding oldData: " + err.Error())
				}
				current, err := mkDynamicEnvFromYamlFile(currentData)
				if err != nil {
					Fail("Error decoding currentData")
				}
				errorResult := current.ValidateUpdate(runtime.Object(&old))
				Expect(errorResult).To(HaveOccurred())
				Expect(errorResult.Error()).To(ContainSubstring(partialError))
			},
			Entry(
				"modifying container name",
				"fixtures/disallowed-modifications-modifying-container-name-old.yaml",
				"fixtures/disallowed-modifications-modifying-container-name-new.yaml",
				"couldn't find matching container to existing container: details",
			),
			Entry(
				"modified initContainer name",
				"fixtures/disallowed-modifications-modifying-init_container-name-old.yaml",
				`fixtures/disallowed-modifications-modifying-init_container-name-new.yaml`,
				"couldn't find matching container to existing container: details",
			),
			Entry(
				"modifying default version",
				"fixtures/disallowed-modifications-modifying-default-version-old.yaml",
				"fixtures/disallowed-modifications-modifying-default-version-new.yaml",
				"(modified 'shared' to 'modified')",
			),
			Entry(
				"Modifying to '0' replicas",
				"fixtures/disallowed-modifications-modifying-to-zero-replicas-old.yaml",
				"fixtures/disallowed-modifications-modifying-to-zero-replicas-new.yaml",
				"0 replicas",
			),
		)
	})
})
