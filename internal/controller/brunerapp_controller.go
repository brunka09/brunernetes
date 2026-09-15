// Package controller contains the Brunernetes controllers. Only the BrunerApp
// controller is implemented in the MVP; the Togliatti, AvtoVAZ and Konveer
// reconcilers are specified in docs/reconciliation.md and land in v0.2.
package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	brunerv1alpha1 "github.com/brunka09/brunernetes/api/v1alpha1"
	"github.com/brunka09/brunernetes/internal/factory"
)

// Requeue cadence. The night shift is quieter on purpose: it resyncs less often
// so a machine left running overnight does not churn.
const (
	requeueDayShift   = 30 * time.Second
	requeueNightShift = 2 * time.Minute
	requeueOnFault    = 15 * time.Second
)

// BrunerAppReconciler reconciles a BrunerApp into one Deployment and one
// ClusterIP Service, and publishes the application on the loopback interface.
type BrunerAppReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=bruner.dev,resources=brunerapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bruner.dev,resources=brunerapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=bruner.dev,resources=brunerapps/finalizers,verbs=update
// +kubebuilder:rbac:groups=bruner.dev,resources=avtovazs,verbs=get;list;watch
// +kubebuilder:rbac:groups=bruner.dev,resources=avtovazs/status,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile converges the cluster towards the desired state of one BrunerApp.
// The function is idempotent: running it twice with the same inputs produces
// the same objects and the same status.
func (r *BrunerAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	app := &brunerv1alpha1.BrunerApp{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		// A deleted application takes its Deployment and Service with it
		// through owner references, so there is nothing left to clean up.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !app.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	policy, factoryMode := r.factoryPolicy(ctx)
	mode := brunerv1alpha1.MergeModes(factoryMode, app.Spec.Mode)
	if err := brunerv1alpha1.ValidateMode(mode); err != nil {
		message := fmt.Sprintf(
			"Operating mode is invalid: %v. Correct spec.mode or the AvtoVAZ default mode. The shift plan does not exist.",
			err)
		r.Recorder.Event(app, corev1.EventTypeWarning, brunerv1alpha1.ReasonValidationFailed, message)
		return r.finishWithFault(ctx, app, mode, brunerv1alpha1.ReasonValidationFailed, message, nil)
	}

	status := app.Status.DeepCopy()
	status.ObservedGeneration = app.Generation
	status.EffectiveMode = mode.DeepCopy()

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               brunerv1alpha1.ConditionReconciling,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: app.Generation,
		Reason:             brunerv1alpha1.ReasonReconciling,
		Message:            "Desired state is being applied. Work order accepted at the gate.",
	})

	// 1. Evaluate factory policy before anything is written to the cluster.
	evaluation := factory.EvaluateApp(app, policy, mode, app.Status.AppliedImage)
	r.applyPolicyCondition(status, app.Generation, evaluation)
	for _, warning := range evaluation.Warnings {
		r.Recorder.Event(app, corev1.EventTypeWarning, warning.Reason, warning.Message)
	}
	if violation, blocked := evaluation.FirstViolation(); blocked {
		r.Recorder.Event(app, corev1.EventTypeWarning, violation.Reason, violation.Message)
		logger.Info("policy blocked the rollout", "reason", violation.Reason)
		return r.writeStatus(ctx, app, status, faultResult(mode), func(s *brunerv1alpha1.BrunerAppStatus) {
			s.Stage = brunerv1alpha1.StageInspection
			setCondition(s, app.Generation, brunerv1alpha1.ConditionDegraded, metav1.ConditionTrue, violation.Reason, violation.Message)
			setCondition(s, app.Generation, brunerv1alpha1.ConditionAvailable, metav1.ConditionFalse, violation.Reason, violation.Message)
			setCondition(s, app.Generation, brunerv1alpha1.ConditionReconciling, metav1.ConditionFalse, violation.Reason,
				"Reconciliation is paused until the policy problem is resolved. The line is held at inspection.")
		})
	}

	// 2. Apply the Deployment.
	deployment, deploymentOp, err := r.reconcileDeployment(ctx, app, mode)
	if err != nil {
		message := fmt.Sprintf(
			"Deployment could not be applied: %v. Check controller permissions and the API server. The assembly order was rejected.",
			err)
		r.Recorder.Event(app, corev1.EventTypeWarning, brunerv1alpha1.ReasonReconcileError, message)
		return r.finishWithFault(ctx, app, mode, brunerv1alpha1.ReasonReconcileError, message, err)
	}
	if deploymentOp != controllerutil.OperationResultNone {
		r.Recorder.Eventf(app, corev1.EventTypeNormal, "DeploymentApplied",
			"Deployment %q was %s with image %q. Assembly instructions handed to the line.",
			deployment.Name, deploymentOp, app.Spec.Image)
	}

	// 3. Apply the Service.
	service, serviceOp, err := r.reconcileService(ctx, app, mode)
	if err != nil {
		message := fmt.Sprintf(
			"Service could not be applied: %v. Check controller permissions and the API server. The delivery bay could not be opened.",
			err)
		r.Recorder.Event(app, corev1.EventTypeWarning, brunerv1alpha1.ReasonReconcileError, message)
		return r.finishWithFault(ctx, app, mode, brunerv1alpha1.ReasonReconcileError, message, err)
	}
	if serviceOp != controllerutil.OperationResultNone {
		r.Recorder.Eventf(app, corev1.EventTypeNormal, "ServiceApplied",
			"Service %q was %s. Delivery bay is open.", service.Name, serviceOp)
	}

	// 4. Observe the workload and turn it into a verdict.
	state, err := r.observeWorkload(ctx, app, deployment)
	if err != nil {
		message := fmt.Sprintf(
			"Workload state could not be read: %v. Check controller permissions for pods and deployments. The floor supervisor cannot see the line.",
			err)
		return r.finishWithFault(ctx, app, mode, brunerv1alpha1.ReasonReconcileError, message, err)
	}
	assessment := factory.AssessWorkload(state)

	status.Stage = assessment.Stage
	status.Replicas = state.Replicas
	status.ReadyReplicas = state.ReadyReplicas
	status.DeploymentName = deployment.Name
	status.ServiceName = service.Name
	status.AppliedImage = app.Spec.Image
	status.ClusterIPEndpoint = factory.ClusterIPEndpoint(app)
	status.LocalEndpoint = factory.LocalEndpoint(app)

	availableStatus := metav1.ConditionFalse
	if assessment.Available {
		availableStatus = metav1.ConditionTrue
	}
	setCondition(status, app.Generation, brunerv1alpha1.ConditionAvailable, availableStatus, assessment.Reason, assessment.Message)

	degradedStatus := metav1.ConditionFalse
	degradedReason := brunerv1alpha1.ReasonReconciling
	degradedMessage := "No blocking fault was detected. The line is running."
	if assessment.Degraded {
		degradedStatus = metav1.ConditionTrue
		degradedReason = assessment.Reason
		degradedMessage = assessment.Message
	}
	setCondition(status, app.Generation, brunerv1alpha1.ConditionDegraded, degradedStatus, degradedReason, degradedMessage)

	reconcilingStatus := metav1.ConditionTrue
	reconcilingMessage := "Desired state is still converging. Assembly is in progress."
	if assessment.Available {
		reconcilingStatus = metav1.ConditionFalse
		reconcilingMessage = "Desired state matches the observed state. The work order is complete."
	}
	setCondition(status, app.Generation, brunerv1alpha1.ConditionReconciling, reconcilingStatus, assessment.Reason, reconcilingMessage)

	// 5. Emit an event only when the verdict changed, to keep the log readable.
	r.recordAssessmentChange(app, assessment)

	result := ctrl.Result{RequeueAfter: requeueFor(mode)}
	if assessment.Degraded {
		result = ctrl.Result{RequeueAfter: requeueOnFault}
	}
	return r.writeStatus(ctx, app, status, result, nil)
}

