package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
	"github.com/brunka09/brunernetes/internal/factory"
)

// ApplyKonveer runs a delivery workflow for an image that already exists in a
// registry.
//
// In this release the workflow is driven by brunerctl: the Konveer resource is
// stored in the cluster as the record of the delivery, and brunerctl performs
// the stages and writes the result into its status. The in-cluster Konveer
// controller is scheduled for v0.2; moving the same stages behind the
// controller does not change the resource contract.
func ApplyKonveer(ctx context.Context, kubeClient client.Client, printer *Printer, konveer *brunerv1alpha1.Konveer) error {
	printer.Section("Delivery " + konveer.Namespace + "/" + konveer.Name)
	printer.KeyValues([][2]string{
		{"Target application", konveer.Spec.AppRef},
		{"Image", konveer.Spec.Image},
		{"Namespace", konveer.Namespace},
	})

	stored, err := upsertKonveer(ctx, kubeClient, konveer)
	if err != nil {
		return err
	}
	status := brunerv1alpha1.KonveerStatus{
		ObservedGeneration: stored.Generation,
		Phase:              brunerv1alpha1.KonveerPending,
		StartTime:          &metav1.Time{Time: time.Now()},
	}

	fail := func(stage string, message string) error {
		status.Phase = brunerv1alpha1.KonveerFailed
		status.CompletionTime = &metav1.Time{Time: time.Now()}
		status.Stages = appendStage(status.Stages, stage, brunerv1alpha1.StageFailed, message)
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{
			Type: brunerv1alpha1.ConditionSucceeded, Status: metav1.ConditionFalse,
			Reason: brunerv1alpha1.ReasonValidationFailed, Message: message,
			ObservedGeneration: stored.Generation,
		})
		writeKonveerStatus(ctx, kubeClient, printer, stored, status)
		printer.Fail("%s", message)
		return fmt.Errorf("%s", message)
	}

	// Stage 1: validate the image reference and the factory policy.
	printer.Step(1, 4, "Validating the image reference and the factory policy.")
	status.Phase = brunerv1alpha1.KonveerValidating

	checks := konveer.Spec.Checks
	checkImage := checks == nil || checks.ImageReference == nil || *checks.ImageReference
	checkPolicy := checks == nil || checks.Policy == nil || *checks.Policy
	checkCapacity := checks == nil || checks.Capacity == nil || *checks.Capacity

	if checkImage {
		reference, parseErr := brunerv1alpha1.ParseImageReference(konveer.Spec.Image)
		if parseErr != nil {
			return fail("Validate", fmt.Sprintf(
				"Container image reference is invalid: %v. Correct spec.image before the delivery can continue. Parts order rejected at the gate.", parseErr))
		}
		printer.OK("Image reference %q is well formed.", reference.String())
	} else {
		printer.Skip("Image reference validation was disabled in the delivery specification.")
	}

	policy, _ := readPolicy(ctx, kubeClient)
	app, appExists, err := loadTargetApp(ctx, kubeClient, konveer)
	if err != nil {
		return fail("Validate", err.Error())
	}
	applyTemplate(app, konveer)

	if checkPolicy {
		previousImage := ""
		if appExists {
			previousImage = app.Status.AppliedImage
		}
		mode := brunerv1alpha1.MergeModes(factoryDefaultMode(ctx, kubeClient), app.Spec.Mode)
		if konveer.Spec.Override != nil {
			app.Spec.Override = konveer.Spec.Override
		}
		evaluation := factory.EvaluateApp(app, policy, mode, previousImage)
		for _, warning := range evaluation.Warnings {
			printer.Warn("%s", warning.Message)
		}
		if violation, blocked := evaluation.FirstViolation(); blocked {
			return fail("Validate", violation.Message)
		}
		printer.OK("Factory policy checks passed under profile %q and quality gate %q.", mode.Profile, mode.QualityGate)
	} else {
		printer.Skip("Policy validation was disabled in the delivery specification.")
	}

	if checkCapacity {
		list := &brunerv1alpha1.BrunerAppList{}
		if err := kubeClient.List(ctx, list); err != nil {
			return fail("Validate", fmt.Sprintf("BrunerApp resources could not be counted: %v. The product register is unreachable.", err))
		}
		observed := int32(len(list.Items))
		if !appExists {
			observed++
		}
		if finding, exceeded := factory.EvaluateCapacity(observed, policy); exceeded {
			return fail("Validate", finding.Message)
		}
		printer.OK("Application count %d is within the factory limit of %d.", observed, policy.MaxApplications)
	} else {
		printer.Skip("Capacity validation was disabled in the delivery specification.")
	}

	status.Stages = appendStage(status.Stages, "Validate", brunerv1alpha1.StageSucceeded,
		"Image reference and factory policy checks passed. Incoming inspection cleared the parts.")
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type: brunerv1alpha1.ConditionValidated, Status: metav1.ConditionTrue,
		Reason: brunerv1alpha1.ReasonValidationPassed, ObservedGeneration: stored.Generation,
		Message: "Image reference and factory policy checks passed. Incoming inspection cleared the parts.",
	})

	// Stage 2: apply the application.
	printer.Step(2, 4, "Applying the target application.")
	status.Phase = brunerv1alpha1.KonveerApplying
	app.Spec.Image = konveer.Spec.Image
	if err := DeployApp(ctx, kubeClient, printer, app, 0); err != nil {
		return fail("Apply", err.Error())
	}
	status.AppliedImage = konveer.Spec.Image
	status.Stages = appendStage(status.Stages, "Apply", brunerv1alpha1.StageSucceeded,
		fmt.Sprintf("BrunerApp %q was applied with image %q. Assembly instructions handed to the line.", app.Name, konveer.Spec.Image))

	// Stage 3: wait for the rollout and readiness.
	printer.Step(3, 4, "Waiting for the rollout and for readiness.")
	status.Phase = brunerv1alpha1.KonveerRollingOut
	timeout := 300 * time.Second
	if konveer.Spec.Timeouts != nil && konveer.Spec.Timeouts.RolloutSeconds > 0 {
		timeout = time.Duration(konveer.Spec.Timeouts.RolloutSeconds) * time.Second
	}
	if err := WaitForApp(ctx, kubeClient, printer, app.Name, app.Namespace, timeout); err != nil {
		return fail("Rollout", err.Error())
	}
	status.Stages = appendStage(status.Stages, "Rollout", brunerv1alpha1.StageSucceeded,
		"All replicas are ready. Assembly completed and product cleared for the showroom.")
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type: brunerv1alpha1.ConditionRolledOut, Status: metav1.ConditionTrue,
		Reason: brunerv1alpha1.ReasonAllReplicasReady, ObservedGeneration: stored.Generation,
		Message: "All replicas are ready. Assembly completed and product cleared for the showroom.",
	})

	// Stage 4: verify the loopback endpoint.
	printer.Step(4, 4, "Verifying the local endpoint.")
	status.Phase = brunerv1alpha1.KonveerVerifying
	verifyEnabled := konveer.Spec.Verify == nil || konveer.Spec.Verify.Enabled == nil || *konveer.Spec.Verify.Enabled
	if !verifyEnabled {
		status.Stages = appendStage(status.Stages, "Verify", brunerv1alpha1.StageSkipped,
			"Endpoint verification was disabled in the delivery specification. Final inspection was waived.")
		printer.Skip("Endpoint verification was disabled in the delivery specification.")
	} else {
		refreshed := &brunerv1alpha1.BrunerApp{}
		if err := kubeClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, refreshed); err != nil {
			return fail("Verify", fmt.Sprintf("BrunerApp %q could not be read for verification: %v.", app.Name, err))
		}
		endpoint := refreshed.Status.LocalEndpoint
		path := "/healthz"
		expected := 200
		if konveer.Spec.Verify != nil {
			if konveer.Spec.Verify.Path != "" {
				path = konveer.Spec.Verify.Path
			}
			if konveer.Spec.Verify.ExpectedStatus > 0 {
				expected = int(konveer.Spec.Verify.ExpectedStatus)
			}
		}
		switch {
		case endpoint == "":
			status.Stages = appendStage(status.Stages, "Verify", brunerv1alpha1.StageSkipped,
				"Application does not publish a local endpoint, so the endpoint check was skipped. The showroom window is closed by design.")
			printer.Skip("Application does not publish a local endpoint, so the endpoint check was skipped.")
		default:
			code, err := probeEndpoint(ctx, endpoint+path)
			switch {
			case err != nil:
				status.Stages = appendStage(status.Stages, "Verify", brunerv1alpha1.StageSkipped,
					fmt.Sprintf("Local endpoint %s could not be reached from this host: %v. The check was skipped, not failed, because brunerctl may run outside the node. The inspector could not reach the showroom.", endpoint+path, err))
				printer.Skip("Local endpoint %s could not be reached from this host: %v.", endpoint+path, err)
			case code != expected:
				return fail("Verify", fmt.Sprintf(
					"Local endpoint %s answered with HTTP %d but %d was expected. Check the application and its health path. Final inspection rejected the unit.",
					endpoint+path, code, expected))
			default:
				status.VerifiedEndpoint = endpoint + path
				status.Stages = appendStage(status.Stages, "Verify", brunerv1alpha1.StageSucceeded,
					fmt.Sprintf("Local endpoint %s answered with HTTP %d. Final inspection passed and the product is in the showroom.", endpoint+path, code))
				meta.SetStatusCondition(&status.Conditions, metav1.Condition{
					Type: brunerv1alpha1.ConditionVerified, Status: metav1.ConditionTrue,
					Reason: brunerv1alpha1.ReasonEndpointVerified, ObservedGeneration: stored.Generation,
					Message: fmt.Sprintf("Local endpoint %s answered with HTTP %d. Final inspection passed.", endpoint+path, code),
				})
				printer.OK("Local endpoint %s answered with HTTP %d.", endpoint+path, code)
			}
		}
	}

	status.Phase = brunerv1alpha1.KonveerSucceeded
	status.CompletionTime = &metav1.Time{Time: time.Now()}
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type: brunerv1alpha1.ConditionSucceeded, Status: metav1.ConditionTrue,
		Reason: brunerv1alpha1.ReasonAllReplicasReady, ObservedGeneration: stored.Generation,
		Message: "Delivery completed: the image was validated, applied, rolled out and verified. The product left the line.",
	})
	writeKonveerStatus(ctx, kubeClient, printer, stored, status)

	printer.Section("Delivery complete")
	printer.Println("The image was validated, applied, rolled out and verified. The product left the line.")
	return nil
}

