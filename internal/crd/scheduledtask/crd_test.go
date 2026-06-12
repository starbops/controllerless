package scheduledtask_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func crdPath(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "crd.yaml")
}

func loadCRD(t *testing.T) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	data, err := os.ReadFile(crdPath(t))
	if err != nil {
		t.Fatalf("read crd.yaml: %v", err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.NewYAMLOrJSONDecoder(
		bytes.NewReader(data), 4096,
	).Decode(&crd); err != nil {
		t.Fatalf("decode crd.yaml: %v", err)
	}
	return &crd
}

func TestCRD_GroupAndKind(t *testing.T) {
	crd := loadCRD(t)

	if crd.Spec.Group != "agentic.io" {
		t.Errorf("group: got %q, want %q", crd.Spec.Group, "agentic.io")
	}
	if crd.Spec.Names.Kind != "ScheduledTask" {
		t.Errorf("kind: got %q, want %q", crd.Spec.Names.Kind, "ScheduledTask")
	}
	if crd.Spec.Names.Plural != "scheduledtasks" {
		t.Errorf("plural: got %q, want %q", crd.Spec.Names.Plural, "scheduledtasks")
	}
	if crd.Spec.Scope != apiextensionsv1.NamespaceScoped {
		t.Errorf("scope: got %q, want Namespaced", crd.Spec.Scope)
	}
}

func TestCRD_HasStoredVersion(t *testing.T) {
	crd := loadCRD(t)

	var stored []string
	for _, v := range crd.Spec.Versions {
		if v.Storage {
			stored = append(stored, v.Name)
		}
	}
	if len(stored) != 1 {
		t.Fatalf("expected exactly 1 storage version, got %v", stored)
	}
	if stored[0] != "v1alpha1" {
		t.Errorf("storage version: got %q, want %q", stored[0], "v1alpha1")
	}
}

func TestCRD_StatusSubresource(t *testing.T) {
	crd := loadCRD(t)

	for _, v := range crd.Spec.Versions {
		if v.Storage {
			if v.Subresources == nil || v.Subresources.Status == nil {
				t.Errorf("version %s missing status subresource", v.Name)
			}
			return
		}
	}
	t.Fatal("no storage version found")
}

func TestCRD_SpecFields(t *testing.T) {
	crd := loadCRD(t)

	var schema *apiextensionsv1.JSONSchemaProps
	for _, v := range crd.Spec.Versions {
		if v.Storage && v.Schema != nil {
			schema = v.Schema.OpenAPIV3Schema
			break
		}
	}
	if schema == nil {
		t.Fatal("no OpenAPI schema found in storage version")
	}

	specProps := schema.Properties["spec"].Properties
	for _, field := range []string{"schedule", "podSpec", "historyLimit", "suspend"} {
		if _, ok := specProps[field]; !ok {
			t.Errorf("spec.%s missing from schema", field)
		}
	}

	statusProps := schema.Properties["status"].Properties
	for _, field := range []string{"nextFireTime", "fires", "conditions"} {
		if _, ok := statusProps[field]; !ok {
			t.Errorf("status.%s missing from schema", field)
		}
	}
}

func TestCRD_AdditionalPrinterColumns(t *testing.T) {
	crd := loadCRD(t)

	for _, v := range crd.Spec.Versions {
		if v.Storage {
			if len(v.AdditionalPrinterColumns) == 0 {
				t.Error("expected at least one additionalPrinterColumn")
			}
			return
		}
	}
}
