package kube

import (
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/workqueue"
)

// Registry holds one rate-limiting workqueue per GroupVersionResource.
// All methods are safe for concurrent use.
type Registry struct {
	mu     sync.Mutex
	queues map[schema.GroupVersionResource]workqueue.RateLimitingInterface
}

// NewRegistry creates an empty workqueue registry.
func NewRegistry() *Registry {
	return &Registry{
		queues: make(map[schema.GroupVersionResource]workqueue.RateLimitingInterface),
	}
}

// QueueFor returns the workqueue for gvr, creating it on first access.
func (r *Registry) QueueFor(gvr schema.GroupVersionResource) workqueue.RateLimitingInterface {
	r.mu.Lock()
	defer r.mu.Unlock()

	if q, ok := r.queues[gvr]; ok {
		return q
	}
	q := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	r.queues[gvr] = q
	return q
}

// Enqueue adds key to the queue for gvr.
// Duplicate keys are coalesced by the underlying workqueue implementation.
func (r *Registry) Enqueue(gvr schema.GroupVersionResource, key string) {
	r.QueueFor(gvr).Add(key)
}

// ShutDownAll signals all queues to stop processing.
func (r *Registry) ShutDownAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, q := range r.queues {
		q.ShutDown()
	}
}
