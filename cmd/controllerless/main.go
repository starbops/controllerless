package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/starbops/controllerless/internal/config"
)

func main() {
	root := &cobra.Command{
		Use:   "controllerless",
		Short: "LLM-powered Kubernetes agent that executes skill documents",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			fmt.Fprintf(os.Stderr, "controllerless starting (provider=%s model=%s skills=%s)\n",
				cfg.LLMProvider, cfg.LLMModel, cfg.SkillsDir)
			// TODO(T3–T8): wire kube, skill, tools, llm, dispatch, trace
			return fmt.Errorf("not yet implemented")
		},
	}

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
