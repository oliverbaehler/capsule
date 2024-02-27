// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package defaults

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
	capsulegateway "github.com/projectcapsule/capsule/pkg/webhook/gateway"
	"github.com/projectcapsule/capsule/pkg/webhook/utils"
)

func mutateGatewayDefaults(ctx context.Context, req admission.Request, c client.Client, decoder *admission.Decoder, recorder record.EventRecorder, namespace string) *admission.Response {
	gateway := &gwapiv1.Gateway{}
	if err := decoder.Decode(req, gateway); err != nil {
		return utils.ErroredResponse(err)
	}

	gateway.SetNamespace(namespace)

	var tnt *capsulev1beta2.Tenant

	tnt, err := capsulegateway.TenantFromGateway(ctx, c, *gateway)
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

	gatewayClass := string(gateway.Spec.GatewayClassName)

	var mutate bool

	var _ *gwapiv1.GatewayClass
	// GatewayClass name is empty - no GatewayClass was given on gateway
	if len(gatewayClass) > 0 && gatewayClass != allowed.Default {
		_, err = utils.GetGatewayClassByName(ctx, c, gatewayClass)
		if err != nil {
			response := admission.Denied(NewGatewayClassError(gatewayClass, err).Error())

			return &response
		}
	} else {
		mutate = true
	}

	if mutate = mutate; !mutate {
		return nil
	}

	gw, err := utils.GetGatewayClassByName(ctx, c, allowed.Default)
	if err != nil {
		return utils.ErroredResponse(fmt.Errorf("failed to assign tenant default Gateway Class: %w", err))
	}

	gateway.Spec.GatewayClassName = gwapiv1.ObjectName(gw.Name)

	// Marshal Pod
	marshaled, err := json.Marshal(gateway)
	if err != nil {
		return utils.ErroredResponse(err)
	}

	recorder.Eventf(tnt, corev1.EventTypeNormal, "TenantDefault", "Assigned Tenant default Gateway Class %s to %s/%s", allowed.Default, gateway.Namespace, gateway.Name)

	response := admission.PatchResponseFromRaw(req.Object.Raw, marshaled)

	return &response
}
