// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"fmt"

	"github.com/projectcapsule/capsule/pkg/api"
)

type gatewayClassForbiddenError struct {
	className string
	spec      api.DefaultSelectorListSpec
}

func NewGatewayClassForbidden(class string, spec api.DefaultSelectorListSpec) error {
	return &gatewayClassForbiddenError{
		className: class,
		spec:      spec,
	}
}

func (i gatewayClassForbiddenError) Error() string {
	err := fmt.Sprintf("Gateway Class %s is forbidden for the current Tenant: not matching the label selector defined in the Tenant", i.className)

	return err
}

type gatewayClassUndefinedError struct {
	spec api.DefaultSelectorListSpec
}

func NewIngressClassUndefined(spec api.DefaultSelectorListSpec) error {
	return &gatewayClassUndefinedError{
		spec: spec,
	}
}

func (i gatewayClassUndefinedError) Error() string {
	return "No Gateway Class is forbidden for the current Tenant. Specify a Gateway Class which is allowed within the Tenant: not matching the label selector defined in the Tenant"
}

type gatewayClassNotValidError struct {
	className string
	spec      api.DefaultSelectorListSpec
}

func NewGatewayClassNotValid(class string, spec api.DefaultSelectorListSpec) error {
	return &gatewayClassNotValidError{
		className: class,
		spec:      spec,
	}
}

func (i gatewayClassNotValidError) Error() string {
	return "No Gateway Class is forbidden for the current Tenant. Specify a Gateway Class which is allowed within the Tenant: not matching the label selector defined in the Tenant"
}
