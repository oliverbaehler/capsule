package pod

import (
	"github.com/projectcapsule/capsule/pkg/api"
	corev1 "k8s.io/api/core/v1"
)

func overwriteSchedulingOptions(pod *corev1.Pod, scheduling api.SchedulingOptions) {
	nodeselector := scheduling.NodeSelector
	if nodeselector != nil {
		pod.Spec.NodeSelector = nodeselector
	}

	tolerations := scheduling.Tolerations
	if tolerations != nil {
		pod.Spec.Tolerations = tolerations
	}

	topologies := scheduling.TopologySpreadConstraints
	if topologies != nil {
		pod.Spec.TopologySpreadConstraints = topologies
	}

	affinity := scheduling.Affinity
	if affinity.Size() != 0 {
		pod.Spec.Affinity = &affinity
	}

	return
}

func aggregateSchedulingOptions(pod *corev1.Pod, scheduling api.SchedulingOptions) {
	nodeselector := scheduling.NodeSelector
	if nodeselector != nil {
		for k, v := range nodeselector {
			pod.Spec.NodeSelector[k] = v
		}
	}

	tolerations := scheduling.Tolerations
	if tolerations != nil {
		// Merge tolerations
		pod.Spec.Tolerations = append(pod.Spec.Tolerations, tolerations...)
	}

	topologies := scheduling.TopologySpreadConstraints
	if topologies != nil {
		pod.Spec.TopologySpreadConstraints = append(pod.Spec.TopologySpreadConstraints, topologies...)
	}

	return
}
