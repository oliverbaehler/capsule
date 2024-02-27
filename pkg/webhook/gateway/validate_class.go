// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"context"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
	"github.com/projectcapsule/capsule/pkg/configuration"
	capsulewebhook "github.com/projectcapsule/capsule/pkg/webhook"
	"github.com/projectcapsule/capsule/pkg/webhook/utils"
)

type class struct {
	configuration configuration.Configuration
	version       *version.Version
}

func Class(configuration configuration.Configuration, version *version.Version) capsulewebhook.Handler {
	return &class{
		configuration: configuration,
		version:       version,
	}
}

func (r *class) OnCreate(client client.Client, decoder *admission.Decoder, recorder record.EventRecorder) capsulewebhook.Func {
	return func(ctx context.Context, req admission.Request) *admission.Response {
		return r.validate(ctx, r.version, client, req, decoder, recorder)
	}
}

func (r *class) OnUpdate(client client.Client, decoder *admission.Decoder, recorder record.EventRecorder) capsulewebhook.Func {
	return func(ctx context.Context, req admission.Request) *admission.Response {
		return r.validate(ctx, r.version, client, req, decoder, recorder)
	}
}

func (r *class) OnDelete(client.Client, *admission.Decoder, record.EventRecorder) capsulewebhook.Func {
	return func(context.Context, admission.Request) *admission.Response {
		return nil
	}
}

func (r *class) validate(ctx context.Context, version *version.Version, client client.Client, req admission.Request, decoder *admission.Decoder, recorder record.EventRecorder) *admission.Response {
	gateway := &gwapiv1.Gateway{}
	if err := decoder.Decode(req, gateway); err != nil {
		return utils.ErroredResponse(err)
	}

	var tnt *capsulev1beta2.Tenant

	tnt, err := TenantFromGateway(ctx, client, *gateway)
	if err != nil {
		return utils.ErroredResponse(err)
	}

	if tnt == nil {
		return nil
	}

	allowed := tnt.Spec.GatewayOptions.AllowedClasses
	if allowed == nil {
		return nil
	}
	gw := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]interface{}{
				"name": "some-gateway",
			},
		},
	}

	gw.Object["spec"] = map[string]interface{}{

	// Verify if the IngressClass exists and matches the label selector/expression
	if len(allowed.MatchLabels) > 0 || len(allowed.MatchExpressions) > 0 {

		gatewayClass := string(gateway.Spec.GatewayClassName)
		if gatewayClass == "" {
			recorder.Eventf(tnt, corev1.EventTypeWarning, "MissingGatewayClass", "Gateway %s/%s is missing GatewayClass", req.Namespace, req.Name)

			response := admission.Denied(NewIngressClassUndefined(*allowed).Error())

			return &response
		}

		gatewayClassObj, err := utils.GetGatewayClassByName(ctx, client, string(gatewayClass))
		if err != nil {
			response := admission.Errored(http.StatusInternalServerError, err)

			return &response
		}

		// Ingress Class is present, check if it matches the selector
		if !allowed.SelectorMatch(gatewayClassObj) {
			recorder.Eventf(tnt, corev1.EventTypeWarning, "ForbiddenGatewayClass", "Gateway %s/%s GatewayClass %s is forbidden for the current Tenant", req.Namespace, req.Name, gatewayClass)

			response := admission.Denied(NewGatewayClassForbidden(gatewayClass, *allowed).Error())

			return &response
		}
	}

	return nil
}
