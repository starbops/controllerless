package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/starbops/controllerless/internal/config"
)

func TestConfigDefaults(t *testing.T) {
	// Clear all env vars that config reads
	vars := []string{
		"LLM_PROVIDER", "LLM_BASE_URL", "LLM_MODEL",
		"LLM_TEMPERATURE", "LLM_MAX_TOKENS", "LLM_TIMEOUT", "LLM_NUM_CTX",
		"AGENT_MAX_TOOL_TURNS", "AGENT_RECONCILE_TIMEOUT", "AGENT_PER_SKILL_RATE_LIMIT",
		"KUBECONFIG", "SKILLS_DIR", "TRACES_DIR", "LOG_LEVEL",
	}
	for _, v := range vars {
		t.Setenv(v, "")
	}

	cfg := config.Load()

	if cfg.LLMProvider != "ollama" {
		t.Errorf("LLMProvider: got %q, want %q", cfg.LLMProvider, "ollama")
	}
	if cfg.LLMBaseURL != "http://localhost:11434" {
		t.Errorf("LLMBaseURL: got %q, want %q", cfg.LLMBaseURL, "http://localhost:11434")
	}
	if cfg.LLMModel != "gemma4:12b-mxfp8" {
		t.Errorf("LLMModel: got %q, want %q", cfg.LLMModel, "gemma4:12b-mxfp8")
	}
	if cfg.LLMTemperature != 0.2 {
		t.Errorf("LLMTemperature: got %v, want 0.2", cfg.LLMTemperature)
	}
	if cfg.LLMMaxTokens != 4096 {
		t.Errorf("LLMMaxTokens: got %d, want 4096", cfg.LLMMaxTokens)
	}
	if cfg.LLMTimeout != 60*time.Second {
		t.Errorf("LLMTimeout: got %v, want 60s", cfg.LLMTimeout)
	}
	if cfg.LLMNumCtx != 8192 {
		t.Errorf("LLMNumCtx: got %d, want 8192", cfg.LLMNumCtx)
	}
	if cfg.MaxToolTurns != 10 {
		t.Errorf("MaxToolTurns: got %d, want 10", cfg.MaxToolTurns)
	}
	if cfg.ReconcileTimeout != 5*time.Minute {
		t.Errorf("ReconcileTimeout: got %v, want 5m", cfg.ReconcileTimeout)
	}
	if cfg.PerSkillRateLimit != 5*time.Second {
		t.Errorf("PerSkillRateLimit: got %v, want 5s", cfg.PerSkillRateLimit)
	}
	if cfg.SkillsDir != "./skills" {
		t.Errorf("SkillsDir: got %q, want %q", cfg.SkillsDir, "./skills")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "info")
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", "https://api.example.com")
	t.Setenv("LLM_MODEL", "llama3:8b")
	t.Setenv("LLM_TEMPERATURE", "0.7")
	t.Setenv("LLM_MAX_TOKENS", "2048")
	t.Setenv("LLM_TIMEOUT", "30s")
	t.Setenv("LLM_NUM_CTX", "4096")
	t.Setenv("AGENT_MAX_TOOL_TURNS", "5")
	t.Setenv("AGENT_RECONCILE_TIMEOUT", "2m")
	t.Setenv("AGENT_PER_SKILL_RATE_LIMIT", "10s")
	t.Setenv("KUBECONFIG", "/home/user/.kube/config")
	t.Setenv("SKILLS_DIR", "/etc/controllerless/skills")
	t.Setenv("TRACES_DIR", "/tmp/traces")
	t.Setenv("LOG_LEVEL", "debug")

	cfg := config.Load()

	if cfg.LLMProvider != "openai" {
		t.Errorf("LLMProvider: got %q, want %q", cfg.LLMProvider, "openai")
	}
	if cfg.LLMBaseURL != "https://api.example.com" {
		t.Errorf("LLMBaseURL: got %q, want %q", cfg.LLMBaseURL, "https://api.example.com")
	}
	if cfg.LLMModel != "llama3:8b" {
		t.Errorf("LLMModel: got %q, want %q", cfg.LLMModel, "llama3:8b")
	}
	if cfg.LLMTemperature != 0.7 {
		t.Errorf("LLMTemperature: got %v, want 0.7", cfg.LLMTemperature)
	}
	if cfg.LLMMaxTokens != 2048 {
		t.Errorf("LLMMaxTokens: got %d, want 2048", cfg.LLMMaxTokens)
	}
	if cfg.LLMTimeout != 30*time.Second {
		t.Errorf("LLMTimeout: got %v, want 30s", cfg.LLMTimeout)
	}
	if cfg.LLMNumCtx != 4096 {
		t.Errorf("LLMNumCtx: got %d, want 4096", cfg.LLMNumCtx)
	}
	if cfg.MaxToolTurns != 5 {
		t.Errorf("MaxToolTurns: got %d, want 5", cfg.MaxToolTurns)
	}
	if cfg.ReconcileTimeout != 2*time.Minute {
		t.Errorf("ReconcileTimeout: got %v, want 2m", cfg.ReconcileTimeout)
	}
	if cfg.PerSkillRateLimit != 10*time.Second {
		t.Errorf("PerSkillRateLimit: got %v, want 10s", cfg.PerSkillRateLimit)
	}
	if cfg.Kubeconfig != "/home/user/.kube/config" {
		t.Errorf("Kubeconfig: got %q, want %q", cfg.Kubeconfig, "/home/user/.kube/config")
	}
	if cfg.SkillsDir != "/etc/controllerless/skills" {
		t.Errorf("SkillsDir: got %q, want %q", cfg.SkillsDir, "/etc/controllerless/skills")
	}
	if cfg.TracesDir != "/tmp/traces" {
		t.Errorf("TracesDir: got %q, want %q", cfg.TracesDir, "/tmp/traces")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestTracesDir_DefaultExpandsHome(t *testing.T) {
	t.Setenv("TRACES_DIR", "")
	home, _ := os.UserHomeDir()
	cfg := config.Load()
	want := home + "/.controllerless/traces"
	if cfg.TracesDir != want {
		t.Errorf("TracesDir: got %q, want %q", cfg.TracesDir, want)
	}
}