// factoryPolicy reads the cluster-scoped AvtoVAZ policy. When no policy exists
// the factory defaults apply, so a fresh cluster still works.
func (r *BrunerAppReconciler) factoryPolicy(ctx context.Context) (brunerv1alpha1.FactoryPolicy, *brunerv1alpha1.BrunerMode) {
	list := &brunerv1alpha1.AvtoVAZList{}
	if err := r.List(ctx, list); err != nil || len(list.Items) == 0 {
		return factory.DefaultPolicy(), nil
	}
	sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].Name < list.Items[j].Name })
	selected := list.Items[0]
	for i := range list.Items {
		if list.Items[i].Name == "avtovaz" {
			selected = list.Items[i]
			break
		}
	}
	return factory.NormalizePolicy(selected.Spec.Policy), selected.Spec.DefaultMode
}

// reconcileDeployment creates or updates the owned Deployment.
func (r *BrunerAppReconciler) reconcileDeployment(
	ctx context.Context,
	app *brunerv1alpha1.BrunerApp,
	mode brunerv1alpha1.BrunerMode,
) (*appsv1.Deployment, controllerutil.OperationResult, error) {
	desired := factory.BuildDeployment(app, mode)
	existing := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
		existing.Labels = mergeStringMaps(existing.Labels, desired.Labels)
		existing.Annotations = mergeStringMaps(existing.Annotations, desired.Annotations)
		// The selector is immutable once the Deployment exists, so it is only
		// written on creation.
		if existing.CreationTimestamp.IsZero() {
			existing.Spec.Selector = desired.Spec.Selector
		}
		existing.Spec.Replicas = desired.Spec.Replicas
		existing.Spec.RevisionHistoryLimit = desired.Spec.RevisionHistoryLimit
		existing.Spec.ProgressDeadlineSeconds = desired.Spec.ProgressDeadlineSeconds
		existing.Spec.MinReadySeconds = desired.Spec.MinReadySeconds
		existing.Spec.Strategy = desired.Spec.Strategy
		existing.Spec.Template = desired.Spec.Template
		return controllerutil.SetControllerReference(app, existing, r.Scheme)
	})
	if err != nil {
		return nil, op, err
	}
	return existing, op, nil
}

