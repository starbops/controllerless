package kube

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// Clients bundles all Kubernetes client primitives needed by the agent.
type Clients struct {
	Config     *rest.Config
	Dynamic    dynamic.Interface
	Typed      kubernetes.Interface
	RESTMapper meta.RESTMapper
}

// NewClients builds a Clients bundle from a kubeconfig file path.
// If kubeconfigPath is empty, in-cluster config is attempted.
func NewClients(kubeconfigPath string) (*Clients, error) {
	cfg, err := buildConfig(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}

	typedClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("typed client: %w", err)
	}

	// Deferred discovery REST mapper: caches API groups, refreshes on cache miss.
	dc := memory.NewMemCacheClient(typedClient.Discovery())
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(dc)

	return &Clients{
		Config:     cfg,
		Dynamic:    dynClient,
		Typed:      typedClient,
		RESTMapper: mapper,
	}, nil
}

func buildConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("load kubeconfig %q: %w", kubeconfigPath, err)
		}
		return cfg, nil
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	return cfg, nil
}
