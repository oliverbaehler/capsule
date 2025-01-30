// Copyright 2020-2023 Project Capsule Authors.
// SPDX-License-Identifier: Apache-2.0

package globalquota

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
	"github.com/projectcapsule/capsule/pkg/api"
	"github.com/projectcapsule/capsule/pkg/metrics"
	"github.com/projectcapsule/capsule/pkg/utils"
)

// When the Resource Budget assigned to a Tenant is Tenant-scoped we have to rely on the ResourceQuota resources to
// represent the resource quota for the single Tenant rather than the single Namespace,
// so abusing of this API although its Namespaced scope.
//
// Since a Namespace could take-up all the available resource quota, the Namespace ResourceQuota will be a 1:1 mapping
// to the Tenant one: in first time Capsule is going to sum all the analogous ResourceQuota resources on other Tenant
// namespaces to check if the Tenant quota has been exceeded or not, reusing the native Kubernetes policy putting the
// .Status.Used value as the .Hard value.
// This will trigger following reconciliations but that's ok: the mutateFn will re-use the same business logic, letting
// the mutateFn along with the CreateOrUpdate to don't perform the update since resources are identical.
//
// In case of Namespace-scoped Resource Budget, we're just replicating the resources across all registered Namespaces.

//nolint:nakedret
func (r *Manager) syncResourceQuotas(
	ctx context.Context,
	quota *capsulev1beta2.GlobalResourceQuota,
	matchingNamespaces []string,
) (err error) { //nolint:gocognit
	// getting ResourceQuota labels for the mutateFn
	var quotaLabel, typeLabel string

	if quotaLabel, err = utils.GetTypeLabel(&capsulev1beta2.GlobalResourceQuota{}); err != nil {
		return err
	}

	typeLabel = utils.GetGlobalResourceQuotaTypeLabel()

	// Remove prior metrics, to avoid cleaning up for metrics of deleted ResourceQuotas
	metrics.TenantResourceUsage.DeletePartialMatch(map[string]string{"quota": quota.Name})
	metrics.TenantResourceLimit.DeletePartialMatch(map[string]string{"quota": quota.Name})

	// Remove Quotas which are no longer mentioned in spec
	for existingIndex := range quota.Status.Quota {
		if _, exists := quota.Spec.Items[api.Name(existingIndex)]; !exists {

			r.Log.V(7).Info("Orphaned quota index detected", "quotaIndex", existingIndex)

			for _, ns := range append(matchingNamespaces, quota.Status.Namespaces...) {
				selector := labels.SelectorFromSet(map[string]string{
					quotaLabel: quota.Name,
					typeLabel:  existingIndex.String(),
				})

				r.Log.V(7).Info("Searching for ResourceQuotas to delete", "namespace", ns, "selector", selector.String())

				// Query and delete all ResourceQuotas with matching labels in the namespace
				rqList := &corev1.ResourceQuotaList{}
				if err := r.Client.List(ctx, rqList, &client.ListOptions{
					Namespace:     ns,
					LabelSelector: selector,
				}); err != nil {
					r.Log.Error(err, "Failed to list ResourceQuotas", "namespace", ns, "quotaName", quota.Name, "index", existingIndex)
					return err
				}

				r.Log.V(7).Info("Found ResourceQuotas for deletion", "count", len(rqList.Items), "namespace", ns, "quotaIndex", existingIndex)

				for _, rq := range rqList.Items {
					if err := r.Client.Delete(ctx, &rq); err != nil {
						r.Log.Error(err, "Failed to delete ResourceQuota", "name", rq.Name, "namespace", ns)
						return err
					}

					r.Log.V(7).Info("Deleted orphaned ResourceQuota", "name", rq.Name, "namespace", ns)
				}
			}

			// Only Remove from status if the ResourceQuota has been deleted
			// Remove the orphaned quota from status
			delete(quota.Status.Quota, existingIndex)
			r.Log.Info("Removed orphaned quota from status", "quotaIndex", existingIndex)
		} else {
			r.Log.V(7).Info("no lifecycle", "quotaIndex", existingIndex)
		}
	}

	// Convert matchingNamespaces to a map for quick lookup
	matchingNamespaceSet := make(map[string]struct{}, len(matchingNamespaces))
	for _, ns := range matchingNamespaces {
		matchingNamespaceSet[ns] = struct{}{}
	}

	// Garbage collect namespaces which no longer match selector
	for _, existingNamespace := range quota.Status.Namespaces {
		if _, exists := matchingNamespaceSet[existingNamespace]; !exists {
			if err := r.gcResourceQuotas(ctx, quota, existingNamespace); err != nil {
				r.Log.Error(err, "Failed to garbage collect resource quota", "namespace", existingNamespace)
				return err
			}
		}
	}

	//nolint:nestif
	group := new(errgroup.Group)

	// Sync resource quotas for matching namespaces
	for _, ns := range matchingNamespaces {
		namespace := ns

		group.Go(func() error {
			return r.syncResourceQuota(ctx, quota, namespace)
		})
	}

	tenantQuotaStatus := capsulev1beta2.GlobalResourceQuotaStatusQuota{}
	mu := &sync.Mutex{}

	for i, q := range quota.Spec.Items {
		index, resourceQuota := i, q

		toKeep := sets.New[corev1.ResourceName]()
		for k := range resourceQuota.Hard {
			toKeep.Insert(k)
		}

		group.Go(func() (scopeErr error) {
			// Calculating the Resource Budget at Tenant scope just if this is put in place.
			// Requirement to list ResourceQuota of the current Tenant
			var tntRequirement *labels.Requirement

			if tntRequirement, scopeErr = labels.NewRequirement(quotaLabel, selection.Equals, []string{quota.Name}); scopeErr != nil {
				r.Log.Error(scopeErr, "Cannot build ResourceQuota Tenant requirement")
			}
			// Requirement to list ResourceQuota for the current index
			var indexRequirement *labels.Requirement

			if indexRequirement, scopeErr = labels.NewRequirement(typeLabel, selection.Equals, []string{index.String()}); scopeErr != nil {
				r.Log.Error(scopeErr, "Cannot build ResourceQuota index requirement")
			}
			// Listing all the ResourceQuota according to the said requirements.
			// These are required since Capsule is going to sum all the used quota to
			// sum them and get the Tenant one.
			list := &corev1.ResourceQuotaList{}
			if scopeErr = r.List(ctx, list, &client.ListOptions{LabelSelector: labels.NewSelector().Add(*tntRequirement).Add(*indexRequirement)}); scopeErr != nil {
				r.Log.Error(scopeErr, "Cannot list ResourceQuota", "tenantFilter", tntRequirement.String(), "indexFilter", indexRequirement.String())

				return scopeErr
			}

			r.Log.Info("ResourceQuota list", "items", len(list.Items))

			localQuotaStatus := &corev1.ResourceQuotaStatus{
				Used: corev1.ResourceList{},
				Hard: corev1.ResourceList{},
			}

			// Iterating over all the options declared for the ResourceQuota,
			// summing all the used quota across different Namespaces to determinate
			// if we're hitting a Hard quota at Tenant level.
			// For this case, we're going to block the Quota setting the Hard as the
			// used one.
			for name, hardQuota := range resourceQuota.Hard {
				r.Log.Info("Desired hard " + name.String() + " quota is " + hardQuota.String())

				// Getting the whole usage across all the Tenant Namespaces
				var quantity resource.Quantity
				for _, item := range list.Items {
					quantity.Add(item.Status.Used[name])
				}

				r.Log.Info("Computed " + name.String() + " quota for the whole Tenant is " + quantity.String())

				// Expose usage and limit metrics for the resource (name) of the ResourceQuota (index)
				metrics.GlobalResourceUsage.WithLabelValues(
					quota.Name,
					name.String(),
					index.String(),
				).Set(float64(quantity.MilliValue()) / 1000)

				metrics.GlobalResourceLimit.WithLabelValues(
					quota.Name,
					name.String(),
					index.String(),
				).Set(float64(hardQuota.MilliValue()) / 1000)

				localQuotaStatus.Used[name] = quantity
				localQuotaStatus.Hard[name] = hardQuota

				switch quantity.Cmp(resourceQuota.Hard[name]) {
				case 0:
					// The Tenant is matching exactly the Quota:
					// falling through next case since we have to block further
					// resource allocations.
					fallthrough
				case 1:
					r.Log.Info("block overprovisioning")
					// The Tenant is OverQuota:
					// updating all the related ResourceQuota with the current
					// used Quota to block further creations.
					for item := range list.Items {
						if _, ok := list.Items[item].Status.Used[name]; ok {
							list.Items[item].Spec.Hard[name] = list.Items[item].Status.Used[name]
						} else {
							um := make(map[corev1.ResourceName]resource.Quantity)
							um[name] = resource.Quantity{}
							list.Items[item].Spec.Hard = um
						}
					}
				default:
					r.Log.Info("respecting hard quota")
					// The Tenant is respecting the Hard quota:
					// restoring the default one for all the elements,
					// also for the reconciled one.
					for item := range list.Items {
						if list.Items[item].Spec.Hard == nil {
							list.Items[item].Spec.Hard = map[corev1.ResourceName]resource.Quantity{}
						}

						// Effectively this subtracts the usage from all other namespaces in the tenant from the desired tenant hard quota.
						// Thus we can determine, how much is left in this resourcequota (item) for the current resource (name).
						// We use this remaining quota at the tenant level, to update the hard quota for the current namespace.

						newHard := hardQuota                            // start off with desired tenant wide hard quota
						newHard.Sub(quantity)                           // subtract tenant wide usage
						newHard.Add(list.Items[item].Status.Used[name]) // add back usage in current ns

						list.Items[item].Spec.Hard[name] = newHard

						for k := range list.Items[item].Spec.Hard {
							if !toKeep.Has(k) {
								delete(list.Items[item].Spec.Hard, k)
							}
						}
					}
				}

				if scopeErr = r.resourceQuotasUpdate(ctx, name, quantity, toKeep, resourceQuota.Hard[name], list.Items...); scopeErr != nil {
					r.Log.Error(scopeErr, "cannot proceed with outer ResourceQuota")

					return
				}
			}

			mu.Lock()
			tenantQuotaStatus[index] = localQuotaStatus
			mu.Unlock()

			return
		})
	}
	// Waiting the update of all ResourceQuotas
	if err = group.Wait(); err != nil {
		return
	}

	// Update the tenant's status with the computed quota information
	quota.Status.Quota = tenantQuotaStatus
	if err := r.Status().Update(ctx, quota); err != nil {
		r.Log.Info("updating status", "quota", tenantQuotaStatus)

		r.Log.Error(err, "Failed to update tenant status")

		return err
	}

	return group.Wait()
}

