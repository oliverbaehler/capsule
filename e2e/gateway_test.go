//go:build e2e

// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
	"github.com/projectcapsule/capsule/pkg/api"
	"github.com/projectcapsule/capsule/pkg/utils"
)

var _ = Describe("enforcing a Gateway Class", func() {

	gvr := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1alpha1", Resource: "gateways"}

	tntWithDefaults := &capsulev1beta2.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gateway-class-defaults",
		},
		Spec: capsulev1beta2.TenantSpec{
			Owners: capsulev1beta2.OwnerListSpec{
				{
					Name: "great",
					Kind: "User",
				},
			},
			GatewayOptions: capsulev1beta2.GatewayOptions{
				AllowedClasses: &api.DefaultSelectorListSpec{
					Default: "tenant-default",
					LabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"env": "customer",
						},
					},
				},
			},
		},
	}

	tntNoDefaults := &capsulev1beta2.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gateway-class-no-defaults",
		},
		Spec: capsulev1beta2.TenantSpec{
			Owners: capsulev1beta2.OwnerListSpec{
				{
					Name: "gatsby",
					Kind: "User",
				},
			},
			GatewayOptions: capsulev1beta2.GatewayOptions{
				AllowedClasses: &api.DefaultSelectorListSpec{
					LabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"env": "customer",
						},
					},
				},
			},
		},
	}

	tenantDefaultClass := &gwapiv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant-default",
			Labels: map[string]string{
				"env": "e2e",
			},
		},
		Spec: gwapiv1.GatewayClassSpec{
			ControllerName: "networking.k8s.io/controller",
		},
	}

	customerClass := &gwapiv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "customer-class",
			Labels: map[string]string{
				"env": "customer",
			},
		},
		Spec: gwapiv1.GatewayClassSpec{
			ControllerName: "networking.k8s.io/controller",
		},
	}

	infraClass := &gwapiv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "infra-class",
			Labels: map[string]string{
				"env": "e2e",
			},
		},
		Spec: gwapiv1.GatewayClassSpec{
			ControllerName: "networking.k8s.io/controller",
		},
	}

	JustBeforeEach(func() {
		if err := k8sClient.List(context.Background(), &gwapiv1.GatewayList{}); err != nil {
			if utils.IsUnsupportedAPI(err) {
				Skip(fmt.Sprintf("Running test due to unsupported API kind: %s", err.Error()))
			}
		}
		if err := k8sClient.List(context.Background(), &gwapiv1.GatewayClassList{}); err != nil {
			if utils.IsUnsupportedAPI(err) {
				Skip(fmt.Sprintf("Running test due to unsupported API kind: %s", err.Error()))
			}
		}

		for _, tnt := range []*capsulev1beta2.Tenant{tntWithDefaults, tntNoDefaults} {
			EventuallyCreation(func() error {
				tnt.ResourceVersion = ""

				return k8sClient.Create(context.TODO(), tnt)
			}).Should(Succeed())
		}
	})

	JustAfterEach(func() {
		for _, tnt := range []*capsulev1beta2.Tenant{tntWithDefaults, tntNoDefaults} {
			Expect(k8sClient.Delete(context.TODO(), tnt)).Should(Succeed())
		}

		Eventually(func() (err error) {
			req, _ := labels.NewRequirement("env", selection.Exists, nil)

			return k8sClient.DeleteAllOf(context.TODO(), &gwapiv1.GatewayClass{}, &client.DeleteAllOfOptions{
				ListOptions: client.ListOptions{
					LabelSelector: labels.NewSelector().Add(*req),
				},
			})
		}, defaultTimeoutInterval, defaultPollInterval).Should(Succeed())
	})

	It("should block non allowed GatewayClass (GatewayClass not existing)", func() {
		ns := NewNamespace("")
		NamespaceCreation(ns, tntNoDefaults.Spec.Owners[0], defaultTimeoutInterval).Should(Succeed())

		gateway := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.networking.k8s.io/v1",
				"kind":       "Gateway",
				"metadata": map[string]interface{}{
					"name": "some-gateway",
				},
				"spec": map[string]interface{}{
					"gatewayClassName": "test",
				},
			},
		}

		cs := dynamicOwnerClient(tntNoDefaults.Spec.Owners[0])
		EventuallyCreation(func() error {
			_, err := cs.Resource(gvr).Namespace(ns.GetName()).Create(context.Background(), gateway, metav1.CreateOptions{})
			return err
		}).ShouldNot(Succeed())
	})

	It("should block non allowed GatewayClass (GatewayClass existing)", func() {
		ns := NewNamespace("")
		NamespaceCreation(ns, tntNoDefaults.Spec.Owners[0], defaultTimeoutInterval).Should(Succeed())

		Expect(k8sClient.Create(context.TODO(), infraClass)).Should(Succeed())

		gateway := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.networking.k8s.io/v1",
				"kind":       "Gateway",
				"metadata": map[string]interface{}{
					"name": "some-gateway",
				},
				"spec": map[string]interface{}{
					"gatewayClassName": infraClass.GetName(),
				},
			},
		}

		cs := dynamicOwnerClient(tntNoDefaults.Spec.Owners[0])
		EventuallyCreation(func() error {
			_, err := cs.Resource(gvr).Namespace(ns.GetName()).Create(context.Background(), gateway, metav1.CreateOptions{})
			return err
		}).ShouldNot(Succeed())
	})

	It("should allow selected GatewayClass", func() {
		ns := NewNamespace("")
		NamespaceCreation(ns, tntNoDefaults.Spec.Owners[0], defaultTimeoutInterval).Should(Succeed())

		Expect(k8sClient.Create(context.TODO(), customerClass)).Should(Succeed())

		gateway := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.networking.k8s.io/v1",
				"kind":       "Gateway",
				"metadata": map[string]interface{}{
					"name": "some-gateway",
				},
				"spec": map[string]interface{}{
					"gatewayClassName": customerClass.GetName(),
				},
			},
		}

		cs := dynamicOwnerClient(tntNoDefaults.Spec.Owners[0])
		EventuallyCreation(func() error {
			_, err := cs.Resource(gvr).Namespace(ns.GetName()).Create(context.Background(), gateway, metav1.CreateOptions{})
			return err
		}).Should(Succeed())
	})

	It("fail if default tenant GatewayClass is absent", func() {
		ns := NewNamespace("")
		NamespaceCreation(ns, tntWithDefaults.Spec.Owners[0], defaultTimeoutInterval).Should(Succeed())

		gateway := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.networking.k8s.io/v1",
				"kind":       "Gateway",
				"metadata": map[string]interface{}{
					"name": "some-gateway",
				},
			},
		}

		cs := dynamicOwnerClient(tntNoDefaults.Spec.Owners[0])
		EventuallyCreation(func() error {
			_, err := cs.Resource(gvr).Namespace(ns.GetName()).Create(context.Background(), gateway, metav1.CreateOptions{})
			return err
		}).ShouldNot(Succeed())
	})

	It("assign tenant GatewayClass", func() {
		ns := NewNamespace("")
		NamespaceCreation(ns, tntWithDefaults.Spec.Owners[0], defaultTimeoutInterval).Should(Succeed())

		Expect(k8sClient.Create(context.TODO(), tenantDefaultClass)).Should(Succeed())

		gateway := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.networking.k8s.io/v1",
				"kind":       "Gateway",
				"metadata": map[string]interface{}{
					"name": "some-gateway",
				},
			},
		}

		cs := dynamicOwnerClient(tntNoDefaults.Spec.Owners[0])
		EventuallyCreation(func() error {
			_, err := cs.Resource(gvr).Namespace(ns.GetName()).Create(context.Background(), gateway, metav1.CreateOptions{})
			return err
		}).Should(Succeed())

		Expect(gateway.Object["spec"].(string)["gatewayClassName"]).To(Equal(tenantDefaultClass.GetName()))
	})
})
