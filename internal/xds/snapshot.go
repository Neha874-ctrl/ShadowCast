package xds

import (
	"fmt"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	types "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	trafficv1alpha1 "github.com/neha874-ctrl/shadowcast/api/v1alpha1"
)

// BuildSnapshot creates the xDS snapshot for active shadowing
func BuildSnapshot(version string, policy *trafficv1alpha1.ShadowPolicy) (cachev3.ResourceSnapshot, error) {
	sourceHost := fmt.Sprintf("%s.%s.svc.cluster.local", policy.Spec.SourceService, policy.Namespace)
	targetHost := fmt.Sprintf("%s.%s.svc.cluster.local", policy.Spec.TargetService, policy.Namespace)

	primaryCluster := makeCluster(policy.Spec.SourceService, sourceHost, 8080, cluster.Cluster_LOGICAL_DNS)
	shadowCluster := makeCluster(policy.Spec.TargetService, targetHost, 8080, cluster.Cluster_LOGICAL_DNS)

	routeConfig := makeRouteConfig("local_route", policy)

	listener, err := makeHTTPListener("ingress_listener", 10000, routeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}

	snapshot, err := cachev3.NewSnapshot(
		version,
		map[resource.Type][]types.Resource{
			resource.EndpointType: {},
			resource.ClusterType:  {primaryCluster, shadowCluster},
			resource.RouteType:    {routeConfig},
			resource.ListenerType: {listener},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	return snapshot, nil
}

// BuildFallbackSnapshot creates the xDS snapshot when policy is deleted
func BuildFallbackSnapshot(version string) (cachev3.ResourceSnapshot, error) {
	sourceHost := "order-service.default.svc.cluster.local"

	primaryCluster := makeCluster("order-service", sourceHost, 8080, cluster.Cluster_LOGICAL_DNS)

	routeConfig := makePrimaryOnlyRouteConfig("local_route", "order-service")

	listener, err := makeHTTPListener("ingress_listener", 10000, routeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create fallback listener: %w", err)
	}

	snapshot, err := cachev3.NewSnapshot(
		version,
		map[resource.Type][]types.Resource{
			resource.EndpointType: {},
			resource.ClusterType:  {primaryCluster},
			resource.RouteType:    {routeConfig},
			resource.ListenerType: {listener},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create fallback snapshot: %w", err)
	}

	return snapshot, nil
}