// reconcileService creates or updates the owned ClusterIP Service. The assigned
// cluster IP and the service type are preserved, because both are effectively
// immutable for an existing Service.
func (r *BrunerAppReconciler) reconcileService(
	ctx context.Context,
	app *brunerv1alpha1.BrunerApp,
	mode brunerv1alpha1.BrunerMode,
) (*corev1.Service, controllerutil.OperationResult, error) {
	desired := factory.BuildService(app, mode)
	existing := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
		existing.Labels = mergeStringMaps(existing.Labels, desired.Labels)
		existing.Annotations = mergeStringMaps(existing.Annotations, desired.Annotations)
		existing.Spec.Type = corev1.ServiceTypeClusterIP
		existing.Spec.Selector = desired.Spec.Selector
		existing.Spec.Ports = desired.Spec.Ports
		return controllerutil.SetControllerReference(app, existing, r.Scheme)
	})
	if err != nil {
		return nil, op, err
	}
	return existing, op, nil
}

// observeWorkload collects the Deployment status and the pod level facts that
// explain a stalled rollout.
func (r *BrunerAppReconciler) observeWorkload(
	ctx context.Context,
	app *brunerv1alpha1.BrunerApp,
	deployment *appsv1.Deployment,
) (factory.WorkloadState, error) {
	state := factory.WorkloadState{
		DesiredReplicas:   factory.ResolvedReplicas(app),
		Replicas:          deployment.Status.Replicas,
		UpdatedReplicas:   deployment.Status.UpdatedReplicas,
		ReadyReplicas:     deployment.Status.ReadyReplicas,
		AvailableReplicas: deployment.Status.AvailableReplicas,
	}
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing &&
			condition.Status == corev1.ConditionFalse &&
			condition.Reason == "ProgressDeadlineExceeded" {
			state.ProgressDeadlineExceeded = true
		}
	}

	pods := &corev1.PodList{}
	if err := r.List(ctx, pods,
		client.InNamespace(app.Namespace),
		client.MatchingLabels(factory.SelectorLabels(app)),
	); err != nil {
		return state, err
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if !pod.DeletionTimestamp.IsZero() {
			continue
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodScheduled &&
				condition.Status == corev1.ConditionFalse &&
				condition.Reason == corev1.PodReasonUnschedulable {
				state.SchedulingBlocked = condition.Message
			}
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionFalse &&
				pod.Status.Phase == corev1.PodRunning {
				state.ReadinessFailing = true
			}
		}
		for _, containerStatus := range pod.Status.ContainerStatuses {
			waiting := containerStatus.State.Waiting
			if waiting == nil {
				continue
			}
			switch waiting.Reason {
			case "ErrImagePull", "ImagePullBackOff", "InvalidImageName", "ImageInspectError":
				state.ImagePullFailure = waiting.Reason
			case "CrashLoopBackOff":
				state.CrashLoop = true
			}
		}
	}
	return state, nil
}