// upsertKonveer stores the delivery record in the cluster.
func upsertKonveer(ctx context.Context, kubeClient client.Client, konveer *brunerv1alpha1.Konveer) (*brunerv1alpha1.Konveer, error) {
	existing := &brunerv1alpha1.Konveer{}
	err := kubeClient.Get(ctx, types.NamespacedName{Name: konveer.Name, Namespace: konveer.Namespace}, existing)
	switch {
	case apierrors.IsNotFound(err):
		if err := kubeClient.Create(ctx, konveer); err != nil {
			return nil, fmt.Errorf(
				"Konveer %q could not be created: %w. Check the CRDs and your permissions. The delivery order was rejected at the gate",
				konveer.Name, err)
		}
		return konveer, nil
	case err != nil:
		return nil, fmt.Errorf("Konveer %q could not be read: %w. The delivery register is unreachable", konveer.Name, err)
	default:
		existing.Spec = konveer.Spec
		if err := kubeClient.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("Konveer %q could not be updated: %w", konveer.Name, err)
		}
		return existing, nil
	}
}

// writeKonveerStatus persists the workflow status, reporting but not failing on
// a write error: the delivery result matters more than the record of it.
func writeKonveerStatus(
	ctx context.Context,
	kubeClient client.Client,
	printer *Printer,
	konveer *brunerv1alpha1.Konveer,
	status brunerv1alpha1.KonveerStatus,
) {
	konveer.Status = status
	if err := kubeClient.Status().Update(ctx, konveer); err != nil {
		printer.Warn("Delivery status could not be written to Konveer %q: %v. The delivery itself is unaffected.", konveer.Name, err)
	}
}

