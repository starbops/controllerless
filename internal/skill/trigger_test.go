package skill

import (
	"testing"
)

func TestEvalTrigger_LiteralTrue(t *testing.T) {
	result, err := EvalTrigger("true", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result {
		t.Error("expected true, got false")
	}
}

func TestEvalTrigger_LiteralFalse(t *testing.T) {
	result, err := EvalTrigger("false", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result {
		t.Error("expected false, got true")
	}
}

func TestEvalTrigger_EmptyExpr(t *testing.T) {
	result, err := EvalTrigger("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result {
		t.Error("expected true for empty expression, got false")
	}
}

func TestEvalTrigger_ResourceFieldExpr(t *testing.T) {
	obj := map[string]any{
		"spec": map[string]any{
			"suspend": false,
		},
	}
	result, err := EvalTrigger("spec.suspend != true", obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result {
		t.Error("expected true (suspend=false, suspend!=true), got false")
	}
}

func TestEvalTrigger_ResourceFieldExpr_False(t *testing.T) {
	obj := map[string]any{
		"spec": map[string]any{
			"suspend": true,
		},
	}
	result, err := EvalTrigger("spec.suspend != true", obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result {
		t.Error("expected false (suspend=true, suspend!=true is false), got true")
	}
}

func TestEvalTrigger_InvalidCEL(t *testing.T) {
	_, err := EvalTrigger("this is not valid CEL !!!", nil)
	if err == nil {
		t.Fatal("expected error for invalid CEL expression, got nil")
	}
}
