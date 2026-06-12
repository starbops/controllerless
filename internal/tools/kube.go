package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"k8s.io/apimachinery/pkg/api/meta"
)

// RegisterKubeTools registers get, list, patch, create, and delete tools
// backed by the provided dynamic client and REST mapper.
func RegisterKubeTools(reg *Registry, dynClient dynamic.Interface, mapper meta.RESTMapper) {
	reg.Register(ToolDef{
		Name:        "get",
		Description: "Fetch a single Kubernetes object by GVK, namespace, and name.",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["gvk","namespace","name"],
			"properties":{
				"gvk":{"type":"string","description":"group/version/Kind, e.g. '/v1/Pod' or 'apps/v1/Deployment'"},
				"namespace":{"type":"string"},
				"name":{"type":"string"}
			}
		}`),
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var p struct {
			GVK       string `json:"gvk"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		gvr, err := resolveGVR(mapper, p.GVK)
		if err != nil {
			return nil, err
		}
		return dynClient.Resource(gvr).Namespace(p.Namespace).Get(ctx, p.Name, metav1.GetOptions{})
	})

	reg.Register(ToolDef{
		Name:        "list",
		Description: "List Kubernetes objects by GVK and namespace with optional selectors.",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["gvk","namespace"],
			"properties":{
				"gvk":{"type":"string"},
				"namespace":{"type":"string"},
				"labelSelector":{"type":"string"},
				"fieldSelector":{"type":"string"}
			}
		}`),
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var p struct {
			GVK           string `json:"gvk"`
			Namespace     string `json:"namespace"`
			LabelSelector string `json:"labelSelector"`
			FieldSelector string `json:"fieldSelector"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		gvr, err := resolveGVR(mapper, p.GVK)
		if err != nil {
			return nil, err
		}
		opts := metav1.ListOptions{
			LabelSelector: p.LabelSelector,
			FieldSelector: p.FieldSelector,
		}
		return dynClient.Resource(gvr).Namespace(p.Namespace).List(ctx, opts)
	})

	reg.Register(ToolDef{
		Name:        "patch",
		Description: "Apply a patch to a Kubernetes object. Defaults to server-side apply.",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["gvk","namespace","name","patch"],
			"properties":{
				"gvk":{"type":"string"},
				"namespace":{"type":"string"},
				"name":{"type":"string"},
				"patch":{"type":"object"},
				"patchType":{"type":"string","enum":["SSA","MergePatch","JSONPatch"]},
				"fieldManager":{"type":"string"},
				"subresource":{"type":"string"}
			}
		}`),
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var p struct {
			GVK          string         `json:"gvk"`
			Namespace    string         `json:"namespace"`
			Name         string         `json:"name"`
			Patch        map[string]any `json:"patch"`
			PatchType    string         `json:"patchType"`
			FieldManager string         `json:"fieldManager"`
			Subresource  string         `json:"subresource"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		gvr, err := resolveGVR(mapper, p.GVK)
		if err != nil {
			return nil, err
		}

		patchBytes, err := json.Marshal(p.Patch)
		if err != nil {
			return nil, fmt.Errorf("marshal patch: %w", err)
		}

		pt := types.ApplyPatchType // default: SSA
		switch p.PatchType {
		case "MergePatch":
			pt = types.MergePatchType
		case "JSONPatch":
			pt = types.JSONPatchType
		}

		fm := p.FieldManager
		if fm == "" {
			fm = "controllerless"
		}

		ri := dynClient.Resource(gvr).Namespace(p.Namespace)
		if pt == types.ApplyPatchType {
			applyOpts := metav1.ApplyOptions{FieldManager: fm, Force: true}
			if p.Subresource != "" {
				return ri.Apply(ctx, p.Name, &unstructured.Unstructured{Object: p.Patch}, applyOpts, p.Subresource)
			}
			return ri.Apply(ctx, p.Name, &unstructured.Unstructured{Object: p.Patch}, applyOpts)
		}
		patchOpts := metav1.PatchOptions{FieldManager: fm}
		return ri.Patch(ctx, p.Name, pt, patchBytes, patchOpts)
	})

	reg.Register(ToolDef{
		Name:        "create",
		Description: "Create a Kubernetes object.",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["object"],
			"properties":{
				"object":{"type":"object","description":"Full Kubernetes manifest including apiVersion, kind, and metadata"}
			}
		}`),
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var p struct {
			Object map[string]any `json:"object"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		u := &unstructured.Unstructured{Object: p.Object}
		gvkStr := u.GetAPIVersion() + "/" + u.GetKind()
		// apiVersion is "group/version" or just "version"; normalise to group/version/Kind
		gvr, err := resolveGVRFromObject(mapper, u)
		if err != nil {
			return nil, fmt.Errorf("resolve GVR for %s: %w", gvkStr, err)
		}
		return dynClient.Resource(gvr).Namespace(u.GetNamespace()).Create(ctx, u, metav1.CreateOptions{})
	})

	reg.Register(ToolDef{
		Name:        "delete",
		Description: "Delete a Kubernetes object by GVK, namespace, and name.",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["gvk","namespace","name"],
			"properties":{
				"gvk":{"type":"string"},
				"namespace":{"type":"string"},
				"name":{"type":"string"},
				"propagationPolicy":{"type":"string","enum":["Background","Foreground","Orphan"]}
			}
		}`),
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		var p struct {
			GVK               string `json:"gvk"`
			Namespace         string `json:"namespace"`
			Name              string `json:"name"`
			PropagationPolicy string `json:"propagationPolicy"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		gvr, err := resolveGVR(mapper, p.GVK)
		if err != nil {
			return nil, err
		}
		policy := metav1.DeletePropagationBackground
		switch p.PropagationPolicy {
		case "Foreground":
			policy = metav1.DeletePropagationForeground
		case "Orphan":
			policy = metav1.DeletePropagationOrphan
		}
		opts := metav1.DeleteOptions{PropagationPolicy: &policy}
		if err := dynClient.Resource(gvr).Namespace(p.Namespace).Delete(ctx, p.Name, opts); err != nil {
			return nil, err
		}
		return "ok", nil
	})
}

// resolveGVR parses a "group/version/Kind" string and maps it to a GVR via the REST mapper.
// Core-group resources use "/v1/Kind" (empty group).
func resolveGVR(mapper meta.RESTMapper, gvkStr string) (schema.GroupVersionResource, error) {
	parts := strings.SplitN(gvkStr, "/", 3)
	var group, version, kind string
	switch len(parts) {
	case 3:
		group, version, kind = parts[0], parts[1], parts[2]
	case 2:
		// "version/Kind" — core group
		version, kind = parts[0], parts[1]
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("invalid GVK string %q; expected group/version/Kind or /version/Kind", gvkStr)
	}
	gk := schema.GroupKind{Group: group, Kind: kind}
	mapping, err := mapper.RESTMapping(gk, version)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("REST mapping for %s: %w", gvkStr, err)
	}
	return mapping.Resource, nil
}

// resolveGVRFromObject extracts the GVK from an unstructured object and maps it to a GVR.
func resolveGVRFromObject(mapper meta.RESTMapper, u *unstructured.Unstructured) (schema.GroupVersionResource, error) {
	gvk := u.GroupVersionKind()
	gk := schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}
	mapping, err := mapper.RESTMapping(gk, gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	return mapping.Resource, nil
}
