// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package v1beta2

import (
	"github.com/projectcapsule/capsule/pkg/api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TenantResourceQuotaSpec defines the desired state of TenantResourceQuota
type TenantResourceQuotaSpec struct {
	// Selector is used to select the tenants that the Resource Budget should apply to.
	NamespaceSelector metav1.LabelSelector `json:"selector,omitempty"`
	// Takes a resource quota
	ResourceQuota corev1.ResourceQuotaSpec `json:"quota,omitempty"`
	// Allow Specifying scheduling options for the selected tenants
	Scheduling []api.SchedulingOptions `json:"scheduling,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// TenantResourceQuota is the Schema for the tenantresourcequotas API
type TenantResourceQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantResourceQuotaSpec   `json:"spec,omitempty"`
	Status TenantResourceQuotaStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TenantResourceQuotaList contains a list of TenantResourceQuota
type TenantResourceQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TenantResourceQuota `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TenantResourceQuota{}, &TenantResourceQuotaList{})
}
