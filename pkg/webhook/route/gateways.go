// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package route

import (
	capsulewebhook "github.com/projectcapsule/capsule/pkg/webhook"
)

// +kubebuilder:webhook:path=/gateways,mutating=false,sideEffects=None,admissionReviewVersions=v1,failurePolicy=fail,groups=gateway.networking.k8s.io,resources=gateways,verbs=create;update,versions=v1,name=gateways.projectcapsule.dev

type gateway struct {
	handlers []capsulewebhook.Handler
}

func Gateway(handler ...capsulewebhook.Handler) capsulewebhook.Webhook {
	return &gateway{handlers: handler}
}

func (w *gateway) GetHandlers() []capsulewebhook.Handler {
	return w.handlers
}

func (w *gateway) GetPath() string {
	return "/gateways"
}
