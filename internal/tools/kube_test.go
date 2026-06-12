package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/starbops/controllerless/internal/tools"
)

// staticRESTMapper implements meta.RESTMapper with a fixed GVK→GVR map.
type staticRESTMapper struct {
	mappings map[schema.GroupVersionKind]schema.GroupVersionResource
}

func (m *staticRESTMapper) KindFor(_ schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}
func (m *staticRESTMapper) KindsFor(_ schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return nil, nil
}
func (m *staticRESTMapper) ResourceFor(_ schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{}, nil
}
func (m *staticRESTMapper) ResourcesFor(_ schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return nil, nil
}
func (m *staticRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	for gvk, gvr := range m.mappings {
		if gvk.Group == gk.Group && gvk.Kind == gk.Kind {
			return &meta.RESTMapping{Resource: gvr, GroupVersionKind: gvk}, nil
		}
	}
	return nil, &meta.NoKindMatchError{GroupKind: gk}
}
func (m *staticRESTMapper) RESTMappings(gk schema.GroupKind, _ ...string) ([]*meta.RESTMapping, error) {
	return nil, nil
}
func (m *staticRESTMapper) ResourceSingularizer(resource string) (string, error) {
	return resource, nil
}

var (
	podGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
	podGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
)

func newKubeTestEnv(t *testing.T, objs ...runtime.Object) (*tools.Registry, *dynamicfake.FakeDynamicClient) {
	t.Helper()

	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(podGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PodList"},
		&unstructured.UnstructuredList{},
	)

	dynClient := dynamicfake.NewSimpleDynamicClient(scheme, objs...)
	mapper := &staticRESTMapper{
		mappings: map[schema.GroupVersionKind]schema.GroupVersionResource{
			podGVK: podGVR,
		},
	}

	reg := tools.NewRegistry()
	tools.RegisterKubeTools(reg, dynClient, mapper)
	return reg, dynClient
}

func mustCall(t *testing.T, reg *tools.Registry, toolName string, args any) any {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	result, err := reg.Call(context.Background(), toolName, raw)
	if err != nil {
		t.Fatalf("tool %q: %v", toolName, err)
	}
	return result
}

func makePod(namespace, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(podGVK)
	u.SetNamespace(namespace)
	u.SetName(name)
	return u
}

func TestKubeTool_Get(t *testing.T) {
	pod := makePod("default", "mypod")
	reg, _ := newKubeTestEnv(t, pod)

	result := mustCall(t, reg, "get", map[string]any{
		"gvk":       "/v1/Pod",
		"namespace": "default",
		"name":      "mypod",
	})

	obj, ok := result.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("expected *unstructured.Unstructured, got %T", result)
	}
	if obj.GetName() != "mypod" {
		t.Fatalf("got name %q, want %q", obj.GetName(), "mypod")
	}
}

func TestKubeTool_Get_NotFound(t *testing.T) {
	reg, _ := newKubeTestEnv(t)

	raw, _ := json.Marshal(map[string]any{
		"gvk":       "/v1/Pod",
		"namespace": "default",
		"name":      "missing",
	})
	_, err := reg.Call(context.Background(), "get", raw)
	if err == nil {
		t.Fatal("expected error for missing object, got nil")
	}
}

func TestKubeTool_List(t *testing.T) {
	pod1 := makePod("default", "pod1")
	pod2 := makePod("default", "pod2")
	reg, _ := newKubeTestEnv(t, pod1, pod2)

	result := mustCall(t, reg, "list", map[string]any{
		"gvk":       "/v1/Pod",
		"namespace": "default",
	})

	list, ok := result.(*unstructured.UnstructuredList)
	if !ok {
		t.Fatalf("expected *unstructured.UnstructuredList, got %T", result)
	}
	if len(list.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list.Items))
	}
}

func TestKubeTool_Create(t *testing.T) {
	reg, dynClient := newKubeTestEnv(t)

	pod := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"namespace": "default",
			"name":      "newpod",
		},
	}
	mustCall(t, reg, "create", map[string]any{"object": pod})

	_, err := dynClient.Resource(podGVR).Namespace("default").Get(
		context.Background(), "newpod", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("created pod not found: %v", err)
	}
}

func TestKubeTool_Delete(t *testing.T) {
	pod := makePod("default", "topod")
	reg, dynClient := newKubeTestEnv(t, pod)

	mustCall(t, reg, "delete", map[string]any{
		"gvk":       "/v1/Pod",
		"namespace": "default",
		"name":      "topod",
	})

	_, err := dynClient.Resource(podGVR).Namespace("default").Get(
		context.Background(), "topod", metav1.GetOptions{},
	)
	if err == nil {
		t.Fatal("expected error after delete, pod should not exist")
	}
}

func TestKubeTool_Patch(t *testing.T) {
	pod := makePod("default", "patchpod")
	reg, dynClient := newKubeTestEnv(t, pod)

	// Use MergePatch: fake dynamic client's Apply falls back to StrategicMergePatch
	// which requires typed objects (fails on Unstructured). MergePatch works fine.
	patchDelta := map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{"patched": "true"},
		},
	}
	mustCall(t, reg, "patch", map[string]any{
		"gvk":          "/v1/Pod",
		"namespace":    "default",
		"name":         "patchpod",
		"patch":        patchDelta,
		"patchType":    "MergePatch",
		"fieldManager": "test-manager",
	})

	obj, err := dynClient.Resource(podGVR).Namespace("default").Get(
		context.Background(), "patchpod", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get after patch: %v", err)
	}
	labels := obj.GetLabels()
	if labels["patched"] != "true" {
		t.Fatalf("expected label patched=true, got %v", labels)
	}
}
