package xds

import (
	"testing"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	trafficv1alpha1 "github.com/neha874-ctrl/shadowcast/api/v1alpha1"
)

func TestBuildSnapshot_K8sMode(t *testing.T) {
	t.Setenv("UPSTREAM_MODE", "")
	t.Setenv("LOCAL_DEV", "")
	t.Setenv("SHADOWCAST_ENV", "")

	policy := &trafficv1alpha1.ShadowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: trafficv1alpha1.ShadowPolicySpec{
			SourceService:    "order-service",
			TargetService:    "shadow-service",
			MirrorPercentage: 50,
		},
	}

	snapshot, err := BuildSnapshot("v1", policy)
	if err != nil {
		t.Fatalf("unexpected error building snapshot: %v", err)
	}

	clusters := snapshot.GetResources(resource.ClusterType)
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}

	primary := clusters["order-service"].(*cluster.Cluster)
	if primary.ClusterDiscoveryType.(*cluster.Cluster_Type).Type != cluster.Cluster_LOGICAL_DNS {
		t.Errorf("expected primary cluster discovery type LOGICAL_DNS, got %v", primary.ClusterDiscoveryType)
	}
	if primary.DnsLookupFamily != cluster.Cluster_V4_ONLY {
		t.Errorf("expected DnsLookupFamily V4_ONLY, got %v", primary.DnsLookupFamily)
	}

	addr := primary.LoadAssignment.Endpoints[0].LbEndpoints[0].HostIdentifier.(*endpoint.LbEndpoint_Endpoint).Endpoint.Address.Address.(*core.Address_SocketAddress).SocketAddress
	if addr.Address != "order-service.default.svc.cluster.local" {
		t.Errorf("expected FQDN order-service.default.svc.cluster.local, got %s", addr.Address)
	}
	if addr.GetPortValue() != 8080 {
		t.Errorf("expected port 8080, got %d", addr.GetPortValue())
	}

	shadow := clusters["shadow-service"].(*cluster.Cluster)
	shadowAddr := shadow.LoadAssignment.Endpoints[0].LbEndpoints[0].HostIdentifier.(*endpoint.LbEndpoint_Endpoint).Endpoint.Address.Address.(*core.Address_SocketAddress).SocketAddress
	if shadowAddr.Address != "shadow-service.default.svc.cluster.local" {
		t.Errorf("expected FQDN shadow-service.default.svc.cluster.local, got %s", shadowAddr.Address)
	}
	if shadowAddr.GetPortValue() != 8080 {
		t.Errorf("expected port 8080, got %d", shadowAddr.GetPortValue())
	}
}

func TestBuildSnapshot_HostMode(t *testing.T) {
	t.Setenv("UPSTREAM_MODE", "host")
	t.Setenv("HOST_GATEWAY_IP", "172.19.0.1")

	policy := &trafficv1alpha1.ShadowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: trafficv1alpha1.ShadowPolicySpec{
			SourceService:    "order-service",
			TargetService:    "shadow-service",
			MirrorPercentage: 100,
		},
	}

	snapshot, err := BuildSnapshot("v2", policy)
	if err != nil {
		t.Fatalf("unexpected error building snapshot: %v", err)
	}

	clusters := snapshot.GetResources(resource.ClusterType)
	primary := clusters["order-service"].(*cluster.Cluster)
	if primary.ClusterDiscoveryType.(*cluster.Cluster_Type).Type != cluster.Cluster_STATIC {
		t.Errorf("expected STATIC discovery type in host mode, got %v", primary.ClusterDiscoveryType)
	}

	primaryAddr := primary.LoadAssignment.Endpoints[0].LbEndpoints[0].HostIdentifier.(*endpoint.LbEndpoint_Endpoint).Endpoint.Address.Address.(*core.Address_SocketAddress).SocketAddress
	if primaryAddr.Address != "172.19.0.1" {
		t.Errorf("expected host gateway 172.19.0.1, got %s", primaryAddr.Address)
	}
	if primaryAddr.GetPortValue() != 8081 {
		t.Errorf("expected port 8081 for order-service, got %d", primaryAddr.GetPortValue())
	}

	shadow := clusters["shadow-service"].(*cluster.Cluster)
	shadowAddr := shadow.LoadAssignment.Endpoints[0].LbEndpoints[0].HostIdentifier.(*endpoint.LbEndpoint_Endpoint).Endpoint.Address.Address.(*core.Address_SocketAddress).SocketAddress
	if shadowAddr.Address != "172.19.0.1" {
		t.Errorf("expected host gateway 172.19.0.1, got %s", shadowAddr.Address)
	}
	if shadowAddr.GetPortValue() != 8082 {
		t.Errorf("expected port 8082 for shadow-service, got %d", shadowAddr.GetPortValue())
	}
}
