// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package v1beta2

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
)

// Assigns selected Tenants to status
func (in *TenantResourceQuota) AssignNamespaces(namespaces []corev1.Namespace) {
	var s []string

	for _, t := range namespaces {
		s = append(s, t.GetName())
	}

	sort.Strings(s)

	in.Status.Namespaces = s
}
