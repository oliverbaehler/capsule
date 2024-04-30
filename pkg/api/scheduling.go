package api

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	SchedulingOverwrite SchedulingAction = "overwrite"
	SchedulingAggregate SchedulingAction = "aggregate"
)

// +kubebuilder:validation:Enum=overwrite;aggregate
type SchedulingAction string

func (p SchedulingAction) String() string {
	return string(p)
}

// +kubebuilder:object:generate=true
type SchedulingOptions struct {
	// Specify Action for defined Scheduling options
	//+kubebuilder:default=overwrite
	Action SchedulingAction `json:"action"`
	// Specify Selector for selecting the pods
	//Condition SchedulingSelector `json:"selector,omitempty"`
	// Allow Specifying Nodeselectors for the pod
	Affinity corev1.Affinity `json:"affinity,omitempty"`
	// Allow Specifying Tolerations for the pod
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// Allow Specifying TopologySpreadConstraints for the pod
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	// Allow Specifying NodeSelector for the pod (directly applied to the pod)
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}
