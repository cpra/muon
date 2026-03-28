package main

import (
	"bufio"
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

	if len(args) == 1 && args[0] == "list-models" {
		models, err := client.ListModels(context.Background())
		if err != nil {
			log.Fatalf("list models: %v", err)
		}
		fmt.Printf("Models supported by provider %q:\n", cfg.ProviderName)
		for _, m := range models {
			fmt.Printf(" - %s\n", m.ID)
		}
		return
	}

	registry := tool.NewRegistry()
	registry.Register(&tool.BashTool{})
	registry.Register(&tool.ReadTool{})
	registry.Register(&tool.WriteTool{})
	registry.Register(&tool.EditTool{})

	a := agent.New(client, registry, cfg.MaxTurns, cfg.SystemPrompt)

	if len(args) > 0 {
		result, err := a.Run(context.Background(), strings.Join(args, " "))
		if err != nil {
			log.Fatalf("agent error: %v", err)
		}
		fmt.Println(result)
		return
	}

	runInteractive(a)
}

func runInteractive(a *agent.Agent) {
	ctx := context.Background()
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Fprintln(os.Stderr, "muon interactive mode — type /exit to quit")
	fmt.Fprintln(os.Stderr)

	var session *agent.Session

	for {
		fmt.Fprint(os.Stderr, "> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "/exit" {
			break
		}

		var result string
		var err error

		if session == nil {
			session, result, err = a.Start(ctx, line)
		} else {
			result, err = session.Continue(ctx, line)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}

		fmt.Println(result)
		fmt.Fprintln(os.Stderr)
	}

	if session != nil {
		cost := session.Cost()
		fmt.Fprintf(os.Stderr, "session cost: $%.6f (%d prompt + %d completion tokens)\n",
			cost.TotalCost, session.Usage().PromptTokens, session.Usage().CompletionTokens)
	}
}
