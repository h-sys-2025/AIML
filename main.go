package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	model := flag.String("model", "qwen2.5:3b", "Ollama model to use")
	host := flag.String("host", "http://localhost:11434", "Ollama host URL")
	maxTurns := flag.Int("turns", 20, "Maximum agentic turns per session")
	verbose := flag.Bool("verbose", false, "Show raw AI output and tool calls")
	listTools := flag.Bool("list-tools", false, "List all available tools and exit")
	sysPrompt := flag.String("system", "", "Override system prompt (optional)")
	flag.Parse()

	registry := NewToolRegistry()
	RegisterAllTools(registry)

	if *listTools {
		fmt.Println("=== Available Tools ===")
		for _, t := range registry.List() {
			fmt.Printf("\n[%s]\n  %s\n  Args: %s\n", t.Name, t.Description, formatArgs(t.Args))
		}
		return
	}

	client := NewOllamaClient(*host, *model)
	interp := NewInterpreter(registry, client, *verbose, *maxTurns)

	systemPrompt := *sysPrompt
	if systemPrompt == "" {
		systemPrompt = BuildSystemPrompt(registry)
	}

	fmt.Printf("🤖 AIML Agent — model: %s @ %s\n", *model, *host)
	fmt.Println("Commands: /clear  /help  exit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("You> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		switch {
		case input == "exit" || input == "quit":
			fmt.Println("Bye!")
			return

		case input == "/clear":
			interp.Clear()
			fmt.Print("🧹 Context cleared.\n\n")
			continue

		case input == "/help":
			fmt.Println("Commands:")
			fmt.Println("  /clear   Clear conversation history")
			fmt.Println("  /help    Show this help")
			fmt.Println("  exit     Quit")
			fmt.Println()
			continue

		case strings.HasPrefix(input, "/"):
			fmt.Printf("Unknown command: %s. Try /help\n\n", input)
			continue
		}

		interp.Run(systemPrompt, input)
	}
}

func formatArgs(args []ArgDef) string {
	parts := make([]string, len(args))
	for i, a := range args {
		req := ""
		if a.Required {
			req = "*"
		}
		parts[i] = fmt.Sprintf("%s%s(%s)", a.Name, req, a.Type)
	}
	return strings.Join(parts, ", ")
}