//nolint:nakedret
func (r *Manager) syncResourceQuota(ctx context.Context, quota *capsulev1beta2.GlobalResourceQuota, namespace string) (err error) {
	// getting ResourceQuota labels for the mutateFn
	var quotaLabel, typeLabel string

	if quotaLabel, err = utils.GetTypeLabel(&capsulev1beta2.GlobalResourceQuota{}); err != nil {
		return err
	}

	typeLabel = utils.GetGlobalResourceQuotaTypeLabel()

	for index, resQuota := range quota.Spec.Items {
		target := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ItemObjectName(index, quota),
				Namespace: namespace,
			},
		}

		var res controllerutil.OperationResult

		err = retry.RetryOnConflict(retry.DefaultBackoff, func() (retryErr error) {
			res, retryErr = controllerutil.CreateOrUpdate(ctx, r.Client, target, func() (err error) {
				targetLabels := target.GetLabels()
				if targetLabels == nil {
					targetLabels = map[string]string{}
				}

				targetLabels[quotaLabel] = quota.Name
				targetLabels[typeLabel] = index.String()

				target.SetLabels(targetLabels)
				target.Spec.Scopes = resQuota.Scopes
				target.Spec.ScopeSelector = resQuota.ScopeSelector

				initialValues, err := quota.GetQuotaSpace(index)
				if err != nil {
					return err
				}

				r.Log.Info("Resource Quota sync result", "initial", initialValues, "name", target.Name, "namespace", target.Namespace)

				// This is important when a resourcequota is newly added (new namespace)
				// We don't want to have a racing condition and wait until the elements are synced to
				// the quota. But we take what's left (or when first namespace then hard 1:1) and assign it.
				// It may be further reduced by the limits reconciler
				target.Spec.Hard = initialValues
				target.Status.Hard = initialValues

				return controllerutil.SetControllerReference(quota, target, r.Client.Scheme())
			})

			return retryErr
		})

		r.emitEvent(quota, target.GetNamespace(), res, fmt.Sprintf("Ensuring ResourceQuota %s", target.GetName()), err)

		r.Log.Info("Resource Quota sync result: "+string(res), "name", target.Name, "namespace", target.Namespace)

		if err != nil {
			return
		}
	}

	return nil
}

