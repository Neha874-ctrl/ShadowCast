package xds

import (
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

	trafficv1alpha1 "github.com/neha874-ctrl/shadowcast/api/v1alpha1"
)

// BuildSnapshot constructs an Envoy xDS snapshot for a given ShadowPolicy.
func BuildSnapshot(version string, policy *trafficv1alpha1.ShadowPolicy) (cachev3.ResourceSnapshot, error) {
	primaryCluster := makeCluster(policy.Spec.SourceService, policy.Spec.SourceService, 80)
	shadowCluster := makeCluster(policy.Spec.TargetService, policy.Spec.TargetService, 80)

	routeConfig := makeRouteConfig("shadowcast_routes", policy)

	resources := map[resource.Type][]types.Resource{
		resource.ClusterType: {primaryCluster, shadowCluster},
		resource.RouteType:   {routeConfig},
	}

	return cachev3.NewSnapshot(version, resources)
}

func makeCluster(clusterName, host string, port uint32) *cluster.Cluster {
	return &cluster.Cluster{
		Name:                 clusterName,
		ConnectTimeout:       durationpb.New(2 * time.Second),
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_LOGICAL_DNS},
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
