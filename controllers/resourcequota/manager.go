// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package resourcequota

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	quota "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
)

type Controller struct {
	client.Client
	Log        logr.Logger
	Recorder   record.EventRecorder
	RESTConfig *rest.Config
	Options    ControllerOptions
}

type ControllerOptions struct {
	// Must have authority to list all quotas, and update quota status
	//QuotaClient corev1client.ResourceQuotasGetter
	// Shared informer for resource quotas
	//ResourceQuotaInformer coreinformers.ResourceQuotaInformer
	// Controls full recalculation of quota usage
	//ResyncPeriod controller.ResyncPeriodFunc
	// Maintains evaluators that know how to calculate usage for group resource
	Registry quota.Registry
	// Discover list of supported resources on the server.
	//DiscoveryFunc NamespacedResourcesFunc
	// A function that returns the list of resources to ignore
	//IgnoredResourcesFunc func() map[schema.GroupResource]struct{}
	// InformersStarted knows if informers were started.
	//InformersStarted <-chan struct{}
	// InformerFactory interfaces with informers.
	//InformerFactory informerfactory.InformerFactory
	// Controls full resync of objects monitored for replenishment.
	//ReplenishmentResyncPeriod controller.ResyncPeriodFunc
	// Filters update events so we only enqueue the ones where we know quota will change
	//UpdateFilter UpdateFilter
}

func (r *Controller) enqueueRequestFromNamespace(ctx context.Context, object client.Object) (reqs []reconcile.Request) {
	ns := object.(*corev1.Namespace) //nolint:forcetypeassert

	resList := capsulev1beta2.TenantResourceQuotaList{}
	if err := r.Client.List(ctx, &resList); err != nil {
		return nil
	}

	set := sets.NewString()

	for _, res := range resList.Items {
		nsSelector := res.Spec.NamespaceSelector

		selector, err := metav1.LabelSelectorAsSelector(&nsSelector)
		if err != nil {
			continue
		}

		if selector.Matches(labels.Set(ns.GetLabels())) {
			set.Insert(res.GetName())
		}
	}
	// No need of ordered value here
	for res := range set {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: res,
			},
		})
	}

	return reqs
}

func (r *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capsulev1beta2.TenantResourceQuota{}).
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.enqueueRequestFromNamespace)).
		//Watches(&capsulev1beta2.Tenant{}, handler.EnqueueRequestsFromMapFunc(r.enqueueRequestFromTenant)).
		Owns(&corev1.ResourceQuota{}).
		Complete(r)
}

//nolint:nakedret
func (r Controller) Reconcile(ctx context.Context, request ctrl.Request) (result ctrl.Result, err error) {
	r.Log = r.Log.WithValues("Request.Name", request.Name)
	// Fetch the Tenant instance
	origin := &capsulev1beta2.TenantResourceQuota{}
	if err = r.Client.Get(ctx, types.NamespacedName{Name: request.Name}, origin); err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Request object not found, could have been deleted after reconcile request")

			return reconcile.Result{}, nil
		}

		r.Log.Error(err, "Error reading the object")

		return
	}

	// Ensuring all namespaces are collected
	if result, err = r.reconcile(ctx, origin); err != nil {
		r.Log.Error(err, "TenantResourceQuota reconciling failed")

		return result, err
	}

	r.Log.Info("TenantResourceQuota reconciling completed")

	return ctrl.Result{}, err
}

func (r *Controller) reconcile(ctx context.Context, origin *capsulev1beta2.TenantResourceQuota) (reconcile.Result, error) {
	log := ctrllog.FromContext(ctx)

	// Retrieving the list of the Tenants up to the selector provided by the GlobalTenantResource resource.
	nsSelector, err := metav1.LabelSelectorAsSelector(&origin.Spec.NamespaceSelector)
	if err != nil {
		log.Error(err, "cannot create MatchingLabelsSelector for Global filtering")

		return reconcile.Result{}, err
	}

	// cleanup tenants which are no longer controlled by the TenantResourceQuota
	quotaNamespaces := corev1.NamespaceList{}
	if err = r.Client.List(ctx, &quotaNamespaces, &client.MatchingLabelsSelector{Selector: nsSelector}); err != nil {
		log.Error(err, "cannot list Tenants matching the provided selector")

		return reconcile.Result{}, err
	}

	// Synchronize ResourceQuotas
	err = r.syncResourceQuota(ctx, origin, quotaNamespaces.Items)
	if err != nil {
		log.Error(err, "cannot sync ResourceQuotas")
	}

	// Update Status with controlled tenants
	if err = r.collectNamespaces(ctx, origin, quotaNamespaces.Items); err != nil {
		log.Error(err, "cannot update TenantResourceQuota status")

		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *Controller) collectNamespaces(ctx context.Context, origin *capsulev1beta2.TenantResourceQuota, nsList []corev1.Namespace) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, origin.DeepCopy(), func() error {
			origin.AssignNamespaces(nsList)

			return r.Client.Status().Update(ctx, origin, &client.SubResourceUpdateOptions{})
		})

		return
	})
}

// Compares observed tenants with the current tenants (diffs require cleanup)
func (r *Controller) observedTenants(ctx context.Context, origin *capsulev1beta2.TenantResourceQuota, nsList []corev1.Namespace) error {
	observed := origin.Status.Namespaces
	if observed == nil {
		return nil
	}

	observedMap := make(map[string]bool)
	for _, ns := range origin.Status.Namespaces {
		observedMap[ns] = true
	}

	for _, ns := range nsList {
		if _, ok := observedMap[ns.GetName()]; !ok {
			// Tenant's Name was not found in the observed map; invoke cleanup.
			r.cleanup(ctx, origin, ns)
		}
	}

	return nil
}
