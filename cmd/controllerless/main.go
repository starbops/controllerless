package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/starbops/controllerless/internal/config"
	"github.com/starbops/controllerless/internal/dispatch"
	"github.com/starbops/controllerless/internal/kube"
	ollama "github.com/starbops/controllerless/internal/llm/providers/ollama"
	"github.com/starbops/controllerless/internal/skill"
	"github.com/starbops/controllerless/internal/tools"
	"github.com/starbops/controllerless/internal/trace"
)

func main() {
	trace.Init()
	cfg := config.Load()

	clients, err := kube.NewClients(cfg.Kubeconfig)
	if err != nil {
		log.Fatalf("kube clients: %v", err)
	}

	skills, err := skill.Load(cfg.SkillsDir)
	if err != nil {
		log.Fatalf("skill load: %v", err)
	}
	slog.Info("skills loaded", "count", len(skills), "dir", cfg.SkillsDir)

	toolReg := tools.NewRegistry()
	tools.RegisterKubeTools(toolReg, clients.Dynamic, clients.RESTMapper)
	tools.RegisterHelperTools(toolReg)
	tools.RegisterMetaTools(toolReg)

	prov := ollama.New(ollama.Config{
		BaseURL:     cfg.LLMBaseURL,
		Model:       cfg.LLMModel,
		Temperature: float32(cfg.LLMTemperature),
		MaxTokens:   cfg.LLMMaxTokens,
		NumCtx:      cfg.LLMNumCtx,
		Timeout:     cfg.LLMTimeout,
	})

	wqReg := kube.NewRegistry()
	factory := kube.NewFactory(clients.Dynamic, 0)

	disp := dispatch.New(dispatch.Deps{
		Kube:       clients,
		WQRegistry: wqReg,
		Skills:     skills,
		Tools:      toolReg,
		Provider:   prov,
		TracesDir:  cfg.TracesDir,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	closer, err := skill.Watch(cfg.SkillsDir, func(updated []skill.Skill) {
		slog.Info("skills hot-reloaded", "count", len(updated))
		disp.UpdateSkills(updated)
	})
	if err != nil {
		log.Fatalf("skill watch: %v", err)
	}
	defer closer.Close()

	slog.Info("controllerless starting",
		"provider", cfg.LLMProvider,
		"model", cfg.LLMModel,
		"skills_dir", cfg.SkillsDir,
	)

	if err := disp.Run(ctx, factory); err != nil {
		slog.Error("dispatcher exited with error", "err", err)
		os.Exit(1)
	}
}