// applyPolicyCondition writes the PolicyWarning condition from the evaluation.
func (r *BrunerAppReconciler) applyPolicyCondition(
	status *brunerv1alpha1.BrunerAppStatus,
	generation int64,
	evaluation factory.PolicyResult,
) {
	// PolicyEnforced answers one question: did factory policy allow this
	// application onto the line?
	if violation, blocked := evaluation.FirstViolation(); blocked {
		setCondition(status, generation, brunerv1alpha1.ConditionPolicyEnforced, metav1.ConditionFalse,
			violation.Reason, violation.Message)
	} else {
		setCondition(status, generation, brunerv1alpha1.ConditionPolicyEnforced, metav1.ConditionTrue,
			brunerv1alpha1.ReasonValidationPassed,
			"All factory policy checks passed. Incoming inspection cleared the parts.")
	}

	if len(evaluation.Warnings) == 0 {
		setCondition(status, generation, brunerv1alpha1.ConditionPolicyWarning, metav1.ConditionFalse,
			brunerv1alpha1.ReasonValidationPassed,
			"No policy deviation was detected. Quality paperwork is in order.")
		return
	}
	messages := ""
	for i, warning := range evaluation.Warnings {
		if i > 0 {
			messages += " "
		}
		messages += warning.Message
	}
	setCondition(status, generation, brunerv1alpha1.ConditionPolicyWarning, metav1.ConditionTrue,
		brunerv1alpha1.ReasonPolicyDeviation, messages)
}

// recordAssessmentChange emits an event only when the verdict differs from the
// verdict already recorded in the status, so a steady line stays quiet.
func (r *BrunerAppReconciler) recordAssessmentChange(app *brunerv1alpha1.BrunerApp, assessment factory.Assessment) {
	previous := meta.FindStatusCondition(app.Status.Conditions, brunerv1alpha1.ConditionAvailable)
	if previous != nil && previous.Reason == assessment.Reason && app.Status.Stage == assessment.Stage {
		return
	}
	eventType := corev1.EventTypeNormal
	if assessment.Degraded {
		eventType = corev1.EventTypeWarning
	}
	r.Recorder.Event(app, eventType, assessment.Reason, assessment.Message)
}

// finishWithFault records a blocking fault in the status and requeues.
func (r *BrunerAppReconciler) finishWithFault(
	ctx context.Context,
	app *brunerv1alpha1.BrunerApp,
	mode brunerv1alpha1.BrunerMode,
	reason string,
	message string,
	cause error,
) (ctrl.Result, error) {
	status := app.Status.DeepCopy()
	status.ObservedGeneration = app.Generation
	status.Stage = brunerv1alpha1.StageLineStopped
	setCondition(status, app.Generation, brunerv1alpha1.ConditionDegraded, metav1.ConditionTrue, reason, message)
	setCondition(status, app.Generation, brunerv1alpha1.ConditionAvailable, metav1.ConditionFalse, reason, message)
	setCondition(status, app.Generation, brunerv1alpha1.ConditionReconciling, metav1.ConditionFalse, reason, message)

	if _, err := r.writeStatus(ctx, app, status, ctrl.Result{}, nil); err != nil {
		return ctrl.Result{}, err
	}
	if cause != nil {
		return ctrl.Result{}, cause
	}
	return faultResult(mode), nil
}

