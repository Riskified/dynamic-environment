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

package handlers

import (
	"context"
	riskifiedv1alpha1 "github.com/riskified/dynamic-environment/api/v1alpha1"
)

// Common functionality for SRHandler and MRHandler
type BaseHandler interface {
	// An entry point to the handler. Takes care of initializing / updating the resource.
	Handle(context.Context) error
	// Get the subset unique name for this handler
	GetSubset() string
}

// A SingleResourceHandler for any of the resources we manage in DynamicEnv controller.
type SRHandler interface {
	BaseHandler
	// Computes what the current status of the resource should be. It does not
	// update the status, just computes what the current status should be.
	GetStatus(ctx context.Context) (riskifiedv1alpha1.ResourceStatus, error)
	// Apply the provided status to the DynamicEnvironment.
	ApplyStatus(riskifiedv1alpha1.ResourceStatus) error
}

// MultiResourceHandler is a spacial kind of Handler as it may affect several resources.
type MRHandler interface {
	BaseHandler
	// Computes the status of all the services which are affected by this handler. It doesn't change
	// anything, just returns the found statuses.
	GetStatus(ctx context.Context) ([]riskifiedv1alpha1.ResourceStatus, error)
	// Apply the provided status to the DynamicEnvironment.
	ApplyStatus([]riskifiedv1alpha1.ResourceStatus) error
	/// GetHosts returns non-missing hosts.
	GetHosts() []string
}

// IgnoredMissing should indicate an acceptable missing resource (e.g. missing DR per hostname)
type IgnoredMissing struct{}

func (im IgnoredMissing) Error() string { return "Ignored Missing Resource" }
