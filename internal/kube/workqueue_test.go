package kube_test

import (
	"testing"

	"github.com/starbops/controllerless/internal/kube"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	gvrPods = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	gvrSTs  = schema.GroupVersionResource{Group: "agentic.io", Version: "v1alpha1", Resource: "scheduledtasks"}
)

func TestRegistry_QueueFor_CreatesQueueOnFirstCall(t *testing.T) {
	r := kube.NewRegistry()
	q := r.QueueFor(gvrPods)
	if q == nil {
		t.Fatal("QueueFor returned nil")
	}
}

func TestRegistry_QueueFor_ReturnsSameQueueForSameGVR(t *testing.T) {
	r := kube.NewRegistry()
	q1 := r.QueueFor(gvrPods)
	q2 := r.QueueFor(gvrPods)
	if q1 != q2 {
		t.Error("expected same queue instance for same GVR")
	}
}

func TestRegistry_QueueFor_CreatesDistinctQueuesPerGVR(t *testing.T) {
	r := kube.NewRegistry()
	q1 := r.QueueFor(gvrPods)
	q2 := r.QueueFor(gvrSTs)
	if q1 == q2 {
		t.Error("expected distinct queues for different GVRs")
	}
}

func TestRegistry_Enqueue_AddsItemToQueue(t *testing.T) {
	r := kube.NewRegistry()
	r.Enqueue(gvrPods, "default/my-pod")

	q := r.QueueFor(gvrPods)
	if q.Len() != 1 {
		t.Errorf("queue length: got %d, want 1", q.Len())
	}

	item, _ := q.Get()
	if item != "default/my-pod" {
		t.Errorf("dequeued item: got %v, want %q", item, "default/my-pod")
	}
	q.Done(item)
}

func TestRegistry_Enqueue_DeduplicatesIdenticalKeys(t *testing.T) {
	r := kube.NewRegistry()
	r.Enqueue(gvrPods, "default/my-pod")
	r.Enqueue(gvrPods, "default/my-pod")

	q := r.QueueFor(gvrPods)
	if q.Len() != 1 {
		t.Errorf("queue length after duplicate enqueue: got %d, want 1", q.Len())
	}
}

func TestRegistry_Enqueue_RoutesToCorrectQueue(t *testing.T) {
	r := kube.NewRegistry()
	r.Enqueue(gvrPods, "default/pod-a")
	r.Enqueue(gvrSTs, "default/task-b")

	if r.QueueFor(gvrPods).Len() != 1 {
		t.Error("pods queue should have 1 item")
	}
	if r.QueueFor(gvrSTs).Len() != 1 {
		t.Error("scheduledtasks queue should have 1 item")
	}
}
