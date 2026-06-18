package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func main() {
	model := flag.String("model", "qwen2.5:3b", "Ollama model to use")
	host := flag.String("host", "http://localhost:11434", "Ollama host URL")
	maxTurns := flag.Int("turns", 20, "Maximum agentic turns per session")
	verbose := flag.Bool("verbose", false, "Show raw AI output and tool calls")
	listTools := flag.Bool("list-tools", false, "List all available tools and exit")
	sysPrompt := flag.String("system", "", "Override system prompt (optional)")
	speak := flag.Bool("speak", false, "Speak AI answers aloud via TTS (espeak/say)")
	listen := flag.Bool("listen", false, "Capture microphone input for questions instead of typing (requires sox+whisper)")
	nothink := flag.Bool("nothink", true, "Disable think/reasoning blocks (default: on)")
	think := flag.Bool("think", false, "Enable think/reasoning blocks (overrides -nothink)")
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
	interp := NewInterpreter(registry, client, *verbose, *maxTurns, *speak)

	noThinkMode := *nothink
	if *think {
		noThinkMode = false
	}
	interp.SetShowThink(!noThinkMode)
	if noThinkMode {
		client.SetThink(false)
	} else {
		client.SetThink(true)
	}

	systemPrompt := *sysPrompt
	if systemPrompt == "" {
		systemPrompt = BuildSystemPrompt(registry, noThinkMode)
	}

	fmt.Printf("🤖 AIML Agent — model: %s @ %s\n", *model, *host)
	if *speak {
		fmt.Println("  Speak mode ON — AI will read answers aloud")
	}
	if *listen {
		fmt.Println("  Listen mode ON — speak your questions instead of typing")
	}
	if noThinkMode {
		fmt.Println("  Think mode OFF — AI will respond directly without reasoning blocks")
	} else {
		fmt.Println("  Think mode ON — AI will show reasoning in <tool:thinking> blocks")
	}
	fmt.Println("Commands: /clear  /help  exit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		var input string
		if *listen {
			fmt.Print("🎤 Listening (5s)... ")
			input = listenOnce()
			if input == "" {
				fmt.Println("(nothing heard)")
				continue
			}
			fmt.Printf("You said: %s\n", input)
		} else {
			fmt.Print("You> ")
			if !scanner.Scan() {
				break
			}
			input = strings.TrimSpace(scanner.Text())
			if input == "" {
				continue
			}
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
			fmt.Println("  /set <key> <value>   Change a setting")
			fmt.Println("    Built-in: model, host, verbose, turns, think")
			fmt.Println("  /think               Enable thinking (top-level think=true)")
			fmt.Println("  /nothink             Disable thinking (top-level think=false)")
			fmt.Println("  /show                Show current settings + Ollama options")
			fmt.Println("  /clear               Clear conversation history")
			fmt.Println("  /help                Show this help")
			fmt.Println("  exit                 Quit")
			fmt.Println()
			continue

		case input == "/nothink" || input == "/set nothink":
			client.SetThink(false)
			interp.SetShowThink(false)
			noThinkMode = true
			systemPrompt = BuildSystemPrompt(registry, true)
			interp.Clear()
			fmt.Print("  ✓ Thinking OFF — think=false sent as top-level parameter.\n\n")
			continue

		case input == "/think" || input == "/set think":
			client.SetThink(true)
			interp.SetShowThink(true)
			noThinkMode = false
			systemPrompt = BuildSystemPrompt(registry, false)
			interp.Clear()
			fmt.Print("  ✓ Thinking ON — think=true sent as top-level parameter.\n\n")
			continue

		case strings.HasPrefix(input, "/set "):
			parts := strings.Fields(input[5:])
			if len(parts) < 2 {
				fmt.Println("Usage: /set <key> <value>. Try /help")
				fmt.Println()
				continue
			}
			key, val := parts[0], strings.Join(parts[1:], " ")
			switch key {
			case "model":
				client.SetModel(val)
				fmt.Printf("  ✓ Model set to %s\n\n", val)
			case "host":
				client.SetHost(val)
				fmt.Printf("  ✓ Host set to %s\n\n", val)
			case "verbose":
				v, err := strconv.ParseBool(val)
				if err != nil {
					fmt.Print("  ✗ verbose must be true or false\n\n")
					continue
				}
				interp.SetVerbose(v)
				fmt.Printf("  ✓ Verbose set to %v\n\n", v)
			case "turns":
				n, err := strconv.Atoi(val)
				if err != nil || n < 1 {
					fmt.Print("  ✗ turns must be a positive number\n\n")
					continue
				}
				interp.SetMaxTurns(n)
				fmt.Printf("  ✓ Max turns set to %d\n\n", n)
			case "think":
				v, err := strconv.ParseBool(val)
				if err != nil {
					fmt.Print("  ✗ think must be true or false\n\n")
					continue
				}
				client.SetThink(v)
				interp.SetShowThink(v)
				noThinkMode = !v
				systemPrompt = BuildSystemPrompt(registry, !v)
				fmt.Printf("  ✓ think=%v (top-level parameter)\n\n", v)
			default:
				// Assume it's an Ollama option
				if err := client.SetOption(key, val); err != nil {
					fmt.Printf("  ✗ %v\n\n", err)
				} else {
					fmt.Printf("  ✓ ollama.%s = %s\n\n", key, val)
				}
			}
			continue

		case input == "/show":
			thinkStr := "not set"
			if t := client.GetThink(); t != nil {
				thinkStr = fmt.Sprintf("%v", *t)
			}
			fmt.Printf("  Model:    %s\n", client.Model())
			fmt.Printf("  Host:     %s\n", client.Host())
			fmt.Printf("  Turns:    %d\n", interp.MaxTurns())
			fmt.Printf("  Verbose:  %v\n", interp.Verbose())
			fmt.Printf("  Think:    %s (top-level parameter)\n", thinkStr)
			fmt.Printf("  History:  %d messages\n", interp.MessageCount())
			fmt.Println("  Ollama options:")
			for k, v := range client.AllOptions() {
				fmt.Printf("    %s = %v\n", k, v)
			}
			fmt.Println()
			continue

		case strings.HasPrefix(input, "/"):
			fmt.Printf("Unknown command: %s. Try /help\n\n", input)
			continue
		}

		interp.Run(systemPrompt, input)

		// If speak mode: press any key to stop speaking
		if *speak {
			if cmd := interp.SpeakCmd(); cmd != nil && cmd.Process != nil {
				stop := make(chan struct{})
				go func() {
					one := make([]byte, 1)
					os.Stdin.Read(one)
					close(stop)
				}()
				done := make(chan struct{})
				go func() {
					cmd.Wait()
					close(done)
				}()
				select {
				case <-stop:
					cmd.Process.Kill()
				case <-done:
				}
			}
		}
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

func listenOnce() string {
	record := exec.Command("sh", "-c", "rec -q -r 16000 -c 1 -b 16 /tmp/aiml_input.wav 2>/dev/null")
	if err := record.Start(); err != nil {
		return ""
	}

	fmt.Print("(press Enter when done speaking) ")
	bufio.NewReader(os.Stdin).ReadString('\n')

	record.Process.Kill()
	record.Wait()

	out, err := exec.Command("sh", "-c", "whisper /tmp/aiml_input.wav --output_format txt --output_dir /tmp 2>/dev/null && cat /tmp/aiml_input.txt").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}