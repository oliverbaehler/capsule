// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package replicated

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	capsulewebhook "github.com/projectcapsule/capsule/pkg/webhook"
	"github.com/projectcapsule/capsule/pkg/webhook/utils"
)

type lockHandler struct{}

func LockHandler() capsulewebhook.Handler {
	return &lockHandler{}
}

func (h *lockHandler) OnCreate(c client.Client, decoder admission.Decoder, recorder record.EventRecorder) capsulewebhook.Func {
	return func(ctx context.Context, req admission.Request) *admission.Response {
		return nil
	}
}

func (h *lockHandler) OnDelete(c client.Client, decoder admission.Decoder, recorder record.EventRecorder) capsulewebhook.Func {
	return func(ctx context.Context, req admission.Request) *admission.Response {
		return h.handle(ctx, c, decoder, recorder, req)
	}
}

func (h *lockHandler) OnUpdate(c client.Client, decoder admission.Decoder, recorder record.EventRecorder) capsulewebhook.Func {
	return func(ctx context.Context, req admission.Request) *admission.Response {
		return h.handle(ctx, c, decoder, recorder, req)
	}
}

func (h *lockHandler) handle(ctx context.Context, c client.Client, decoder admission.Decoder, recorder record.EventRecorder, req admission.Request) *admission.Response {
	// Decode the incoming object
	obj := &unstructured.Unstructured{}

	// Decode the incoming object
	if err := decoder.Decode(req, obj); err != nil {
		return utils.ErroredResponse(err)
	}

	// Log and create an event for denied admission
	recorder.Eventf(obj, "Warning", "DeniedAdmission", "Deletion blocked for object %s in namespace %s by user %s", obj.GetName(), obj.GetNamespace(), req.UserInfo.Username)

	response := admission.Denied("Deletion denied: object matches protected label criteria")
	return &response
}
