// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
)

// TenantResourceQuotaStatus defines the observed state of TenantResourceQuota
type TenantResourceQuotaStatus struct {
	// Hard is the set of enforced hard limits for each named resource
	// +optional
	Quota corev1.ResourceQuotaStatus `json:"quota,omitempty"`
	// Tenant workload is distributed to these nodes
	// +optional
	Nodes []string `json:"nodes,omitempty"`
	// List of namespaces which are using this resource quota
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`
}
