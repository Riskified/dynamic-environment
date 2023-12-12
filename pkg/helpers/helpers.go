package helpers

import (
	"crypto/sha256"
	"fmt"
	"strings"

	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	KeyValueHeaderSeparator    = ":"
	KeyValueHeaderConcatinator = "|"
)

func IsStringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func StringSliceContains(s string, strings []string) bool {
	for _, item := range strings {
		if item == s {
			return true
		}
	}
	return false
}

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

func UniqueDynamicEnvName(de *riskifiedv1alpha1.DynamicEnv) string {
	name := de.Name
	ns := de.Namespace
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
			ctrl.Log.V(1).Info("Ignoring non-exact header while serializing match for envoy filters", "ignored-key", k)
		}
	}
	return serialized.String()
}

func HeadersContainsExactStringMatch(headers map[string]riskifiedv1alpha1.StringMatch) bool {
	for _, h := range headers {
		if len(h.Exact) > 0 {
			return true
		}
	}
	return false
}

// Validate intersection between two slices is not empty/
func CommonValueExists(l1, l2 []string) bool {
	for _, s := range l2 {
		if StringSliceContains(s, l1) {
			return true
		}
	}
	return false
}
