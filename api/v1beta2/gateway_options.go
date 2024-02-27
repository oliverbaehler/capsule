// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package v1beta2

import (
	"github.com/projectcapsule/capsule/pkg/api"
)

type GatewayOptions struct {
	// Specifies the allowed IngressClasses assigned to the Tenant.
	// Capsule assures that all Ingress resources created in the Tenant can use only one of the allowed IngressClasses.
	// A default value can be specified, and all the Ingress resources created will inherit the declared class.
	// Optional.
	AllowedClasses *api.DefaultSelectorListSpec `json:"allowedClasses,omitempty"`
}
