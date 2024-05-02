package resourcequota

import (
	"context"

	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Controller) statusPhaseTransition(ctx context.Context, origin *capsulev1beta2.TenantResourceQuota, phase capsulev1beta2.TenantResourceQuotaPhase) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, origin.DeepCopy(), func() error {
			origin.Status.Phase = phase

			return r.Client.Status().Update(ctx, origin, &client.SubResourceUpdateOptions{})
		})

		return
	})
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
