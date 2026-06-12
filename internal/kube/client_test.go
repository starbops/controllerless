package kube_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/starbops/controllerless/internal/kube"
)

const minimalKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
    insecure-skip-tls-verify: true
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
users:
- name: test
  user:
    token: fake-token
`

func writeTempKubeconfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig.yaml")
	if err := os.WriteFile(path, []byte(minimalKubeconfig), 0600); err != nil {
		t.Fatalf("write temp kubeconfig: %v", err)
	}
	return path
}

func TestNewClients_FromKubeconfig(t *testing.T) {
	path := writeTempKubeconfig(t)
	clients, err := kube.NewClients(path)
	if err != nil {
		t.Fatalf("NewClients: %v", err)
	}
	if clients.Dynamic == nil {
		t.Error("Dynamic client is nil")
	}
	if clients.Typed == nil {
		t.Error("Typed client is nil")
	}
	if clients.RESTMapper == nil {
		t.Error("RESTMapper is nil")
	}
	if clients.Config == nil {
		t.Error("rest.Config is nil")
	}
}

func TestNewClients_FailsWithMissingKubeconfig(t *testing.T) {
	_, err := kube.NewClients("/nonexistent/path/kubeconfig.yaml")
	if err == nil {
		t.Error("expected error for missing kubeconfig, got nil")
	}
}

func TestNewClients_ServerURLPreserved(t *testing.T) {
	path := writeTempKubeconfig(t)
	clients, err := kube.NewClients(path)
	if err != nil {
		t.Fatalf("NewClients: %v", err)
	}
	if clients.Config.Host != "https://localhost:6443" {
		t.Errorf("Config.Host: got %q, want %q", clients.Config.Host, "https://localhost:6443")
	}
}
