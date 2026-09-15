package factory

import (
	"fmt"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

// WorkloadState is the observed state of the Deployment owned by an
// application, plus the few pod level facts that explain a stalled rollout.
// The controller collects it; AssessWorkload turns it into a verdict.
type WorkloadState struct {
	// DesiredReplicas is the replica count requested by the specification.
	DesiredReplicas int32
	// Replicas is the number of replicas created by the Deployment.
	Replicas int32
	// UpdatedReplicas is the number of replicas running the current template.
	UpdatedReplicas int32
	// ReadyReplicas is the number of replicas that passed readiness.
	ReadyReplicas int32
	// AvailableReplicas is the number of replicas that stayed ready long
	// enough to count as available.
	AvailableReplicas int32
	// ProgressDeadlineExceeded is set when the Deployment reported that its
	// rollout stopped making progress.
	ProgressDeadlineExceeded bool
	// ImagePullFailure carries the container status reason when the image
	// cannot be pulled, for example "ImagePullBackOff" or "ErrImagePull".
	ImagePullFailure string
	// CrashLoop is set when at least one container is restarting in a loop.
	CrashLoop bool
	// ReadinessFailing is set when replicas are running but not ready.
	ReadinessFailing bool
	// SchedulingBlocked carries the scheduler message when a pod cannot be
	// placed on the node.
	SchedulingBlocked string
}

// Assessment is the verdict about a workload, ready to be written into status
// conditions and events.
type Assessment struct {
	// Stage is the themed progress stage.
	Stage brunerv1alpha1.AssemblyStage
	// Available reports whether every desired replica is ready.
	Available bool
	// Degraded reports whether a technical fault blocks progress.
	Degraded bool
	// Reason is the machine readable condition reason.
	Reason string
	// Message states the technical condition first and the themed wording
	// second. It is used for both the condition message and the event message.
	Message string
}

// AssessWorkload turns observed workload state into a single verdict. The
// order of the checks is the order in which a human would debug the workload:
// scheduling, image, crash loop, readiness, replica count.
func AssessWorkload(state WorkloadState) Assessment {
	if state.DesiredReplicas == 0 {
		return Assessment{
			Stage:     brunerv1alpha1.StageInspection,
			Available: false,
			Degraded:  false,
			Reason:    brunerv1alpha1.ReasonReplicasUnavailable,
			Message:   "Desired replica count is zero, so no pods are expected. Scale the application up to start production. The line is idle by request.",
		}
	}

	if state.SchedulingBlocked != "" {
		return Assessment{
			Stage:     brunerv1alpha1.StageLineStopped,
			Available: false,
			Degraded:  true,
			Reason:    brunerv1alpha1.ReasonReplicasUnavailable,
			Message: fmt.Sprintf(
				"Pod cannot be scheduled: %s. Check node resources, taints and host port conflicts. No free bay on the factory floor.",
				state.SchedulingBlocked),
		}
	}

	if state.ImagePullFailure != "" {
		return Assessment{
			Stage:     brunerv1alpha1.StageLineStopped,
			Available: false,
			Degraded:  true,
			Reason:    brunerv1alpha1.ReasonImagePullFailure,
			Message: fmt.Sprintf(
				"Container image could not be pulled (%s). Verify image reference and registry access. Parts delivery failed.",
				state.ImagePullFailure),
		}
	}

	if state.CrashLoop {
		return Assessment{
			Stage:     brunerv1alpha1.StageLineStopped,
			Available: false,
			Degraded:  true,
			Reason:    brunerv1alpha1.ReasonProbeFailure,
			Message:   "Container is restarting repeatedly. Inspect container logs and the liveness probe configuration. The unit keeps falling off the line.",
		}
	}

	if state.ProgressDeadlineExceeded {
		return Assessment{
			Stage:     brunerv1alpha1.StageLineStopped,
			Available: false,
			Degraded:  true,
			Reason:    brunerv1alpha1.ReasonRolloutTimeout,
			Message:   "Deployment exceeded its progress deadline. Inspect pod events and container logs for the stalled replica. The conveyor stopped mid-shift.",
		}
	}

	if state.ReadyReplicas >= state.DesiredReplicas && state.UpdatedReplicas >= state.DesiredReplicas {
		return Assessment{
			Stage:     brunerv1alpha1.StageShowroom,
			Available: true,
			Degraded:  false,
			Reason:    brunerv1alpha1.ReasonAllReplicasReady,
			Message: fmt.Sprintf(
				"All %d replicas are ready. Assembly completed and product cleared for the showroom.",
				state.DesiredReplicas),
		}
	}

	// A replica that is running but has not passed its readiness probe is a
	// quality control hold, not an assembly step. Partial readiness during a
	// normal rollout is reported as assembly below.
	if state.ReadinessFailing {
		return Assessment{
			Stage:     brunerv1alpha1.StageQualityControl,
			Available: false,
			Degraded:  false,
			Reason:    brunerv1alpha1.ReasonProbeFailure,
			Message: fmt.Sprintf(
				"Readiness probe has not succeeded for %d of %d replicas. Final inspection is not complete.",
				state.DesiredReplicas-state.ReadyReplicas, state.DesiredReplicas),
		}
	}

	return Assessment{
		Stage:     brunerv1alpha1.StageAssembly,
		Available: false,
		Degraded:  false,
		Reason:    brunerv1alpha1.ReasonReconciling,
		Message: fmt.Sprintf(
			"Deployment reports %d of %d replicas created and %d ready. Assembly is in progress.",
			state.Replicas, state.DesiredReplicas, state.ReadyReplicas),
	}
}
