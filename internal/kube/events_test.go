package kube_test

import (
	"testing"

	"github.com/starbops/controllerless/internal/kube"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewEventRecorder_ReturnsNonNil(t *testing.T) {
	client := fake.NewSimpleClientset()
	rec := kube.NewEventRecorder(client, "controllerless")
	if rec == nil {
		t.Error("NewEventRecorder returned nil")
	}
}
