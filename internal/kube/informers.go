package kube

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

// Factory wraps a dynamic shared informer factory, providing per-GVR informers.
type Factory struct {
	inner dynamicinformer.DynamicSharedInformerFactory
}

// NewFactory creates an informer factory with the given resync period.
func NewFactory(client dynamic.Interface, resync time.Duration) *Factory {
	return &Factory{
		inner: dynamicinformer.NewDynamicSharedInformerFactory(client, resync),
	}
}

// InformerFor returns a SharedIndexInformer for gvr and registers handler.
// The informer is shared: multiple callers for the same GVR get the same instance.
func (f *Factory) InformerFor(gvr schema.GroupVersionResource, handler cache.ResourceEventHandler) cache.SharedIndexInformer {
	inf := f.inner.ForResource(gvr).Informer()
	if handler != nil {
		inf.AddEventHandler(handler)
	}
	return inf
}

// Start launches all registered informers in background goroutines.
func (f *Factory) Start(stopCh <-chan struct{}) {
	f.inner.Start(stopCh)
}

// WaitForCacheSync blocks until all started informers have synced.
func (f *Factory) WaitForCacheSync(stopCh <-chan struct{}) map[schema.GroupVersionResource]bool {
	return f.inner.WaitForCacheSync(stopCh)
}
