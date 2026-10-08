package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ActivePolicies tracks the total number of active ShadowPolicy CRDs
	ActivePolicies = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "shadowcast_active_policies_total",
			Help: "Total number of active ShadowPolicy CRDs managed by ShadowCast",
		},
	)

	// ReconcileTotal tracks total reconciliation attempts by status
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "shadowcast_reconcile_total",
			Help: "Total number of ShadowPolicy reconciliations processed",
		},
		[]string{"status"},
	)

	// XDSSnapshotUpdates tracks successful xDS snapshot pushes to Envoy
	XDSSnapshotUpdates = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "shadowcast_xds_snapshot_updates_total",
			Help: "Total number of successful xDS snapshot updates pushed to Envoy control plane",
		},
	)
)

func init() {
	// Register metrics with controller-runtime's default registry
	metrics.Registry.MustRegister(
		ActivePolicies,
		ReconcileTotal,
		XDSSnapshotUpdates,
	)
}
