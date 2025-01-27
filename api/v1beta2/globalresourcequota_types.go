// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GlobalResourceQuotaSpec defines the desired state of GlobalResourceQuota
type GlobalResourceQuotaSpec struct {
	// When a quota is active it's checking for the resources in the cluster
	// If not active the resourcequotas are removed and the webhook no longer blocks updates
	// +kubebuilder:default=true
	Active bool `json:"active"`

	// Selector to match the namespaces that should be managed by the GlobalResourceQuota

	// Define resourcequotas for the namespaces
	Items map[string]corev1.ResourceQuotaSpec `json:"items,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GlobalResourceQuota is the Schema for the globalresourcequotas API
type GlobalResourceQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GlobalResourceQuotaSpec   `json:"spec,omitempty"`
	Status GlobalResourceQuotaStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GlobalResourceQuotaList contains a list of GlobalResourceQuota
type GlobalResourceQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalResourceQuota `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GlobalResourceQuota{}, &GlobalResourceQuotaList{})
}
