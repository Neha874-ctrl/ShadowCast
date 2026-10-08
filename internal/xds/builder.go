package xds

import (
	"fmt"

	// Standard Protobuf
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	// Envoy v3 Configs
	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"

	// Control Plane Cache & Types
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"

	trafficv1alpha1 "github.com/neha874-ctrl/shadowcast/api/v1alpha1"
)

func makeCluster(name string, host string, port uint32, discoveryType cluster.Cluster_DiscoveryType) *cluster.Cluster {
	return &cluster.Cluster{
		Name:                 name,
		ConnectTimeout:       durationpb.New(2 * 1000000000), // 2s
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: discoveryType},
		LbPolicy:             cluster.Cluster_ROUND_ROBIN,
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpoint.LocalityLbEndpoints{
				{
					LbEndpoints: []*endpoint.LbEndpoint{
						{
							HostIdentifier: &endpoint.LbEndpoint_Endpoint{
								Endpoint: &endpoint.Endpoint{
									Address: &core.Address{
										Address: &core.Address_SocketAddress{
											SocketAddress: &core.SocketAddress{
												Protocol: core.SocketAddress_TCP,
												Address:  host,
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
}

func makeRouteConfig(name string, policy *trafficv1alpha1.ShadowPolicy) *route.RouteConfiguration {
	primaryCluster := policy.Spec.SourceService
	shadowCluster := policy.Spec.TargetService
	mirrorPercent := min(uint32(policy.Spec.MirrorPercentage), 100)

	var mirrorPolicies []*route.RouteAction_RequestMirrorPolicy

	if mirrorPercent > 0 {
		mirrorPolicies = []*route.RouteAction_RequestMirrorPolicy{
			{
				Cluster: shadowCluster,
				RuntimeFraction: &core.RuntimeFractionalPercent{
					DefaultValue: &type_v3.FractionalPercent{
						Numerator:   mirrorPercent,
						Denominator: type_v3.FractionalPercent_HUNDRED,
					},
				},
			},
		}
	}

	// Build Header Matchers from Policy Spec
	var headerMatchers []*route.HeaderMatcher
	for _, h := range policy.Spec.Headers {
		matchValue := h.Value
		if h.Exact != "" {
			matchValue = h.Exact
		}

		if matchValue != "" {
			headerMatchers = append(headerMatchers, &route.HeaderMatcher{
				Name: h.Name,
				HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
					ExactMatch: matchValue, //nolint:staticcheck
				},
			})
		} else {
			headerMatchers = append(headerMatchers, &route.HeaderMatcher{
				Name: h.Name,
				HeaderMatchSpecifier: &route.HeaderMatcher_PresentMatch{
					PresentMatch: true,
				},
			})
		}
	}

	routes := []*route.Route{
		{
			Match: &route.RouteMatch{
				PathSpecifier: &route.RouteMatch_Prefix{
					Prefix: "/",
				},
				Headers: headerMatchers,
			},
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: primaryCluster,
					},
					RequestMirrorPolicies: mirrorPolicies,
				},
			},
		},
		// Default fallback route: send unmatched traffic directly to primary without mirroring
		{
			Match: &route.RouteMatch{
				PathSpecifier: &route.RouteMatch_Prefix{
					Prefix: "/",
				},
			},
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: primaryCluster,
					},
				},
			},
		},
	}

	return &route.RouteConfiguration{
		Name: name,
		VirtualHosts: []*route.VirtualHost{
			{
				Name:    "shadowcast_vh",
				Domains: []string{"*", "127.0.0.1:*", "localhost:*"},
				Routes:  routes,
			},
		},
	}
}
func makePrimaryOnlyRouteConfig(name string, primaryCluster string) *route.RouteConfiguration {
	return &route.RouteConfiguration{
		Name: name,
		VirtualHosts: []*route.VirtualHost{
			{
				Name:    "primary_vh",
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
									Cluster: primaryCluster,
								},
							},
						},
					},
				},
			},
		},
	}
}

func makeHTTPListener(listenerName string, port uint32, routeConfig *route.RouteConfiguration) (*listener.Listener, error) {
	routerConfig, err := anypb.New(&routerv3.Router{})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal router filter: %w", err)
	}

	manager := &hcm.HttpConnectionManager{
		CodecType:  hcm.HttpConnectionManager_AUTO,
		StatPrefix: "ingress_http",
		RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
			RouteConfig: routeConfig,
		},
		HttpFilters: []*hcm.HttpFilter{
			{
				Name: wellknown.Router,
				ConfigType: &hcm.HttpFilter_TypedConfig{
					TypedConfig: routerConfig,
				},
			},
		},
	}

	pbst, err := anypb.New(manager)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal http connection manager: %w", err)
	}

	return &listener.Listener{
		Name: listenerName,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  "0.0.0.0",
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{
			{
				Filters: []*listener.Filter{
					{
						Name: wellknown.HTTPConnectionManager,
						ConfigType: &listener.Filter_TypedConfig{
							TypedConfig: pbst,
						},
					},
				},
			},
		},
	}, nil
}
