// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package route

import (
	capsulewebhook "github.com/projectcapsule/capsule/pkg/webhook"
)

// +kubebuilder:webhook:path=/resourcequota,mutating=false,sideEffects=None,admissionReviewVersions=v1,failurePolicy=fail,groups="",resources=resourcequotas,verbs=create;update;delete,versions=v1,name=resourcequota.projectcapsule.dev

type resourceQuota struct {
	handlers []capsulewebhook.Handler
}

func ResourceQuota(handler ...capsulewebhook.Handler) capsulewebhook.Webhook {
	return &resourceQuota{handlers: handler}
}

func (w *resourceQuota) GetHandlers() []capsulewebhook.Handler {
	return w.handlers
}

func (w *resourceQuota) GetPath() string {
	return "/resourcequota"
}
