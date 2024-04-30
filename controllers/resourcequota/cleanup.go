package resourcequota

import (
	"context"

	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
	"github.com/projectcapsule/capsule/pkg/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Controller) cleanup(ctx context.Context, origin *capsulev1beta2.TenantResourceQuota, ns corev1.Namespace) (err error) {
	selector := labels.NewSelector()

	var exists *labels.Requirement

	if exists, err = labels.NewRequirement(api.ClusterResourceQuotaLabel, selection.Equals, []string{origin.GetName()}); err != nil {
		return
	}

	selector = selector.Add(*exists)

	r.Log.Info("Pruning quotas with label selector " + selector.String())
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Client.DeleteAllOf(ctx, &corev1.ResourceQuota{}, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				LabelSelector: selector,
				Namespace:     ns.GetName(),
			},
			DeleteOptions: client.DeleteOptions{},
		})
	})

}
