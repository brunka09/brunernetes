package factory

import (
	"strings"
	"testing"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
)

func TestAssessWorkloadReportsTheFirstRealFault(t *testing.T) {
	cases := []struct {
		name          string
		state         WorkloadState
		wantStage     brunerv1alpha1.AssemblyStage
		wantDegraded  bool
		wantAvailable bool
		wantInMessage string
	}{
		{
			name:          "scaled to zero on purpose",
			state:         WorkloadState{DesiredReplicas: 0},
			wantStage:     brunerv1alpha1.StageInspection,
			wantInMessage: "zero",
		},
		{
			name:          "nothing created yet",
			state:         WorkloadState{DesiredReplicas: 1},
			wantStage:     brunerv1alpha1.StageAssembly,
			wantInMessage: "0 of 1",
		},
		{
			name: "scheduling is blocked",
			state: WorkloadState{
				DesiredReplicas:   1,
				Replicas:          1,
				SchedulingBlocked: "0/1 nodes are available: 1 node(s) had untolerated taint",
			},
			wantStage:     brunerv1alpha1.StageLineStopped,
			wantDegraded:  true,
			wantInMessage: "untolerated taint",
		},
		{
			name: "the image cannot be pulled",
			state: WorkloadState{
				DesiredReplicas:  1,
				Replicas:         1,
				ImagePullFailure: "ImagePullBackOff",
			},
			wantStage:     brunerv1alpha1.StageLineStopped,
			wantDegraded:  true,
			wantInMessage: "ImagePullBackOff",
		},
		{
			name: "the container crash loops",
			state: WorkloadState{
				DesiredReplicas: 1,
				Replicas:        1,
				CrashLoop:       true,
			},
			wantStage:    brunerv1alpha1.StageLineStopped,
			wantDegraded: true,
		},
		{
			name: "the rollout exceeded its deadline",
			state: WorkloadState{
				DesiredReplicas:          1,
				Replicas:                 1,
				ProgressDeadlineExceeded: true,
			},
			wantStage:    brunerv1alpha1.StageLineStopped,
			wantDegraded: true,
		},
		{
			name: "replicas are running but not ready",
			state: WorkloadState{
				DesiredReplicas:  1,
				Replicas:         1,
				UpdatedReplicas:  1,
				ReadinessFailing: true,
			},
			wantStage: brunerv1alpha1.StageQualityControl,
		},
		{
			name: "the rollout is still in progress",
			state: WorkloadState{
				DesiredReplicas: 2,
				Replicas:        2,
				UpdatedReplicas: 1,
				ReadyReplicas:   1,
			},
			wantStage: brunerv1alpha1.StageAssembly,
		},
		{
			name: "every replica is ready and available",
			state: WorkloadState{
				DesiredReplicas:   1,
				Replicas:          1,
				UpdatedReplicas:   1,
				ReadyReplicas:     1,
				AvailableReplicas: 1,
			},
			wantStage:     brunerv1alpha1.StageShowroom,
			wantAvailable: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assessment := AssessWorkload(testCase.state)
			if assessment.Stage != testCase.wantStage {
				t.Errorf("stage: got %q, want %q", assessment.Stage, testCase.wantStage)
			}
			if assessment.Degraded != testCase.wantDegraded {
				t.Errorf("degraded: got %v, want %v", assessment.Degraded, testCase.wantDegraded)
			}
			if assessment.Available != testCase.wantAvailable {
				t.Errorf("available: got %v, want %v", assessment.Available, testCase.wantAvailable)
			}
			if assessment.Reason == "" {
				t.Error("every assessment must carry a machine readable reason")
			}
			if assessment.Message == "" {
				t.Fatal("every assessment must carry a human readable message")
			}
			if testCase.wantInMessage != "" && !strings.Contains(assessment.Message, testCase.wantInMessage) {
				t.Errorf("message %q must contain %q", assessment.Message, testCase.wantInMessage)
			}
		})
	}
}

func TestAssessWorkloadNeverReportsAvailableAndDegradedTogether(t *testing.T) {
	states := []WorkloadState{
		{DesiredReplicas: 1},
		{DesiredReplicas: 1, Replicas: 1, CrashLoop: true},
		{DesiredReplicas: 1, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1},
		{DesiredReplicas: 3, Replicas: 3, UpdatedReplicas: 3, ReadyReplicas: 2, AvailableReplicas: 2},
	}
	for _, state := range states {
		assessment := AssessWorkload(state)
		if assessment.Available && assessment.Degraded {
			t.Errorf("state %+v produced a contradictory verdict", state)
		}
	}
}

func TestAssessWorkloadMessagesStateTheTechnicalConditionFirst(t *testing.T) {
	// The message must not open with themed wording: an operator reading the
	// first clause has to learn the real condition.
	themedOpeners := []string{"The factory", "The line", "The showroom", "Welcome"}
	states := []WorkloadState{
		{DesiredReplicas: 1},
		{DesiredReplicas: 1, Replicas: 1, ImagePullFailure: "ErrImagePull"},
		{DesiredReplicas: 1, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1},
	}
	for _, state := range states {
		message := AssessWorkload(state).Message
		for _, opener := range themedOpeners {
			if strings.HasPrefix(message, opener) {
				t.Errorf("message must not open with themed wording: %q", message)
			}
		}
	}
}
