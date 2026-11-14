// Copyright 2020-2026 Project Capsule Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	clientcache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
	"github.com/projectcapsule/capsule/internal/cache"
	"github.com/projectcapsule/capsule/internal/controllers/utils"
	"github.com/projectcapsule/capsule/internal/metrics"
	"github.com/projectcapsule/capsule/pkg/runtime/replication"
)

const maxResourceWatches = 64

// ResourceWatches owns one informer per explicit GVR on the elected replica.
// The controller maintains the watch set; event handling uses the registered
// manager-cache index and immutable prepared source projections.
type ResourceWatches struct {
	newInformer func(schema.GroupVersionResource) clientcache.SharedIndexInformer
	client      client.Client
	dynamic     dynamic.Interface
	mapper      apimeta.RESTMapper
	log         logr.Logger
	conditions  *cache.CELCache
	selectors   *cache.LabelSelectorCache
	prepared    *cache.PreparedReplicationTriggers
	notifier    replication.Notifier
	metrics     *metrics.TriggerRecorder
	mu          sync.Mutex
	desired     map[schema.GroupVersionResource]schema.GroupVersionKind
	changed     chan struct{}
	events      chan resourceWatchEvent
}

func NewResourceWatches(c client.Client, dyn dynamic.Interface, mapper apimeta.RESTMapper, log logr.Logger, conditions *cache.CELCache, selectors *cache.LabelSelectorCache, notifier replication.Notifier, recorder *metrics.TriggerRecorder) *ResourceWatches {
	return &ResourceWatches{client: c, dynamic: dyn, mapper: mapper, log: log, conditions: conditions, selectors: selectors, prepared: cache.NewPreparedReplicationTriggers(), notifier: notifier, metrics: recorder, desired: make(map[schema.GroupVersionResource]schema.GroupVersionKind), changed: make(chan struct{}, 1), events: make(chan resourceWatchEvent, 256)}
}

func (r *ResourceWatches) NeedLeaderElection() bool { return true }

func (r *ResourceWatches) SetupWithManager(mgr ctrl.Manager, options utils.ControllerOptions) error {
	enqueue := handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
		return []reconcile.Request{{Name: "resource-watches"}}
	})
	if err := ctrl.NewControllerManagedBy(mgr).Named("capsule/resources/watches").
		Watches(&capsulev1beta2.TenantResource{}, enqueue, builder.WithPredicates(watchSubscriptionChanged())).
		Watches(&capsulev1beta2.GlobalTenantResource{}, enqueue, builder.WithPredicates(watchSubscriptionChanged())).
		WithOptions(options.Runtime.ToControllerOptions()).Complete(reconcile.Func(func(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
		return r.Reconcile(ctx, reconcile.Request{})
	})); err != nil {
		return err
	}

	return mgr.Add(r)
}

func triggerSpec(obj client.Object) *capsulev1beta2.TenantResourceCommonSpec {
	switch source := obj.(type) {
	case *capsulev1beta2.TenantResource:
		return &source.Spec.TenantResourceCommonSpec
	case *capsulev1beta2.GlobalTenantResource:
		return &source.Spec.TenantResourceCommonSpec
	default:
		return nil
	}
}

func watchSubscriptionChanged() predicate.Predicate {
	hasTriggers := func(obj client.Object) bool {
		spec := triggerSpec(obj)

		return spec != nil && len(spec.Triggers) > 0
	}

	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return hasTriggers(e.Object) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return hasTriggers(e.Object) },
		GenericFunc: func(event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			return (hasTriggers(e.ObjectOld) || hasTriggers(e.ObjectNew)) && (e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() || e.ObjectOld.GetDeletionTimestamp().IsZero() != e.ObjectNew.GetDeletionTimestamp().IsZero())
		},
	}
}

func (r *ResourceWatches) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	local := &capsulev1beta2.TenantResourceList{}
	global := &capsulev1beta2.GlobalTenantResourceList{}

	filter := client.MatchingFields{replication.TriggerIndexerField: replication.TriggerEnabledKey}
	if err := r.client.List(ctx, local, filter, client.UnsafeDisableDeepCopy); err != nil {
		return reconcile.Result{}, err
	}

	if err := r.client.List(ctx, global, filter, client.UnsafeDisableDeepCopy); err != nil {
		return reconcile.Result{}, err
	}

	active := make(map[types.UID]int64, len(local.Items)+len(global.Items))

	sources := make([]client.Object, 0, len(local.Items)+len(global.Items))
	for i := range local.Items {
		sources = append(sources, &local.Items[i])
		active[local.Items[i].UID] = local.Items[i].Generation
	}

	for i := range global.Items {
		sources = append(sources, &global.Items[i])
		active[global.Items[i].UID] = global.Items[i].Generation
	}

	r.prepared.PruneActive(active)
	sort.Slice(sources, func(i, j int) bool {
		a, b := sources[i], sources[j]

		return a.GetNamespace()+"/"+a.GetName() < b.GetNamespace()+"/"+b.GetName()
	})

	resources := make(map[schema.GroupVersionResource]struct{})
	omitted := 0

	for _, source := range sources {
		spec := triggerSpec(source)

		count := 0

		for _, trigger := range spec.Triggers {
			count += len(trigger.MatchConditions)
		}

		encoded, err := json.Marshal(spec.Triggers)
		if err != nil {
			return reconcile.Result{}, err
		}

		fingerprint := sha256.Sum256(encoded)

		prepared, err := r.prepared.GetOrBuild(source.GetUID(), source.GetGeneration(), count, fingerprint, func() (*replication.PreparedTriggers, error) {
			return replication.PrepareTriggers(spec, source.GetNamespace() != "", r.selectors, r.conditions)
		})
		if err != nil {
			r.prepared.Delete(source.GetUID())

			omitted++

			r.log.Error(err, "cannot prepare resource watches; periodic reconciliation remains active", "name", source.GetName(), "namespace", source.GetNamespace())

			continue
		}

		for _, resource := range prepared.Resources() {
			resources[resource] = struct{}{}
		}
	}

	keys := make([]schema.GroupVersionResource, 0, len(resources))
	for resource := range resources {
		keys = append(keys, resource)
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })

	desired := make(map[schema.GroupVersionResource]schema.GroupVersionKind)
	for _, resource := range keys {
		if len(desired) >= maxResourceWatches {
			omitted++

			continue
		}

		kind, err := r.mapper.KindFor(resource)
		if err != nil {
			omitted++

			r.log.Error(err, "cannot discover trigger resource", "resource", resource.String())

			continue
		}

		mapping, err := r.mapper.RESTMapping(kind.GroupKind(), kind.Version)
		if err != nil {
			omitted++

			r.log.Error(err, "cannot map trigger resource", "resource", resource.String())

			continue
		}

		if mapping.Resource != resource {
			omitted++

			r.log.Error(fmt.Errorf("watch requires the canonical plural resource name"), "cannot watch trigger resource", "resource", resource.String())

			continue
		}

		if mapping.Scope.Name() != apimeta.RESTScopeNameNamespace {
			omitted++

			r.log.Error(fmt.Errorf("watch requires a namespaced resource"), "cannot watch trigger resource", "resource", resource.String())

			continue
		}

		desired[resource] = kind
	}

	r.metrics.OmittedWatches.Set(float64(omitted))
	r.mu.Lock()
	changed := !maps.Equal(r.desired, desired)
	r.desired = desired
	r.mu.Unlock()

	if changed {
		select {
		case r.changed <- struct{}{}:
		default:
		}
	}

	return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
}
