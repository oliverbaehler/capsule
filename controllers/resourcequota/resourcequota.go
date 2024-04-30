package resourcequota

import (
	"context"
	"fmt"

	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	quota "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/projectcapsule/capsule/pkg/api"
)

func (r *Controller) syncResourceQuota(ctx context.Context, origin *capsulev1beta2.TenantResourceQuota, nsList []corev1.Namespace) (err error) {
	statusLimitsDirty := !apiequality.Semantic.DeepEqual(origin.Spec.ResourceQuota.Hard, origin.Status.Quota.Hard)

	// dirty tracks if the usage status differs from the previous sync,
	// if so, we send a new usage with latest status
	// if this is our first sync, it will be dirty by default, since we need track usage
	dirty := statusLimitsDirty || origin.Spec.ResourceQuota.Hard == nil || origin.Status.Quota.Hard == nil

	used := v1.ResourceList{}
	if origin.Status.Quota.Used != nil {
		used = quota.Add(v1.ResourceList{}, origin.Status.Quota.Used)
	}
	hardLimits := quota.Add(v1.ResourceList{}, origin.Spec.ResourceQuota.Hard)

	var errs []error

	newUsage := v1.ResourceList{}
	group := new(errgroup.Group)
	//for _, namespace := range nsList {
	//	group.Go(func() error {
	//		calc := v1.ResourceList{}
	//		calc, err = quota.CalculateUsage(namespace.GetName(), origin.Spec.ResourceQuota.Scopes, hardLimits, r.Options.Registry, origin.Spec.ResourceQuota.ScopeSelector)
	//		if err != nil {
	//			return err
	//		}
	//		newUsage = quota.Add(newUsage, calc)
	//
	//		return nil
	//	})
	//}

	for key, value := range newUsage {
		used[key] = value
	}

	hardResources := quota.ResourceNames(hardLimits)
	used = quota.Mask(used, hardResources)

	// Create a usage object that is based on the quota resource version that will handle updates
	// by default, we preserve the past usage observation, and set hard to the current spec
	usage := origin.DeepCopy()
	usage.Status.Quota = v1.ResourceQuotaStatus{
		Hard: hardLimits,
		Used: used,
	}

	// Replicate Quotas to selected namespaces
	for _, ns := range nsList {
		namespace := ns
		group.Go(func() error {
			return r.syncResourceQuotaToNamespace(ctx, usage, namespace)
		})
	}

	dirty = dirty || !quota.Equals(usage.Status.Quota.Used, origin.Status.Quota.Used)

	// there was a change observed by this controller that requires we update quota
	if dirty {
		err = r.Client.Status().Update(context.Background(), usage)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return utilerrors.NewAggregate(errs)
}

//nolint:nakedret
func (r *Controller) syncResourceQuotaToNamespace(ctx context.Context, origin *capsulev1beta2.TenantResourceQuota, namespace corev1.Namespace) (err error) {
	// Pruning resource of non-requested resources
	//if err = r.pruningResources(ctx, namespace, keys, &corev1.ResourceQuota{}); err != nil {
	//	return err
	//}

	target := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("capsule-%s", origin.Name),
			Namespace: namespace.GetName(),
		},
	}

	var res controllerutil.OperationResult
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() (retryErr error) {
		res, retryErr = controllerutil.CreateOrUpdate(ctx, r.Client, target, func() (err error) {
			targetLabels := target.GetLabels()
			if targetLabels == nil {
				targetLabels = map[string]string{}
			}

			targetLabels[api.ClusterResourceQuotaLabel] = origin.Name

			target.Spec = *origin.Spec.ResourceQuota.DeepCopy()

			return controllerutil.SetControllerReference(origin, target, r.Client.Scheme())
		})

		return retryErr
	})

	//r.emitEvent(origin, target.GetNamespace(), res, fmt.Sprintf("Ensuring ResourceQuota %s", target.GetName()), err)

	r.Log.Info("Resource Quota sync result: "+string(res), "name", target.Name, "namespace", target.Namespace)

	if err != nil {
		return
	}

	return nil
}
