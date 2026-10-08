package xds

import (
	"fmt"
	"os"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	types "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	trafficv1alpha1 "github.com/neha874-ctrl/shadowcast/api/v1alpha1"
)

// BuildSnapshot creates the xDS snapshot for active shadowing
func BuildSnapshot(version string, policy *trafficv1alpha1.ShadowPolicy) (cachev3.ResourceSnapshot, error) {
	upstreamMode := os.Getenv("UPSTREAM_MODE")

	var discoveryType cluster.Cluster_DiscoveryType
	var sourceHost, targetHost string
	sourcePort, targetPort := uint32(8080), uint32(8080)

	if upstreamMode == "host" {
		hostGatewayIP := os.Getenv("HOST_GATEWAY_IP")
		discoveryType = cluster.Cluster_STATIC
		sourceHost = hostGatewayIP
		targetHost = hostGatewayIP
		sourcePort = 8081
		targetPort = 8082
	} else {
		discoveryType = cluster.Cluster_LOGICAL_DNS
		sourceHost = fmt.Sprintf("%s.%s.svc.cluster.local", policy.Spec.SourceService, policy.Namespace)
		targetHost = fmt.Sprintf("%s.%s.svc.cluster.local", policy.Spec.TargetService, policy.Namespace)
	}

	primaryCluster := makeCluster(policy.Spec.SourceService, sourceHost, sourcePort, discoveryType)
	shadowCluster := makeCluster(policy.Spec.TargetService, targetHost, targetPort, discoveryType)

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
	upstreamMode := os.Getenv("UPSTREAM_MODE")

	discoveryType := cluster.Cluster_LOGICAL_DNS
	sourceHost := "order-service.default.svc.cluster.local"
	var port uint32 = 8080

	if upstreamMode == "host" {
		hostGatewayIP := os.Getenv("HOST_GATEWAY_IP")
		discoveryType = cluster.Cluster_STATIC
		sourceHost = hostGatewayIP
		port = 8081
	}

	primaryCluster := makeCluster("order-service", sourceHost, port, discoveryType)

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
