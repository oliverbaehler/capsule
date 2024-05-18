package resourcequota

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/projectcapsule/capsule/pkg/api"
	"github.com/projectcapsule/capsule/pkg/configuration"
	capsulewebhook "github.com/projectcapsule/capsule/pkg/webhook"
	"github.com/projectcapsule/capsule/pkg/webhook/utils"
	corev1 "k8s.io/api/core/v1"
)

var (
	managedLabels = []string{
		"capsule.clastix.io/managed",
		api.ClusterResourceQuotaLabel,
	}
)

type validationhandler struct {
	cfg     configuration.Configuration
	version *version.Version
}

func ValidationHandler(cfg configuration.Configuration, version *version.Version) capsulewebhook.Handler {
	return &validationhandler{
		cfg:     cfg,
		version: version,
	}
}

func (h *validationhandler) OnCreate(client client.Client, decoder admission.Decoder, recorder record.EventRecorder) capsulewebhook.Func {
	return func(ctx context.Context, req admission.Request) *admission.Response {
		return nil
	}
}

func (h *validationhandler) OnDelete(client client.Client, decoder admission.Decoder, recorder record.EventRecorder) capsulewebhook.Func {
	return func(ctx context.Context, req admission.Request) *admission.Response {
		return h.handle(ctx, req, client, decoder, recorder)
	}
}

func (h *validationhandler) OnUpdate(client client.Client, decoder admission.Decoder, recorder record.EventRecorder) capsulewebhook.Func {
	return func(ctx context.Context, req admission.Request) *admission.Response {
		return nil
	}
}

func (h *validationhandler) handle(_ context.Context, req admission.Request, _ client.Client, decoder admission.Decoder, _ record.EventRecorder) (response *admission.Response) {
	res := admission.Denied(fmt.Sprintf("User:" + req.UserInfo.String() + " Managed ResourceQuota can not be modified"))
	response = &res
	return

	if req.Resource == (metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "resourcequotas"}) {
		quota := &corev1.ResourceQuota{}
		if err := decoder.Decode(req, quota); err != nil {
			return utils.ErroredResponse(err)
		}

		for _, value := range managedLabels {
			if _, ok := quota.GetLabels()[value]; !ok {
				res := admission.Denied(fmt.Sprintf("Managed ResourceQuota can not be modified"))
				response = &res

				break
			}
		}
	}

	if response == nil {
		skip := admission.Allowed("")

		response = &skip
	}

	return
}
