package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	LLMProvider       string
	LLMBaseURL        string
	LLMModel          string
	LLMTemperature    float64
	LLMMaxTokens      int
	LLMTimeout        time.Duration
	LLMNumCtx         int
	MaxToolTurns      int
	ReconcileTimeout  time.Duration
	PerSkillRateLimit time.Duration
	Kubeconfig        string
	SkillsDir         string
	TracesDir         string
	LogLevel          string
}

// Load reads configuration from environment variables, applying defaults for any unset values.
func Load() Config {
	home, _ := os.UserHomeDir()

	cfg := Config{
		LLMProvider:       envOr("LLM_PROVIDER", "ollama"),
		LLMBaseURL:        envOr("LLM_BASE_URL", "http://localhost:11434"),
		LLMModel:          envOr("LLM_MODEL", "gemma4:12b-mxfp8"),
		LLMTemperature:    envFloat("LLM_TEMPERATURE", 0.2),
		LLMMaxTokens:      envInt("LLM_MAX_TOKENS", 4096),
		LLMTimeout:        envDuration("LLM_TIMEOUT", 60*time.Second),
		LLMNumCtx:         envInt("LLM_NUM_CTX", 8192),
		MaxToolTurns:      envInt("AGENT_MAX_TOOL_TURNS", 10),
		ReconcileTimeout:  envDuration("AGENT_RECONCILE_TIMEOUT", 5*time.Minute),
		PerSkillRateLimit: envDuration("AGENT_PER_SKILL_RATE_LIMIT", 5*time.Second),
		Kubeconfig:        os.Getenv("KUBECONFIG"),
		SkillsDir:         envOr("SKILLS_DIR", "./skills"),
		TracesDir:         envOr("TRACES_DIR", home+"/.controllerless/traces"),
		LogLevel:          envOr("LOG_LEVEL", "info"),
	}
	return cfg
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