// Attempts to garbage collect a ResourceQuota resource.
func (r *Manager) gcResourceQuotas(ctx context.Context, quota *capsulev1beta2.GlobalResourceQuota, namespace string) error {
	// Check if the namespace still exists
	ns := &corev1.Namespace{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		if errors.IsNotFound(err) {
			r.Log.V(5).Info("Namespace does not exist, skipping garbage collection", "namespace", namespace)
			return nil
		}
		return fmt.Errorf("failed to check namespace existence: %w", err)
	}

	// Attempt to delete the ResourceQuota
	for index, _ := range quota.Spec.Items {
		target := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ItemObjectName(index, quota),
				Namespace: namespace,
			},
		}
		err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: target.GetName()}, target)
		if err != nil {
			if errors.IsNotFound(err) {
				r.Log.V(5).Info("ResourceQuota already deleted", "namespace", namespace, "name", ItemObjectName(index, quota))
				continue
			}
			return err
		}

		// Delete the ResourceQuota
		if err := r.Client.Delete(ctx, target); err != nil {
			return fmt.Errorf("failed to delete ResourceQuota %s in namespace %s: %w", ItemObjectName(index, quota), namespace, err)
		}
	}

	r.Log.Info("Deleted ResourceQuota", "namespace", namespace)
	return nil
}

