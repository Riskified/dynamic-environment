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

package helpers

import (
	"crypto/sha256"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/strings/slices"
	"strings"

	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
)

var log = MkLogger("DynamicEnv")

const (
	KeyValueHeaderSeparator    = ":"
	KeyValueHeaderConcatinator = "|"
)

func RemoveItemFromStringSlice(s string, slc []string) []string {
	var result []string
	for _, item := range slc {
		if s != item {
			result = append(result, item)
		}
	}
	return result
}

// Generates (somewhat) unique hash of a struct (or anything else). Depends on
// the object's `String` method. Copied from blog post:
// https://blog.8bitzen.com/posts/22-08-2019-how-to-hash-a-struct-in-go
func AsSha256(o interface{}) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%v", o)))

	return fmt.Sprintf("%x", h.Sum(nil))
}

// Shortens the provided string to the desired length
func Shorten(msg string, length int) string {
	if len(msg) <= length {
		return msg
	} else {
		return msg[0:length]
	}
}

func UniqueDynamicEnvName(id types.NamespacedName) string {
	name := id.Name
	ns := id.Namespace
	if len(name) > 40 {
		name = name[:40]
	}
	if len(ns) > 20 {
		ns = ns[:20]
	}
	return fmt.Sprintf("%s-%s", ns, name)
}

// This is a temporary hack so we should not care about edge cases:
//
//goland:noinspection ALL
func SerializeIstioMatchExactHeaders(headers map[string]riskifiedv1alpha1.StringMatch) string {
	var serialized strings.Builder
	for k, v := range headers {
		if len(v.Exact) > 0 {
			if serialized.Len() > 0 {
				fmt.Fprint(&serialized, KeyValueHeaderConcatinator)
			}
			fmt.Fprintf(&serialized, "%s%s%s", k, KeyValueHeaderSeparator, v.Exact)
		} else {
			log.V(1).Info("Ignoring non-exact header while serializing match for envoy filters", "ignored-key", k)
		}
	}
	return serialized.String()
}

// Validate intersection between two slices is not empty/
func CommonValueExists(l1, l2 []string) bool {
	for _, s := range l2 {
		if slices.Contains(l1, s) {
			return true
		}
	}
	return false
}

// Just a convention to name subsets
func MKSubsetName(subset riskifiedv1alpha1.Subset) string {
	return fmt.Sprintf("%s/%s", subset.Namespace, subset.Name)
}

func MkResourceName(id types.NamespacedName) string {
	return fmt.Sprintf("%s/%s", id.Namespace, id.Name)
}

func MkSubsetUniqueName(name, version string) string {
	return name + "-" + version
}
