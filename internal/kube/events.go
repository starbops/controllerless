package kube

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
)

// NewEventRecorder creates a Kubernetes event recorder for the given component name.
func NewEventRecorder(client kubernetes.Interface, component string) record.EventRecorder {
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{
		Interface: client.CoreV1().Events(""),
	})
	return broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: component})
}
