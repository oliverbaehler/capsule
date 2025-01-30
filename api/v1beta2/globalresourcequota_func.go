// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package v1beta2

import (
	"fmt"
	"sort"

	"github.com/projectcapsule/capsule/pkg/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func (g *GlobalResourceQuota) GetQuotaSpace(index api.Name) (corev1.ResourceList, error) {
	quotaSpace := corev1.ResourceList{}

	// First, check if quota exists in the status
	if quotaStatus, exists := g.Status.Quota[index]; exists {
		// Iterate over all resources in the status
		for resourceName, hardValue := range quotaStatus.Hard {
			usedValue, usedExists := quotaStatus.Used[resourceName]
			if !usedExists {
				usedValue = resource.MustParse("0") // Default to zero if no used value is found
			}

			// Compute remaining quota (hard - used)
			remaining := hardValue.DeepCopy()
			remaining.Sub(usedValue)

			// Ensure we don't set negative values
			if remaining.Sign() == -1 {
				remaining.Set(0)
			}

			quotaSpace[resourceName] = remaining
		}

		return quotaSpace, nil
	}

	// If not in status, fall back to spec.Hard
	if quotaSpec, exists := g.Spec.Items[index]; exists {
		for resourceName, hardValue := range quotaSpec.Hard {
			quotaSpace[resourceName] = hardValue.DeepCopy()
		}

		return quotaSpace, nil
	}

	return nil, fmt.Errorf("no item found")
}

func (in *GlobalResourceQuota) AssignNamespaces(namespaces []corev1.Namespace) {
	var l []string

	for _, ns := range namespaces {
		if ns.Status.Phase == corev1.NamespaceActive {
			l = append(l, ns.GetName())
		}
	}

	sort.Strings(l)

	in.Status.Namespaces = l
	in.Status.Size = uint(len(l))
}
