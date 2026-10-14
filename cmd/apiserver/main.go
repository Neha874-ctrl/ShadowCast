package main

import (
	"os"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/klog/v2"

	trafficv1alpha1 "github.com/neha874-ctrl/shadowcast/api/v1alpha1"
)

var (
	scheme = runtime.NewScheme()
	codecs = serializer.NewCodecFactory(scheme)
)

func init() {
	utilruntime.Must(trafficv1alpha1.AddToScheme(scheme))
}

func main() {
	klog.InitFlags(nil)

	recommendedOptions := genericoptions.NewRecommendedOptions(
		"/registry/shadowcast.io",
		codecs.LegacyCodec(trafficv1alpha1.GroupVersion),
	)

	cmdFlags := pflag.NewFlagSet("shadowcast-api-server", pflag.ExitOnError)
	recommendedOptions.AddFlags(cmdFlags)
	if err := cmdFlags.Parse(os.Args[1:]); err != nil {
		klog.Fatalf("Error parsing flags: %v", err)
	}

	serverConfig := genericapiserver.NewRecommendedConfig(codecs)

	// Disable etcd requirement for in-memory extension APIService
	recommendedOptions.Etcd = nil

	if err := recommendedOptions.ApplyTo(serverConfig); err != nil {
		klog.Fatalf("Failed to apply options: %v", err)
	}

	completedConfig := serverConfig.Complete()
	server, err := completedConfig.New("shadowcast-api-server", genericapiserver.NewEmptyDelegate())
	if err != nil {
		klog.Fatalf("Failed to create extension API server: %v", err)
	}

	stopCh := genericapiserver.SetupSignalHandler()
	if err := server.PrepareRun().Run(stopCh); err != nil {
		klog.Fatalf("Error running extension API server: %v", err)
	}
}
