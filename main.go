package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cpra/muon/agent"
	"github.com/cpra/muon/config"
	"github.com/cpra/muon/llm"
	"github.com/cpra/muon/tool"
	"github.com/cpra/muon/tui"
)

func main() {
	configPath := flag.String("config", "config.yml", "path to config file")
	providersPath := flag.String("providers", "providers.yml", "path to providers file")
	flag.Parse()

	cfg, err := config.Load(*configPath, *providersPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	client := llm.NewClient(llm.Config{
		BaseURL:       cfg.BaseURL,
		APIKey:        cfg.APIKey,
		Model:         cfg.Model,
		MaxTokens:     cfg.MaxTokens,
		ContextLength: cfg.MaxContextTokens,
	})

	args := flag.Args()
	ctx := context.Background()

	if len(args) > 0 && args[0] == "list-models" {
		models, err := client.ListModels(ctx)
		if err != nil {
			log.Fatalf("list models: %v", err)
		}
		for _, m := range models {
			fmt.Println(m.ID)
		}
		return
	}

	if len(args) > 0 {
		prompt := strings.Join(args, " ")

		registry := tool.NewRegistry()

		a := agent.New(client, registry, cfg.MaxTurns, cfg.SystemPrompt)
		reply, err := a.Run(ctx, prompt)
		if err != nil {
			log.Fatalf("agent: %v", err)
		}
		fmt.Println(reply)
		return
	}

	registry := tool.NewRegistry()
	workingDir, _ := os.Getwd()
	app := tui.New(client, cfg, workingDir)
	a := agent.New(client, registry, cfg.MaxTurns, cfg.SystemPrompt,
		agent.WithHook(app.Hook),
	)
	app.SetAgent(a)

	if err := app.Run(); err != nil {
		log.Fatalf("TUI error: %v", err)
	}
}
