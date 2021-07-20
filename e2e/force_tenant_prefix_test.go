//+build e2e

// Copyright 2020-2021 Clastix Labs
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	capsulev1alpha1 "github.com/clastix/capsule/api/v1alpha1"

	capsulev1beta1 "github.com/clastix/capsule/api/v1beta1"
)

var _ = Describe("creating a Namespace with Tenant name prefix enforcement", func() {
	t1 := &capsulev1beta1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: "awesome",
		},
		Spec: capsulev1beta1.TenantSpec{
			Owners: capsulev1beta1.OwnerListSpec{
				{
					Name: "john",
					Kind: "User",
				},
			},
		},
	}
	t2 := &capsulev1beta1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: "awesome-tenant",
		},
		Spec: capsulev1beta1.TenantSpec{
			Owners: capsulev1beta1.OwnerListSpec{
				{
					Name: "john",
					Kind: "User",
				},
			},
		},
	}

	JustBeforeEach(func() {
		EventuallyCreation(func() error {
			t1.ResourceVersion = ""
			return k8sClient.Create(context.TODO(), t1)
		}).Should(Succeed())
		EventuallyCreation(func() error {
			t2.ResourceVersion = ""
			return k8sClient.Create(context.TODO(), t2)
		}).Should(Succeed())

		ModifyCapsuleConfigurationOpts(func(configuration *capsulev1alpha1.CapsuleConfiguration) {
			configuration.Spec.ForceTenantPrefix = true
		})
	})
	JustAfterEach(func() {
		Expect(k8sClient.Delete(context.TODO(), t1)).Should(Succeed())
		Expect(k8sClient.Delete(context.TODO(), t2)).Should(Succeed())

		ModifyCapsuleConfigurationOpts(func(configuration *capsulev1alpha1.CapsuleConfiguration) {
			configuration.Spec.ForceTenantPrefix = false
		})
	})

	It("should fail when non using prefix", func() {
		ns := NewNamespace("awesome")
		NamespaceCreation(ns, t1.Spec.Owners[0], defaultTimeoutInterval).ShouldNot(Succeed())
	})

	It("should succeed using prefix", func() {
		ns := NewNamespace("awesome-namespace")
		NamespaceCreation(ns, t1.Spec.Owners[0], defaultTimeoutInterval).Should(Succeed())
	})

	It("should succeed and assigned according to closest match", func() {
		ns1 := NewNamespace("awesome-tenant")
		ns2 := NewNamespace("awesome-tenant-namespace")

		NamespaceCreation(ns1, t1.Spec.Owners[0], defaultTimeoutInterval).Should(Succeed())
		NamespaceCreation(ns2, t2.Spec.Owners[0], defaultTimeoutInterval).Should(Succeed())

		TenantNamespaceList(t1, defaultTimeoutInterval).Should(ContainElement(ns1.GetName()))
		TenantNamespaceList(t2, defaultTimeoutInterval).Should(ContainElement(ns2.GetName()))
	})
})
