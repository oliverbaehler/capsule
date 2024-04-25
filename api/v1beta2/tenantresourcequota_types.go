// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=Tenant;Namespace
type ResourceQuotaScope string

const (
	ResourceQuotaScopeTenant    ResourceQuotaScope = "Tenant"
	ResourceQuotaScopeNamespace ResourceQuotaScope = "Namespace"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TenantResourceQuotaSpec defines the desired state of TenantResourceQuota
type TenantResourceQuotaSpec struct {
	// Selector is used to select the resources that the Resource Budget should apply to.
	Selector metav1.LabelSelector `json:"selector,omitempty"`

	// +kubebuilder:default=Tenant
	// Define if the Resource Budget should compute resource across all Namespaces in the Tenant or individually per cluster. Default is Tenant
	Scope ResourceQuotaScope `json:"scope,omitempty"`

	// Takes a resource quota
	ResourceQuota corev1.ResourceQuotaSpec `json:"quota,omitempty"`
}

// TenantResourceQuotaStatus defines the observed state of TenantResourceQuota
type TenantResourceQuotaStatus struct {
	// Hard is the set of enforced hard limits for each named resource
	// +optional
	Hard corev1.ResourceList
	// Used is the current observed total usage of the resource on this policy
	// +optional
	Used corev1.ResourceList
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
