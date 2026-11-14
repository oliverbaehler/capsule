// Copyright 2020-2026 Project Capsule Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	dto "github.com/prometheus/client_model/go"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clientcache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
)

func TestSharedWatchStartupInitialListAndShutdown(t *testing.T) {
	f := watchFixtureFor(t, 0)
	f.watcher.log = testr.New(t)
	dynamic := f.watcher.dynamic.(*dynamicfake.FakeDynamicClient)
	initial := &corev1.ConfigMap{Name: "source", Namespace: "a", ResourceVersion: "1", Labels: map[string]string{"source": "yes"}}
	if err := dynamic.Tracker().Add(initial); err != nil {
		t.Fatal(err)
	}
	var watchStarts atomic.Int64
	informer := dynamicinformer.NewFilteredDynamicInformer(dynamic, configMapsGVR, metav1.NamespaceAll, 0, clientcache.Indexers{}, nil).Informer()
	f.watcher.newInformer = func(schema.GroupVersionResource) clientcache.SharedIndexInformer { watchStarts.Add(1); return informer }
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- f.watcher.Start(ctx) }()
	poll := func(phase string, condition func() bool) {
		t.Helper()
		if err := wait.PollUntilContextTimeout(t.Context(), 5*time.Millisecond, 5*time.Second, true, func(context.Context) (bool, error) { return condition(), nil }); err != nil {
			t.Fatalf("%s: %v starts=%d synced=%t queued=%d targets=%v", phase, err, watchStarts.Load(), informer.HasSynced(), len(f.watcher.events), f.notifications.take())
		}
	}
	poll("initial sync", func() bool { return informer.HasSynced() })
	if len(f.notifications.take()) != 0 {
		t.Fatal("initial list replayed existing objects as creates")
	}
	source := sourceObject("a")
	source.SetName("created-after-start")
	if _, err := dynamic.Resource(configMapsGVR).Namespace("a").Create(t.Context(), source, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	poll("new event", func() bool {
		f.notifications.mu.Lock()
		defer f.notifications.mu.Unlock()
		return f.notifications.targets["local-a"] == 1 && f.notifications.targets["global"] == 1
	})
	if watchStarts.Load() != 1 {
		t.Fatal("identical subscriptions opened separate watches")
	}
	for _, object := range []client.Object{&capsulev1beta2.TenantResource{Name: "a", Namespace: "a"}, &capsulev1beta2.TenantResource{Name: "b", Namespace: "b"}, &capsulev1beta2.GlobalTenantResource{Name: "global"}} {
		if err := f.client.Delete(t.Context(), object); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.watcher.Reconcile(t.Context(), reconcile.Request{}); err != nil {
		t.Fatal(err)
	}
	poll("removal", func() bool {
		metric := &dto.Metric{}
		_ = f.watcher.metrics.ActiveWatches.Write(metric)
		return metric.GetGauge().GetValue() == 0
	})
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch shutdown did not join goroutines")
	}
	if !f.watcher.NeedLeaderElection() {
		t.Fatal("watchers must run on elected replica")
	}
}

func TestBookkeepingComparisonPreservesUserChanges(t *testing.T) {
	resource := configMapsGVR
	resource.Group = capsulev1beta2.GroupVersion.Group
	resource.Resource = "tenantresources"
	before := sourceObject("a")
	before.SetAnnotations(map[string]string{"user": "keep"})
	after := before.DeepCopy()
	after.SetResourceVersion("2")
	after.SetAnnotations(map[string]string{"user": "changed"})
	if bookkeepingEvent(resource, before, after) {
		t.Fatal("user annotations ignored")
	}
	after.SetAnnotations(before.GetAnnotations())
	after.SetGeneration(before.GetGeneration() + 1)
	if bookkeepingEvent(resource, before, after) {
		t.Fatal("spec update ignored")
	}
	if before.GetAnnotations()["user"] != "keep" {
		t.Fatal("comparison mutated input")
	}
	f := watchFixtureFor(t, 0)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	f.watcher.enqueueEvent(canceled, resource, admissionv1.Create, nil, after)
	if len(f.watcher.events) != 0 {
		t.Fatal("canceled stream accepted an event")
	}
}
