package dispatch

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/starbops/controllerless/internal/kube"
	"github.com/starbops/controllerless/internal/skill"
)

// Run starts the event-loop: one informer + one worker goroutine per unique GVR
// derived from the currently-loaded skills. It blocks until ctx is cancelled.
//
// factory must be non-nil when any skill trigger is present; pass nil only if
// the skills slice is empty (useful in unit tests).
func (d *Dispatcher) Run(ctx context.Context, factory *kube.Factory) error {
	gvrMap := d.collectGVRs()

	if len(gvrMap) == 0 {
		slog.Info("run: no GVRs to watch, waiting for shutdown")
		<-ctx.Done()
		d.deps.WQRegistry.ShutDownAll()
		return nil
	}

	stopCh := ctx.Done()

	for gvr, gvkStr := range gvrMap {
		gvr, gvkStr := gvr, gvkStr
		factory.InformerFor(gvr, cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				key := extractKey(obj)
				slog.Debug("run: enqueue Add", "gvk", gvkStr, "key", key)
				d.deps.WQRegistry.Enqueue(gvr, key)
			},
			UpdateFunc: func(_, obj any) {
				key := extractKey(obj)
				slog.Debug("run: enqueue Update", "gvk", gvkStr, "key", key)
				d.deps.WQRegistry.Enqueue(gvr, key)
			},
		})
	}

	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	var wg sync.WaitGroup
	for gvr, gvkStr := range gvrMap {
		gvr, gvkStr := gvr, gvkStr
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.runWorker(ctx, gvr, gvkStr)
		}()
	}

	<-ctx.Done()
	d.deps.WQRegistry.ShutDownAll()
	wg.Wait()
	return nil
}

// runWorker dequeues keys for gvr and calls Dispatch for each one.
func (d *Dispatcher) runWorker(ctx context.Context, gvr schema.GroupVersionResource, gvkStr string) {
	q := d.deps.WQRegistry.QueueFor(gvr)
	for {
		item, quit := q.Get()
		if quit {
			return
		}
		key := item.(string)
		if err := d.processItem(ctx, gvr, gvkStr, key); err != nil {
			slog.Error("run: processItem failed", "gvk", gvkStr, "key", key, "err", err)
		}
		q.Done(item)
	}
}

// processItem fetches the object for key and calls Dispatch.
func (d *Dispatcher) processItem(ctx context.Context, gvr schema.GroupVersionResource, gvkStr, key string) error {
	ns, name := splitResourceKey(key)
	obj, err := d.deps.Kube.Dynamic.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get %s/%s: %w", ns, name, err)
	}
	return d.Dispatch(ctx, gvkStr, "Add", key, obj.Object)
}

// UpdateSkills atomically replaces the skills slice used by all subsequent
// Dispatch and matchSkills calls. Safe for concurrent use.
func (d *Dispatcher) UpdateSkills(updated []skill.Skill) {
	d.mu.Lock()
	d.deps.Skills = updated
	d.mu.Unlock()
}

// collectGVRs builds the set of unique GVRs from the current skills slice.
func (d *Dispatcher) collectGVRs() map[schema.GroupVersionResource]string {
	d.mu.Lock()
	skills := d.deps.Skills
	d.mu.Unlock()

	gvrMap := make(map[schema.GroupVersionResource]string)
	for _, s := range skills {
		for _, trig := range s.Frontmatter.Triggers {
			gvr, err := parseGVR(d.deps.Kube.RESTMapper, trig.GVK)
			if err != nil {
				slog.Warn("run: cannot resolve GVK, skipping trigger",
					"skill", s.Frontmatter.Name, "gvk", trig.GVK, "err", err)
				continue
			}
			gvrMap[gvr] = trig.GVK
		}
	}
	return gvrMap
}

// parseGVR parses "group/version/Kind" or "/version/Kind" (core group) and
// resolves it to a GroupVersionResource via the REST mapper.
func parseGVR(mapper meta.RESTMapper, gvkStr string) (schema.GroupVersionResource, error) {
	parts := strings.SplitN(gvkStr, "/", 3)
	var group, version, kind string
	switch len(parts) {
	case 3:
		group, version, kind = parts[0], parts[1], parts[2]
	case 2:
		version, kind = parts[0], parts[1]
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("invalid GVK %q: expected group/version/Kind or /version/Kind", gvkStr)
	}
	gk := schema.GroupKind{Group: group, Kind: kind}
	mapping, err := mapper.RESTMapping(gk, version)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("REST mapping for %q: %w", gvkStr, err)
	}
	return mapping.Resource, nil
}

// extractKey returns a "namespace/name" key from an informer object.
func extractKey(obj any) string {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		key, _ := cache.MetaNamespaceKeyFunc(obj)
		return key
	}
	ns := u.GetNamespace()
	name := u.GetName()
	if ns == "" {
		return name
	}
	return ns + "/" + name
}
