// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package quota

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/go-logr/logr"
	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
	capsuleutils "github.com/projectcapsule/capsule/pkg/utils"
	capsulewebhook "github.com/projectcapsule/capsule/pkg/webhook"
	"github.com/projectcapsule/capsule/pkg/webhook/utils"
)

type statusHandler struct {
	log logr.Logger
}

func StatusHandler(log logr.Logger) capsulewebhook.Handler {
	return &statusHandler{log: log}
}

func (h *statusHandler) OnCreate(c client.Client, decoder admission.Decoder, recorder record.EventRecorder) capsulewebhook.Func {
	return func(ctx context.Context, req admission.Request) *admission.Response {
		return nil
	}
}

func (h *statusHandler) OnDelete(client.Client, admission.Decoder, record.EventRecorder) capsulewebhook.Func {
	return func(context.Context, admission.Request) *admission.Response {
		return nil
	}
}

func (h *statusHandler) OnUpdate(c client.Client, decoder admission.Decoder, recorder record.EventRecorder) capsulewebhook.Func {
	return func(ctx context.Context, req admission.Request) *admission.Response {
		return h.validate(ctx, c, decoder, recorder, req)
	}
}

func (h *statusHandler) validate(ctx context.Context, c client.Client, decoder admission.Decoder, recorder record.EventRecorder, req admission.Request) *admission.Response {
	// Focus on status subresource updates
	//if req.SubResource != "status" {
	//	return nil
	//}

	// Decode the incoming object
	quota := &corev1.ResourceQuota{}
	if err := decoder.Decode(req, quota); err != nil {
		return utils.ErroredResponse(fmt.Errorf("failed to decode new ResourceQuota object: %w", err))
	}

	// Decode the old object from the request to retrieve its status
	oldQuota := &corev1.ResourceQuota{}
	if req.OldObject.Raw != nil {
		if err := json.Unmarshal(req.OldObject.Raw, oldQuota); err != nil {
			return utils.ErroredResponse(fmt.Errorf("failed to decode old ResourceQuota object: %w", err))
		}
	}

	indexLabel, err := capsuleutils.GetTypeLabel(&corev1.ResourceQuota{})
	if err != nil {
		return nil
	}

	index, ok := quota.GetLabels()[indexLabel]
	if !ok || index == "" {
		return nil
	}

	h.log.V(7).Info("selected quota", "index", index)

	tntList := &capsulev1beta2.TenantList{}
	if err := c.List(ctx, tntList, client.MatchingFieldsSelector{
		Selector: fields.OneTermEqualSelector(".status.namespaces", quota.Namespace),
	}); err != nil {
		return utils.ErroredResponse(err)
	}

	if len(tntList.Items) == 0 {
		return nil
	}

	tenant := tntList.Items[0]
	h.log.V(5).Info("Retrieved tenant", "tenant", tenant.Name, "namespace", quota.Namespace)

	// Use retry to handle concurrent updates
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Re-fetch the tenant to get the latest status
		if err := c.Get(ctx, client.ObjectKey{Name: tenant.Name}, &tenant); err != nil {
			h.log.Error(err, "Failed to fetch tenant during retry", "tenant", tenant.Name)
			return err
		}

		// Calculate changes in resource usage
		tenantQuota, exists := tenant.Status.Quota[index]
		if !exists {
			h.log.V(5).Info("No quota entry found in tenant status; initializing", "index", index)
			return nil
		}

		tenantUsed := tenantQuota.Usage.Used
		tenantHard := tenantQuota.Usage.Hard

		// Adjust the hard limits in the ResourceQuota
		for resourceName, tenantLimit := range tenantHard {
			currentUsage := tenantUsed[resourceName] // Total used resources in the tenant
			namespaceUsage := quota.Status.Used[resourceName]
			if namespaceUsage.IsZero() {
				namespaceUsage = resource.Quantity{} // Initialize if not present
			}

			// Remaining quota available at the tenant level
			remainingQuota := tenantLimit.DeepCopy()
			remainingQuota.Sub(currentUsage)

			h.log.V(5).Info("Calculating adjusted quota for resource",
				"resource", resourceName,
				"tenantLimit", tenantLimit.String(),
				"currentUsage", currentUsage.String(),
				"remainingQuota", remainingQuota.String(),
				"namespaceUsage", namespaceUsage.String())

			// If the remaining quota is less than or equal to zero, block further resource allocation
			if remainingQuota.Cmp(resource.Quantity{}) <= 0 {
				h.log.Info("Tenant quota exceeded; setting namespace limit to current usage",
					"resource", resourceName,
					"tenantLimit", tenantLimit.String(),
					"currentUsage", currentUsage.String(),
					"namespaceUsage", namespaceUsage.String())
				quota.Spec.Hard[resourceName] = namespaceUsage.DeepCopy()
				continue
			}

			// Calculate the new hard limit for the namespace
			// Ensure it doesn’t allow over-provisioning beyond the tenant's remaining quota
			adjustedHardLimit := namespaceUsage.DeepCopy()
			adjustedHardLimit.Add(remainingQuota)

			if adjustedHardLimit.Cmp(tenantLimit) > 0 {
				adjustedHardLimit = tenantLimit.DeepCopy()
			}

			quota.Spec.Hard[resourceName] = adjustedHardLimit

			h.log.Info("Adjusted ResourceQuota hard limit",
				"resource", resourceName,
				"newHardLimit", adjustedHardLimit.String(),
				"remainingQuota", remainingQuota.String(),
				"namespaceUsage", namespaceUsage.String())
		}

		// Persist the changes to the tenant's status
		tenantQuota.Usage.Used = tenantUsed
		tenant.Status.Quota[index] = tenantQuota
		if err := c.Status().Update(ctx, &tenant); err != nil {
			return fmt.Errorf("failed to update tenant status: %w", err)
		}

		h.log.Info("Successfully updated tenant status", "tenant", tenant.Name, "quota", index)
		return nil
	})

	if err != nil {
		h.log.Error(err, "Failed to process ResourceQuota update", "quota", quota.Name)
		return utils.ErroredResponse(err)
	}

	h.log.Info("ResourceQuota update accepted and tenant status updated", "namespace", quota.Namespace)

	marshaled, err := json.Marshal(quota)
	if err != nil {
		h.log.Error(err, "Failed to marshal mutated ResourceQuota object")
		return utils.ErroredResponse(err)
	}

	response := admission.PatchResponseFromRaw(req.Object.Raw, marshaled)

	return &response
}