// Serial ResourceQuota processing is expensive: using Go routines we can speed it up.
// In case of multiple errors these are logged properly, returning a generic error since we have to repush back the
// reconciliation loop.
func (r *Manager) resourceQuotasUpdate(ctx context.Context, resourceName corev1.ResourceName, actual resource.Quantity, toKeep sets.Set[corev1.ResourceName], limit resource.Quantity, list ...corev1.ResourceQuota) (err error) {
	group := new(errgroup.Group)

	for _, item := range list {
		rq := item

		group.Go(func() (err error) {
			found := &corev1.ResourceQuota{}
			if err = r.Get(ctx, types.NamespacedName{Namespace: rq.Namespace, Name: rq.Name}, found); err != nil {
				return
			}

			return retry.RetryOnConflict(retry.DefaultBackoff, func() (retryErr error) {
				_, retryErr = controllerutil.CreateOrUpdate(ctx, r.Client, found, func() error {
					// Updating the Resource according to the actual.Cmp result
					found.Spec.Hard = rq.Spec.Hard

					return nil
				})

				return retryErr
			})
		})
	}

	if err = group.Wait(); err != nil {
		// We had an error and we mark the whole transaction as failed
		// to process it another time according to the Tenant controller back-off factor.
		r.Log.Error(err, "Cannot update outer ResourceQuotas", "resourceName", resourceName.String())
		err = fmt.Errorf("update of outer ResourceQuota items has failed: %w", err)
	}

	return err
}
