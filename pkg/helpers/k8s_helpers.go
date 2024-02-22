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
	"fmt"
	"github.com/go-logr/logr"

	"github.com/riskified/dynamic-environment/pkg/names"
	v1 "k8s.io/api/core/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// MatchNamespacedHost compares the provided `hostname` and `namespace` to the provided `matchHost`.
// If `matchHost` is *not* fully qualified, it uses the `inNamespace` parameter to match against the
// searched namespace.
func MatchNamespacedHost(hostname, namespace, matchedHost, inNamespace string) bool {
	shortNameEqual := hostname == matchedHost && namespace == inNamespace
	fqdnEqual := fmt.Sprint(hostname, ".", namespace, ".svc.cluster.local") == matchedHost
	return shortNameEqual || fqdnEqual
}

func MergeEnvVars(current []v1.EnvVar, overrides []v1.EnvVar) []v1.EnvVar {
	var newEnv []v1.EnvVar
	for _, env := range current {
		exists, overridingEnv := fetchEnvVar(overrides, env.Name)
		if exists {
			newEnv = append(newEnv, overridingEnv)
		} else {
			newEnv = append(newEnv, *env.DeepCopy())
		}
	}
	for _, env := range overrides {
		if !EnvVarContains(newEnv, env) {
			newEnv = append(newEnv, *env.DeepCopy())
		}
	}
	return newEnv
}

func EnvVarContains(envs []v1.EnvVar, env v1.EnvVar) bool {
	for _, e := range envs {
		if e == env {
			return true
		}
	}
	return false
}

func fetchEnvVar(envs []v1.EnvVar, name string) (exists bool, env v1.EnvVar) {
	for _, env := range envs {
		if env.Name == name {
			return true, *env.DeepCopy()
		}
	}
	return false, v1.EnvVar{}
}

// Calculates the prefix for naming virtual service route name prefix. Note that empty subset
// produces more inclusive prefix (e.g., everything belongs to this single dynamicenv).
func CalculateVirtualServicePrefix(version, subset string) string {
	subsetPart := ""
	if len(subset) > 0 {
		subsetPart = subset
	}
	return fmt.Sprintf("%s-%s-%s", names.VirtualServiceRoutePrefix, version, subsetPart)
}

// Generates Logger with provided values
func MkLogger(name string, keysAndValues ...interface{}) logr.Logger {
	return ctrllog.Log.WithName(name).WithValues(keysAndValues...)
}
