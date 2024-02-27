package gateway

import (
	"context"

	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	capsulev1beta2 "github.com/projectcapsule/capsule/api/v1beta2"
)

func TenantFromGateway(ctx context.Context, c client.Client, gateway gwapiv1.Gateway) (*capsulev1beta2.Tenant, error) {
	tenantList := &capsulev1beta2.TenantList{}
	if err := c.List(ctx, tenantList, client.MatchingFieldsSelector{
		Selector: fields.OneTermEqualSelector(".status.namespaces", gateway.GetNamespace()),
	}); err != nil {
		return nil, err
	}

	if len(tenantList.Items) == 0 {
		return nil, nil //nolint:nilnil
	}

	return &tenantList.Items[0], nil
}
