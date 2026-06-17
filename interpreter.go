package main

import (
	"fmt"
	"strings"
	"time"
)

const (
	colorGray   = "\033[90m"
	colorGreen  = "\033[32;1m"
	colorWhite  = "\033[97;1m"
	colorCyan   = "\033[36;1m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorDim    = "\033[2m"
	colorReset  = "\033[0m"
)

type Interpreter struct {
	registry *ToolRegistry
	client   *OllamaClient
	verbose  bool
	maxTurns int
	messages []Message
}

func NewInterpreter(registry *ToolRegistry, client *OllamaClient, verbose bool, maxTurns int) *Interpreter {
	return &Interpreter{
		registry: registry,
		client:   client,
		verbose:  verbose,
		maxTurns: maxTurns,
	}
}

func (i *Interpreter) Clear() {
	i.messages = nil
}

// Run executes one user query through the agentic loop
func (i *Interpreter) Run(systemPrompt, userInput string) {
	i.messages = append(i.messages, Message{Role: "user", Content: userInput})

	fmt.Println()
	for turn := 0; turn < i.maxTurns; turn++ {
		fmt.Printf("🔄 Turn %d/%d...\n", turn+1, i.maxTurns)

		streamStart := time.Now()
		result, err := i.client.ChatStream(systemPrompt, i.messages, func(token string) {
			fmt.Print(token)
		})
		streamDur := time.Since(streamStart)
		raw := result.Content

		if err != nil {
			if raw == "" {
				fmt.Printf("\n❌ Ollama error: %v\n\n", err)
				return
			}
			fmt.Printf("\n⚠️  Partial error: %v\n\n", err)
		}

		if result.EvalCount > 0 {
			fmt.Printf("\n📊 %.2f tok/s  •  %d tokens  •  %s\n",
				result.TokPerSec, result.EvalCount, formatDuration(streamDur))
		} else if raw != "" {
			words := len(strings.Fields(raw))
			fmt.Printf("\n📊 ~%.1f tok/s (estimated)  •  %s\n",
				float64(words)/streamDur.Seconds(), formatDuration(streamDur))
		} else {
			fmt.Println()
		}

		i.messages = append(i.messages, Message{Role: "assistant", Content: raw})

		nodes, err := ParseBlocks(raw)
		if err != nil {
			fmt.Printf("⚠️  Parse error: %v\n", err)
			nodes = []*Node{{Tag: "text", Text: raw}}
		}

		done, toolResults, answer := i.processNodes(nodes)

		// If the AI put <answer> in the same turn as tool calls, it answered
		// before seeing results — discard the premature answer.
		if len(toolResults) > 0 && answer != "" {
			if i.verbose {
				fmt.Printf("   ⚠️ Premature <answer> discarded (tool calls were pending)\n")
			}
			answer = ""
		}

		// If ok() was called, treat its output as the final answer
		for _, tr := range toolResults {
			if tr.result.Done {
				answer = tr.result.Output
				break
			}
		}

		// Feed tool results back to the AI (unless ok() was called)
		if len(toolResults) > 0 && !done {
			feedback := buildFeedback(toolResults)
			i.messages = append(i.messages, Message{Role: "user", Content: feedback})
			if i.verbose {
				fmt.Printf("📨 Feedback to AI:\n%s\n\n", feedback)
			}
		}

		// Show the answer
		if answer != "" {
			fmt.Printf("\n💬 Answer:\n%s\n\n", answer)
		}

		// Stop conditions
		if done || answer != "" {
			break
		}

		if len(toolResults) == 0 && answer == "" {
			fmt.Printf("\n💬 Response:\n%s\n\n", raw)
			break
		}
	}
}

type toolCallResult struct {
	tag    string
	result ToolResult
}

func (i *Interpreter) processNodes(nodes []*Node) (done bool, toolResults []toolCallResult, answer string) {
	var answerParts []string

	for _, node := range nodes {
		switch node.Tag {
		case "think":
			fmt.Printf("%s  ── think ──────────────────────────────%s\n", colorGray, colorReset)
			fmt.Printf("  %s%s%s\n", colorGray, truncate(node.Text, 120), colorReset)

		case "answer":
			answerParts = append(answerParts, node.Text)

		case "loop":
			fmt.Printf("  %s↻ continuing%s  %s\n", colorYellow, colorReset, truncate(node.Text, 80))

		case "done":
			done = true
			if node.Text != "" {
				answerParts = append(answerParts, node.Text)
			}

		case "text":
			if node.Text != "" {
				answerParts = append(answerParts, node.Text)
			}

		default:
			tag := node.Tag
			isTool := strings.HasPrefix(tag, "tool:") || i.registry.tools[tag] != nil
			if isTool {
				toolName := tag
				if strings.HasPrefix(toolName, "tool:") {
					toolName = toolName[5:]
				}
				fmt.Printf("  %s── %s ──────────────────────────────%s\n", colorGreen, toolName, colorReset)
				if len(node.Attrs) > 0 {
					for k, v := range node.Attrs {
						fmt.Printf("  %s %s%s = %s%q%s\n", colorDim, k, colorReset, colorCyan, truncate(v, 60), colorReset)
					}
				}

				result := i.registry.Dispatch(tag, node.Attrs, node.Text)

				if result.Error != "" {
					fmt.Printf("  %s── error ──────────────────────────────%s\n", colorRed, colorReset)
					fmt.Printf("  %s✖ %s%s\n", colorRed, result.Error, colorReset)
				} else {
					fmt.Printf("  %s── output ────────────────────────────%s\n", colorDim, colorReset)
					if len(result.Output) > 100 {
						fmt.Printf("  %s%s%s\n", colorWhite, truncate(result.Output, 100), colorReset)
					} else {
						for _, line := range strings.Split(result.Output, "\n") {
							fmt.Printf("  %s%s%s\n", colorWhite, line, colorReset)
						}
					}
				}

				toolResults = append(toolResults, toolCallResult{tag: tag, result: result})

				if result.Done {
					done = true
				}
			}
		}
	}

	answer = strings.TrimSpace(strings.Join(answerParts, "\n"))
	return
}

func buildFeedback(results []toolCallResult) string {
	var sb strings.Builder
	sb.WriteString("<tool_results>\n")
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("<result for=\"%s\">\n", r.tag))
		if r.result.Error != "" {
			sb.WriteString(fmt.Sprintf("<error>%s</error>\n", r.result.Error))
		} else {
			sb.WriteString(fmt.Sprintf("<output>%s</output>\n", r.result.Output))
		}
		sb.WriteString("</result>\n")
	}
	sb.WriteString("</tool_results>\n")
	sb.WriteString("Continue. Use <answer>your final response</answer> when done.")
	return sb.String()
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}