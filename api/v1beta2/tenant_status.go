// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package v1beta2

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// +kubebuilder:validation:Enum=Cordoned;Active
type tenantState string

const (
	TenantStateActive   tenantState = "Active"
	TenantStateCordoned tenantState = "Cordoned"
)

// Returns the observed state of the Tenant.
type TenantStatus struct {
	// +kubebuilder:default=Active
	// The operational state of the Tenant. Possible values are "Active", "Cordoned".
	State tenantState `json:"state"`
	// How many namespaces are assigned to the Tenant.
	Size uint `json:"size"`
	// List of namespaces assigned to the Tenant.
	Namespaces []string `json:"namespaces,omitempty"`
	// Quota Usages (Tenant Scope)
	Quota TenantQuotaList `json:"quota,omitempty"`
}

type TenantQuotaList map[string]TenantQuotaStatus

// SetQuotaByIndex sets or updates a TenantQuotaStatus by its index.
func (tql *TenantQuotaList) SetQuotaByIndex(index string, quotaStatus TenantQuotaStatus) {
	if *tql == nil {
		*tql = make(TenantQuotaList)
	}
	(*tql)[index] = quotaStatus
}

// RemoveQuotaByIndex removes a TenantQuotaStatus by its index.
func (tql *TenantQuotaList) RemoveQuotaByIndex(index string) error {
	if *tql == nil {
		return errors.New("tenant quota list is nil")
	}
	if _, exists := (*tql)[index]; !exists {
		return fmt.Errorf("quota with index %s does not exist", index)
	}
	delete(*tql, index)
	return nil
}

// AddNamespaceForQuota adds a namespace to a specific TenantQuotaStatus by its index.
func (tql *TenantQuotaList) AddNamespaceForQuota(index string, namespace string, quotaStatus corev1.ResourceQuotaStatus) error {
	if *tql == nil {
		*tql = make(TenantQuotaList)
	}
	tenantQuota, exists := (*tql)[index]
	if !exists {
		return fmt.Errorf("quota with index %s does not exist", index)
	}
	if tenantQuota.Quotas == nil {
		return nil
	}
	tenantQuota.Quotas[namespace] = quotaStatus
	(*tql)[index] = tenantQuota
	return nil
}

// RemoveNamespaceForQuota removes a namespace from a specific TenantQuotaStatus by its index.
func (tql *TenantQuotaList) RemoveNamespaceForQuota(index string, namespace string) error {
	if *tql == nil {
		return errors.New("tenant quota list is nil")
	}
	tenantQuota, exists := (*tql)[index]
	if !exists {
		return fmt.Errorf("quota with index %s does not exist", index)
	}
	if tenantQuota.Quotas == nil {
		return fmt.Errorf("no namespaces found for quota with index %s", index)
	}
	if _, exists := tenantQuota.Quotas[namespace]; !exists {
		return fmt.Errorf("namespace %s does not exist for quota with index %s", namespace, index)
	}
	delete(tenantQuota.Quotas, namespace)
	(*tql)[index] = tenantQuota
	return nil
}

type TenantQuotaStatus struct {
	// Display the current usage of quotas for tenant scoped  quota
	Usage *corev1.ResourceQuotaStatus `json:"usage,omitempty"`
	// All namespaced Quotas for the Tenant.
	Quotas map[string]corev1.ResourceQuotaStatus `json:"quotas,omitempty"`
}
