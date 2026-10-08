package controller

import (
	"context"

	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	trafficv1alpha1 "github.com/neha874-ctrl/shadowcast/api/v1alpha1"
	shadowmetrics "github.com/neha874-ctrl/shadowcast/internal/metrics"
	"github.com/neha874-ctrl/shadowcast/internal/xds"
)

const TargetNodeID = "shadowcast-envoy"

// ShadowPolicyReconciler reconciles a ShadowPolicy object
type ShadowPolicyReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	SnapshotCache cachev3.SnapshotCache
}

// +kubebuilder:rbac:groups=traffic.shadowcast.io,resources=shadowpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=traffic.shadowcast.io,resources=shadowpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=traffic.shadowcast.io,resources=shadowpolicies/finalizers,verbs=update

// Reconcile handles ShadowPolicy changes and updates Envoy xDS cache.
func (r *ShadowPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	var policy trafficv1alpha1.ShadowPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ShadowPolicy resource deleted, clearing shadow configuration", "name", req.NamespacedName)

			// Handle Policy Deletion: Fallback to default snapshot without shadow cluster
			if r.SnapshotCache != nil {
				fallbackSnapshot, err := xds.BuildFallbackSnapshot("deleted")
				if err != nil {
					logger.Error(err, "Failed to build fallback snapshot on deletion")
					shadowmetrics.ReconcileTotal.WithLabelValues("error").Inc()
					return ctrl.Result{}, err
				}
				if err := r.SnapshotCache.SetSnapshot(ctx, TargetNodeID, fallbackSnapshot); err != nil {
					logger.Error(err, "Failed to clear xDS snapshot cache on deletion")
					shadowmetrics.ReconcileTotal.WithLabelValues("error").Inc()
					return ctrl.Result{}, err
				}
				logger.Info("Successfully applied fallback xDS snapshot to Envoy", "nodeID", TargetNodeID)
				shadowmetrics.XDSSnapshotUpdates.Inc()
			}

			// Record deletion in metrics
			shadowmetrics.ActivePolicies.Dec()
			shadowmetrics.ReconcileTotal.WithLabelValues("success").Inc()
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch ShadowPolicy")
		shadowmetrics.ReconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling ShadowPolicy for xDS",
		"source", policy.Spec.SourceService,
		"target", policy.Spec.TargetService,
		"mirrorPercentage", policy.Spec.MirrorPercentage,
		"resourceVersion", policy.ResourceVersion,
	)

	// Build Envoy xDS Snapshot using ResourceVersion to avoid redundant updates
	snapshot, err := xds.BuildSnapshot(policy.ResourceVersion, &policy)
	if err != nil {
		logger.Error(err, "Failed to build xDS snapshot")
		shadowmetrics.ReconcileTotal.WithLabelValues("error").Inc()
		return ctrl.Result{}, err
	}

	// Update xDS cache for Envoy node group "shadowcast-envoy"
	if r.SnapshotCache != nil {
		if err := r.SnapshotCache.SetSnapshot(ctx, TargetNodeID, snapshot); err != nil {
			logger.Error(err, "Failed to update xDS snapshot cache")
			shadowmetrics.ReconcileTotal.WithLabelValues("error").Inc()
			return ctrl.Result{}, err
		}
		logger.Info("Successfully updated Envoy xDS snapshot", "nodeID", TargetNodeID, "version", policy.ResourceVersion)
		shadowmetrics.XDSSnapshotUpdates.Inc()
	} else {
		logger.Info("SnapshotCache is nil (running in test mode), skipping xDS cache update")
	}

	// Update CRD Status if not already active
	if !policy.Status.Active {
		policy.Status.Active = true
		if err := r.Status().Update(ctx, &policy); err != nil {
			logger.Error(err, "Failed to update ShadowPolicy status")
			shadowmetrics.ReconcileTotal.WithLabelValues("error").Inc()
			return ctrl.Result{}, err
		}
		logger.Info("Updated ShadowPolicy status to Active", "name", policy.Name)
	}

	// Record successful reconciliation & active policy metric
	shadowmetrics.ActivePolicies.Set(1)
	shadowmetrics.ReconcileTotal.WithLabelValues("success").Inc()

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ShadowPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&trafficv1alpha1.ShadowPolicy{}).
		Named("shadowpolicy").
		Complete(r)
}
