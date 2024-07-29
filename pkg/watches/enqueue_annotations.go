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

package watches

import (
	"context"
	"fmt"
	"github.com/riskified/dynamic-environment/pkg/helpers"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
)

const (
	// NamespacedNameAnnotation is an annotation that indicates who the dynamic environment owner of this resource is.
	// The format is `<namespace>/<name>` with comma-separated values if there is more than one dynamic environment
	NamespacedNameAnnotation = "riskified.com/dynamic-environment"
)

type EnqueueRequestForAnnotation struct{}

var _ handler.EventHandler = &EnqueueRequestForAnnotation{}

// Create is called in response to an 'add' event.
func (e *EnqueueRequestForAnnotation) Create(_ context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	addToQueue(evt.Object, q)
}

// Update is called in response to an update event.
func (e *EnqueueRequestForAnnotation) Update(_ context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	addToQueue(evt.ObjectNew, q)
	addToQueue(evt.ObjectOld, q)
}

// Delete is called in response to a 'delete' event.
func (e *EnqueueRequestForAnnotation) Delete(_ context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	addToQueue(evt.Object, q)
}

// GenericFunc is called in response to a generic event.
func (e *EnqueueRequestForAnnotation) Generic(_ context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	addToQueue(evt.Object, q)
}

// addToQueue converts annotations defined for NamespacedNameAnnotation as a comma-separated list and add them to queue.
func addToQueue(object client.Object, q workqueue.RateLimitingInterface) {
	annotations := object.GetAnnotations()
	if annotations != nil {
		dynamicEnvs := strings.Split(annotations[NamespacedNameAnnotation], ",")
		for _, env := range dynamicEnvs {
			if env != "" {
				values := strings.SplitN(env, "/", 2)
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      values[1],
					Namespace: values[0],
				}})
			}
		}
	}
}

// AddToAnnotation appends the current Dynamic environment to `NamespacedNameAnnotation`
func AddToAnnotation(owner types.NamespacedName, object client.Object) {
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	existingDynamicEnvs := strings.Split(annotations[NamespacedNameAnnotation], ",")
	currentDynamicEnv := fmt.Sprintf("%s/%s", owner.Namespace, owner.Name)

	if len(existingDynamicEnvs) == 1 && existingDynamicEnvs[0] == "" {
		existingDynamicEnvs[0] = currentDynamicEnv
	} else if !slices.Contains(existingDynamicEnvs, currentDynamicEnv) {
		existingDynamicEnvs = append(existingDynamicEnvs, currentDynamicEnv)
	}

	annotations[NamespacedNameAnnotation] = strings.Join(existingDynamicEnvs, ",")
	object.SetAnnotations(annotations)
}

// RemoveFromAnnotation removes current Dynamic environment from `NamespacedNameAnnotation`
func RemoveFromAnnotation(owner types.NamespacedName, object client.Object) {
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	existingDynamicEnvs := strings.Split(annotations[NamespacedNameAnnotation], ",")
	currentDynamicEnv := fmt.Sprintf("%s/%s", owner.Namespace, owner.Name)
	existingDynamicEnvs = helpers.RemoveItemFromStringSlice(currentDynamicEnv, existingDynamicEnvs)

	annotations[NamespacedNameAnnotation] = strings.Join(existingDynamicEnvs, ",")
	object.SetAnnotations(annotations)
}
