package xds

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/anypb"

	trafficv1alpha1 "github.com/neha874-ctrl/shadowcast/api/v1alpha1"
)

// BuildSnapshot constructs an Envoy xDS snapshot for a given ShadowPolicy.
func BuildSnapshot(version string, policy *trafficv1alpha1.ShadowPolicy) (cachev3.ResourceSnapshot, error) {
	primaryHost, primaryPort, primaryDiscType := resolveClusterEndpoint(policy.Spec.SourceService, false, policy.Namespace)
	shadowHost, shadowPort, shadowDiscType := resolveClusterEndpoint(policy.Spec.TargetService, true, policy.Namespace)

	primaryCluster := makeCluster(policy.Spec.SourceService, primaryHost, primaryPort, primaryDiscType)
	shadowCluster := makeCluster(policy.Spec.TargetService, shadowHost, shadowPort, shadowDiscType)

	routeConfig := makeRouteConfig("shadowcast_routes", policy)

	resources := map[resource.Type][]types.Resource{
		resource.ClusterType: {primaryCluster, shadowCluster},
		resource.RouteType:   {routeConfig},
	}

	return cachev3.NewSnapshot(version, resources)
}

// resolveClusterEndpoint resolves the target host, port, and discovery type based on deployment environment.
func resolveClusterEndpoint(serviceName string, isTargetService bool, namespace string) (string, uint32, cluster.Cluster_DiscoveryType) {
	// Check if local/host development mode is enabled
	isLocalDev := os.Getenv("UPSTREAM_MODE") == "host" ||
		os.Getenv("LOCAL_DEV") == "true" ||
		os.Getenv("SHADOWCAST_ENV") == "local"

	host := serviceName
	var explicitPort *uint32

	// Check if port is specified in service string (e.g., "order-service:8080")
	if h, p, err := net.SplitHostPort(serviceName); err == nil {
		host = h
		if portVal, err := strconv.ParseUint(p, 10, 32); err == nil {
			pv := uint32(portVal)
			explicitPort = &pv
		}
	}

	if isLocalDev {
		// Host Machine Mode: route to Docker bridge host gateway IP (STATIC discovery)
		gatewayIP := os.Getenv("HOST_GATEWAY_IP")
		if gatewayIP == "" {
			gatewayIP = "172.19.0.1"
		}

		port := uint32(8081)
		if isTargetService {
			port = 8082
		}

		if explicitPort != nil {
			port = *explicitPort
		} else if isTargetService {
			if envPort := os.Getenv("SHADOW_SERVICE_PORT"); envPort != "" {
				if p, err := strconv.ParseUint(envPort, 10, 32); err == nil {
					port = uint32(p)
				}
			}
		} else {
			if envPort := os.Getenv("ORDER_SERVICE_PORT"); envPort != "" {
				if p, err := strconv.ParseUint(envPort, 10, 32); err == nil {
					port = uint32(p)
				}
			}
		}

		return gatewayIP, port, cluster.Cluster_STATIC
	}

	// Kubernetes Pod Mode (Default)
	port := uint32(8080)
	if explicitPort != nil {
		port = *explicitPort
	} else if envPort := os.Getenv("UPSTREAM_PORT"); envPort != "" {
		if p, err := strconv.ParseUint(envPort, 10, 32); err == nil {
			port = uint32(p)
		}
	}

	// If the host is already a static IP address
	if net.ParseIP(host) != nil {
		return host, port, cluster.Cluster_STATIC
	}

	// If host is already an FQDN (e.g. contains dots like order-service.default.svc.cluster.local)
	if strings.Contains(host, ".") {
		return host, port, cluster.Cluster_LOGICAL_DNS
	}

	// Unqualified service name: expand to Kubernetes FQDN
	ns := namespace
	if ns == "" {
		ns = "default"
	}
	fqdn := fmt.Sprintf("%s.%s.svc.cluster.local", host, ns)
	return fqdn, port, cluster.Cluster_LOGICAL_DNS
}

func makeCluster(clusterName, host string, port uint32, discoveryType cluster.Cluster_DiscoveryType) *cluster.Cluster {
	c := &cluster.Cluster{
		Name:                 clusterName,
		ConnectTimeout:       durationpb.New(2 * time.Second),
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: discoveryType},
		LbPolicy:             cluster.Cluster_ROUND_ROBIN,
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: clusterName,
			Endpoints: []*endpoint.LocalityLbEndpoints{
				{
					LbEndpoints: []*endpoint.LbEndpoint{
						{
							HostIdentifier: &endpoint.LbEndpoint_Endpoint{
								Endpoint: &endpoint.Endpoint{
									Address: &core.Address{
										Address: &core.Address_SocketAddress{
											SocketAddress: &core.SocketAddress{
												Address: host,
												PortSpecifier: &core.SocketAddress_PortValue{
													PortValue: port,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if discoveryType == cluster.Cluster_LOGICAL_DNS || discoveryType == cluster.Cluster_STRICT_DNS {
		c.DnsLookupFamily = cluster.Cluster_V4_ONLY
	}

	return c
}

func makeRouteConfig(routeName string, policy *trafficv1alpha1.ShadowPolicy) *route.RouteConfiguration {
	mirrorPercentage := float64(policy.Spec.MirrorPercentage)

	return &route.RouteConfiguration{
		Name: routeName,
		VirtualHosts: []*route.VirtualHost{
			{
				Name:    "shadowcast_vhost",
				Domains: []string{"*"},
				Routes: []*route.Route{
					{
						Match: &route.RouteMatch{
							PathSpecifier: &route.RouteMatch_Prefix{Prefix: "/"},
						},
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								ClusterSpecifier: &route.RouteAction_Cluster{
									Cluster: policy.Spec.SourceService,
								},
								RequestMirrorPolicies: []*route.RouteAction_RequestMirrorPolicy{
									{
										Cluster: policy.Spec.TargetService,
										RuntimeFraction: &core.RuntimeFractionalPercent{
											DefaultValue: &typev3.FractionalPercent{
												Numerator:   uint32(mirrorPercentage * 10000),
												Denominator: typev3.FractionalPercent_MILLION,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func makeHTTPListener(listenerName string, port uint32) (*listener.Listener, error) {
	// 1. Define the HTTP Routing with Shadowing
	routeConfig := &route.RouteConfiguration{
		Name: "local_route",
		VirtualHosts: []*route.VirtualHost{
			{
				Name:    "backend_service",
				Domains: []string{"*"},
				Routes: []*route.Route{
					{
						Match: &route.RouteMatch{
							PathSpecifier: &route.RouteMatch_Prefix{
								Prefix: "/",
							},
						},
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								ClusterSpecifier: &route.RouteAction_Cluster{
									Cluster: "order-service",
								},
								// Shadow/mirror traffic to shadow-service
								RequestMirrorPolicies: []*route.RouteAction_RequestMirrorPolicy{
									{
										Cluster: "shadow-service",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// 2. Wrap in HTTP Connection Manager Filter
	manager := &hcm.HttpConnectionManager{
		CodecType:  hcm.HttpConnectionManager_AUTO,
		StatPrefix: "ingress_http",
		RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
			RouteConfig: routeConfig,
		},
		HttpFilters: []*hcm.HttpFilter{
			{
				Name: wellknown.Router,
			},
		},
	}

	pbst, err := anypb.New(manager)
	if err != nil {
		return nil, err
	}

	// 3. Create Listener on 0.0.0.0:10000
	return &listener.Listener{
		Name: listenerName,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  "0.0.0.0",
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: port, // 10000
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: pbst,
				},
			}},
		}},
	}, nil
}