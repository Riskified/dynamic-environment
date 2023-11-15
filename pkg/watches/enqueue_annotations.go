package watches

import (
	"fmt"
	"strings"

	"github.com/riskified/dynamic-environment/pkg/helpers"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// NamespacedNameAnnotation is an annotation which indicates who the dynamic environment owner of this resource is.
	// The format is `<namespace>/<name>` with comma-separated values if there is more than one dynamic environment
	NamespacedNameAnnotation = "riskified.com/dynamic-environment"
)

type EnqueueRequestForAnnotation struct{}

var _ handler.EventHandler = &EnqueueRequestForAnnotation{}

// Create is called in response to an add event.
func (e *EnqueueRequestForAnnotation) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	addToQueue(evt.Object, q)
}

// Update is called in response to an update event.
func (e *EnqueueRequestForAnnotation) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	addToQueue(evt.ObjectNew, q)
	addToQueue(evt.ObjectOld, q)
}

// Delete is called in response to a delete event.
func (e *EnqueueRequestForAnnotation) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	addToQueue(evt.Object, q)
}

// GenericFunc is called in response to a generic event.
func (e *EnqueueRequestForAnnotation) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	addToQueue(evt.Object, q)
}

// addToQueue converts annotations defined for NamespacedNameAnnotation as comma-separated list and add them to queue.
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
	} else if !helpers.StringSliceContains(currentDynamicEnv, existingDynamicEnvs) {
		existingDynamicEnvs = append(existingDynamicEnvs, currentDynamicEnv)
	}

	annotations[NamespacedNameAnnotation] = strings.Join(existingDynamicEnvs, ",")
	object.SetAnnotations(annotations)
}

// RemoveFromAnnotation removes current Dynamic environment from `NamespacedNameAnnotation`
func RemoveFromAnnotation(owner types.NamespacedName, object client.Object) {
	// Todo: Do we want to delete the annotation entirely if empty?
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

// ContainsAnnotations checks whether the requested annotation already exists.
func ContainsAnnotation(searchItem types.NamespacedName, object client.Object) bool {
	annotations := object.GetAnnotations()
	if annotations == nil {
		return false
	}
	existingAnnotations := strings.Split(annotations[NamespacedNameAnnotation], ",")
	searchFor := fmt.Sprintf("%s/%s", searchItem.Namespace, searchItem.Name)
	return helpers.StringSliceContains(searchFor, existingAnnotations)
}
