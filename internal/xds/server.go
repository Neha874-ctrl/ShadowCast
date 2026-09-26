package xds

import (
	"context"
	"fmt"
	"net"

	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/grpc"
	ctrl "sigs.k8s.io/controller-runtime"
)

var log = ctrl.Log.WithName("xds-server")

// RunServer starts the xDS gRPC control plane server on the specified port.
func RunServer(ctx context.Context, cache cachev3.SnapshotCache, port int) error {
	srv := serverv3.NewServer(ctx, cache, nil)

	grpcServer := grpc.NewServer()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	// Register Envoy xDS v3 Services
	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(grpcServer, srv)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, srv)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, srv)
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, srv)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, srv)

	log.Info("Starting xDS gRPC server", "port", port)

	go func() {
		<-ctx.Done()
		log.Info("Stopping xDS gRPC server")
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(lis); err != nil && ctx.Err() == nil {
		return fmt.Errorf("xDS gRPC server error: %w", err)
	}

	return nil
}
