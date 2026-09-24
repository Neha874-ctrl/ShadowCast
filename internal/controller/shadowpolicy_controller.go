/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	trafficv1alpha1 "github.com/neha874-ctrl/shadowcast/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ShadowPolicyReconciler reconciles a ShadowPolicy object
type ShadowPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=traffic.shadowcast.io,resources=shadowpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=traffic.shadowcast.io,resources=shadowpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=traffic.shadowcast.io,resources=shadowpolicies/finalizers,verbs=update

// Reconcile reads the state of the cluster for a ShadowPolicy object and makes changes as necessary.
func (r *ShadowPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	var policy trafficv1alpha1.ShadowPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ShadowPolicy resource deleted", "name", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch ShadowPolicy")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling ShadowPolicy",
		"source", policy.Spec.SourceService,
		"target", policy.Spec.TargetService,
		"mirrorPercentage", policy.Spec.MirrorPercentage,
	)

	if !policy.Status.Active {
		policy.Status.Active = true
		if err := r.Status().Update(ctx, &policy); err != nil {
			logger.Error(err, "Failed to update ShadowPolicy status")
			return ctrl.Result{}, err
		}
		logger.Info("Updated ShadowPolicy status to Active", "name", policy.Name)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ShadowPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&trafficv1alpha1.ShadowPolicy{}).
		Named("shadowpolicy").
		Complete(r)
}
