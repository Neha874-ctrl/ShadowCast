package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	trafficv1alpha1 "github.com/neha874-ctrl/shadowcast/api/v1alpha1"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(trafficv1alpha1.AddToScheme(scheme))
}

type APIServer struct {
	k8sClient     client.Client
	prometheusURL string
	httpClient    *http.Client
}

// CreatePolicyRequest maps incoming UI form payloads to ShadowPolicy CRD
type CreatePolicyRequest struct {
	Name             string                        `json:"name"`
	Namespace        string                        `json:"namespace"`
	SourceService    string                        `json:"sourceService"`
	TargetService    string                        `json:"targetService"`
	MirrorPercentage int32                         `json:"mirrorPercentage"`
	IgnoredFields    []string                      `json:"ignoredFields,omitempty"`
	Headers          []trafficv1alpha1.HeaderMatch `json:"headers,omitempty"`
}

type UpdatePolicyRequest struct {
	MirrorPercentage *int32                        `json:"mirrorPercentage,omitempty"`
	Headers          []trafficv1alpha1.HeaderMatch `json:"headers,omitempty"`
}

func main() {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Fatalf("Failed to load kubeconfig: %v", err)
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	promURL := os.Getenv("PROMETHEUS_URL")
	if promURL == "" {
		promURL = "http://localhost:9090"
	}

	srv := &APIServer{
		k8sClient:     k8sClient,
		prometheusURL: promURL,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", srv.handleHealth)
	mux.HandleFunc("GET /api/v1/services", srv.handleListServices)
	mux.HandleFunc("GET /api/v1/policies", srv.handleListPolicies)
	mux.HandleFunc("POST /api/v1/policies", srv.handleCreatePolicy)
	mux.HandleFunc("PATCH /api/v1/policies/{namespace}/{name}", srv.handleUpdatePolicy)
	mux.HandleFunc("DELETE /api/v1/policies/{namespace}/{name}", srv.handleDeletePolicy)
	mux.HandleFunc("GET /api/v1/metrics/overview", srv.handleOverviewMetrics)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8090"
	}

	log.Printf("ShadowCast API listening on :%s (Prometheus: %s)", port, promURL)
	if err := http.ListenAndServe(":"+port, withCORS(mux)); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "OPERATIONAL"})
}

// GET /api/v1/services?namespace=default
func (s *APIServer) handleListServices(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	var svcList corev1.ServiceList
	opts := []client.ListOption{}
	if ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}

	if err := s.k8sClient.List(r.Context(), &svcList, opts...); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	type ServiceSummary struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		ClusterIP string `json:"clusterIP"`
	}
	result := make([]ServiceSummary, 0, len(svcList.Items))
	for _, svc := range svcList.Items {
		result = append(result, ServiceSummary{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			ClusterIP: svc.Spec.ClusterIP,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// GET /api/v1/policies
func (s *APIServer) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	var policyList trafficv1alpha1.ShadowPolicyList
	if err := s.k8sClient.List(r.Context(), &policyList); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, policyList.Items)
}

// POST /api/v1/policies
func (s *APIServer) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	var req CreatePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
		return
	}
	if req.Name == "" || req.SourceService == "" || req.TargetService == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, sourceService, and targetService are required"})
		return
	}
	if req.MirrorPercentage < 0 || req.MirrorPercentage > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mirrorPercentage must be between 0 and 100"})
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	policy := &trafficv1alpha1.ShadowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: trafficv1alpha1.ShadowPolicySpec{
			SourceService:    req.SourceService,
			TargetService:    req.TargetService,
			MirrorPercentage: req.MirrorPercentage,
			IgnoredFields:    req.IgnoredFields,
			Headers:          req.Headers,
		},
	}

	if err := s.k8sClient.Create(r.Context(), policy); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, policy)
}

// PATCH /api/v1/policies/{namespace}/{name}
func (s *APIServer) handleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")

	var req UpdatePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
		return
	}

	var policy trafficv1alpha1.ShadowPolicy
	if err := s.k8sClient.Get(r.Context(), types.NamespacedName{Namespace: ns, Name: name}, &policy); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	if req.MirrorPercentage != nil {
		if *req.MirrorPercentage < 0 || *req.MirrorPercentage > 100 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mirrorPercentage must be between 0 and 100"})
			return
		}
		policy.Spec.MirrorPercentage = *req.MirrorPercentage
	}
	if req.Headers != nil {
		policy.Spec.Headers = req.Headers
	}

	if err := s.k8sClient.Update(r.Context(), &policy); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

// DELETE /api/v1/policies/{namespace}/{name}
func (s *APIServer) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")

	policy := &trafficv1alpha1.ShadowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	if err := s.k8sClient.Delete(r.Context(), policy); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": name})
}

// GET /api/v1/metrics/overview?window=15m
func (s *APIServer) handleOverviewMetrics(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "15m"
	}

	primaryRPS := s.queryPrometheusScalar(r.Context(), fmt.Sprintf(`sum(rate(envoy_cluster_upstream_rq_total{envoy_cluster_name!~"shadow.*"}[%s]))`, window))
	shadowRPS := s.queryPrometheusScalar(r.Context(), fmt.Sprintf(`sum(rate(envoy_cluster_upstream_rq_total{envoy_cluster_name=~"shadow.*"}[%s]))`, window))
	shadow2xx := s.queryPrometheusScalar(r.Context(), fmt.Sprintf(`sum(rate(envoy_cluster_upstream_rq_xx{envoy_cluster_name=~"shadow.*",envoy_response_code_class="2"}[%s]))`, window))
	shadow5xx := s.queryPrometheusScalar(r.Context(), fmt.Sprintf(`sum(rate(envoy_cluster_upstream_rq_xx{envoy_cluster_name=~"shadow.*",envoy_response_code_class="5"}[%s]))`, window))
	p95Latency := s.queryPrometheusScalar(r.Context(), fmt.Sprintf(`histogram_quantile(0.95, sum(rate(envoy_cluster_upstream_rq_time_bucket{envoy_cluster_name=~"shadow.*"}[%s])) by (le))`, window))

	passRate := 100.0
	if shadowRPS > 0 {
		passRate = (shadow2xx / shadowRPS) * 100.0
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"window":         window,
		"primary_rps":    primaryRPS,
		"shadow_rps":     shadowRPS,
		"shadow_2xx_rps": shadow2xx,
		"shadow_5xx_rps": shadow5xx,
		"pass_rate_pct":  passRate,
		"shadow_p95_ms":  p95Latency,
	})
}

func (s *APIServer) queryPrometheusScalar(ctx context.Context, promQL string) float64 {
	endpoint := fmt.Sprintf("%s/api/v1/query?query=%s", s.prometheusURL, url.QueryEscape(promQL))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Data.Result) == 0 || len(parsed.Data.Result[0].Value) < 2 {
		return 0
	}
	var val float64
	if strVal, ok := parsed.Data.Result[0].Value[1].(string); ok {
		_, _ = fmt.Sscanf(strVal, "%f", &val)
	}
	return val
}