// writeStatus persists the status subresource. The optional mutate function
// runs immediately before the write so callers can adjust the final state.
func (r *BrunerAppReconciler) writeStatus(
	ctx context.Context,
	app *brunerv1alpha1.BrunerApp,
	status *brunerv1alpha1.BrunerAppStatus,
	result ctrl.Result,
	mutate func(*brunerv1alpha1.BrunerAppStatus),
) (ctrl.Result, error) {
	if mutate != nil {
		mutate(status)
	}
	if equalStatus(&app.Status, status) {
		return result, nil
	}
	app.Status = *status
	if err := r.Status().Update(ctx, app); err != nil {
		if apierrors.IsConflict(err) {
			// Another writer won the race; reconcile again with fresh state.
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return result, nil
}

// SetupWithManager registers the controller with the manager.
func (r *BrunerAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("brunerapp-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&brunerv1alpha1.BrunerApp{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("brunerapp").
		Complete(r)
}

// requeueFor returns the periodic resync interval for the effective mode.
func requeueFor(mode brunerv1alpha1.BrunerMode) time.Duration {
	if mode.Shift == brunerv1alpha1.ShiftNight {
		return requeueNightShift
	}
	return requeueDayShift
}

// faultResult returns the requeue result used after a blocking fault.
func faultResult(mode brunerv1alpha1.BrunerMode) ctrl.Result {
	_ = mode
	return ctrl.Result{RequeueAfter: requeueOnFault}
}

// setCondition writes one condition with the observed generation attached.
func setCondition(
	status *brunerv1alpha1.BrunerAppStatus,
	generation int64,
	conditionType string,
	state metav1.ConditionStatus,
	reason string,
	message string,
) {
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             state,
		ObservedGeneration: generation,
		Reason:             reason,
		Message:            truncateMessage(message),
	})
}

// truncateMessage keeps a condition message inside the API server limit.
func truncateMessage(message string) string {
	const limit = 32 * 1024
	if len(message) <= limit {
		return message
	}
	return message[:limit-3] + "..."
}

// mergeStringMaps copies the desired entries over the existing map without
// dropping labels or annotations another tool added.
func mergeStringMaps(existing, desired map[string]string) map[string]string {
	if existing == nil {
		existing = map[string]string{}
	}
	for key, value := range desired {
		existing[key] = value
	}
	return existing
}

// equalStatus compares two statuses while ignoring condition timestamps, so an
// unchanged status does not produce a write on every resync.
func equalStatus(current, desired *brunerv1alpha1.BrunerAppStatus) bool {
	if current.ObservedGeneration != desired.ObservedGeneration ||
		current.Stage != desired.Stage ||
		current.Replicas != desired.Replicas ||
		current.ReadyReplicas != desired.ReadyReplicas ||
		current.DeploymentName != desired.DeploymentName ||
		current.ServiceName != desired.ServiceName ||
		current.AppliedImage != desired.AppliedImage ||
		current.ClusterIPEndpoint != desired.ClusterIPEndpoint ||
		current.LocalEndpoint != desired.LocalEndpoint {
		return false
	}
	if (current.EffectiveMode == nil) != (desired.EffectiveMode == nil) {
		return false
	}
	if current.EffectiveMode != nil && *current.EffectiveMode != *desired.EffectiveMode {
		return false
	}
	if len(current.Conditions) != len(desired.Conditions) {
		return false
	}
	for _, condition := range desired.Conditions {
		existing := meta.FindStatusCondition(current.Conditions, condition.Type)
		if existing == nil ||
			existing.Status != condition.Status ||
			existing.Reason != condition.Reason ||
			existing.Message != condition.Message ||
			existing.ObservedGeneration != condition.ObservedGeneration {
			return false
		}
	}
	return true
}