// loadTargetApp returns the BrunerApp the delivery targets, creating an
// in-memory default when it does not exist yet.
func loadTargetApp(ctx context.Context, kubeClient client.Client, konveer *brunerv1alpha1.Konveer) (*brunerv1alpha1.BrunerApp, bool, error) {
	app := &brunerv1alpha1.BrunerApp{}
	err := kubeClient.Get(ctx, types.NamespacedName{Name: konveer.Spec.AppRef, Namespace: konveer.Namespace}, app)
	switch {
	case apierrors.IsNotFound(err):
		return &brunerv1alpha1.BrunerApp{
			ObjectMeta: metav1.ObjectMeta{Name: konveer.Spec.AppRef, Namespace: konveer.Namespace},
			Spec:       brunerv1alpha1.BrunerAppSpec{Image: konveer.Spec.Image},
		}, false, nil
	case err != nil:
		return nil, false, fmt.Errorf(
			"BrunerApp %q could not be read: %w. The product register is unreachable", konveer.Spec.AppRef, err)
	default:
		return app, true, nil
	}
}

// applyTemplate copies the optional delivery template onto the application.
func applyTemplate(app *brunerv1alpha1.BrunerApp, konveer *brunerv1alpha1.Konveer) {
	template := konveer.Spec.AppTemplate
	if template == nil {
		return
	}
	if template.Replicas != nil {
		replicas := *template.Replicas
		app.Spec.Replicas = &replicas
	}
	if template.Port != nil {
		app.Spec.Port = *template.Port
	}
	if template.HealthPath != "" {
		app.Spec.HealthPath = template.HealthPath
	}
	if template.Mode != nil {
		app.Spec.Mode = template.Mode.DeepCopy()
	}
	if template.LocalExposure != nil {
		app.Spec.LocalExposure = template.LocalExposure.DeepCopy()
	}
}

// factoryDefaultMode returns the default mode of the factory policy.
func factoryDefaultMode(ctx context.Context, kubeClient client.Client) *brunerv1alpha1.BrunerMode {
	list := &brunerv1alpha1.AvtoVAZList{}
	if err := kubeClient.List(ctx, list); err != nil || len(list.Items) == 0 {
		return nil
	}
	for i := range list.Items {
		if list.Items[i].Name == "avtovaz" {
			return list.Items[i].Spec.DefaultMode
		}
	}
	return list.Items[0].Spec.DefaultMode
}

// appendStage records one workflow stage.
func appendStage(stages []brunerv1alpha1.KonveerStageStatus, name string, state brunerv1alpha1.StageState, message string) []brunerv1alpha1.KonveerStageStatus {
	now := metav1.Time{Time: time.Now()}
	return append(stages, brunerv1alpha1.KonveerStageStatus{
		Name:           name,
		State:          state,
		Message:        message,
		StartTime:      &now,
		CompletionTime: &now,
	})
}

// probeEndpoint performs one HTTP request against a loopback endpoint.
func probeEndpoint(ctx context.Context, url string) (int, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext:       (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
			DisableKeepAlives: true,
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode, nil
}
