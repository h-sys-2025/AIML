package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type EventType string

const (
	EventToken      EventType = "token"
	EventThink      EventType = "think"
	EventToolCall   EventType = "tool_call"
	EventToolOutput EventType = "tool_output"
	EventToolError  EventType = "tool_error"
	EventAnswer     EventType = "answer"
	EventResponse   EventType = "response"
	EventStats      EventType = "stats"
	EventError      EventType = "error"
	EventTurn       EventType = "turn"
	EventFeedback   EventType = "feedback"
)

type OutputEvent struct {
	Type      EventType          `json:"type"`
	Content   string             `json:"content,omitempty"`
	ToolName  string             `json:"toolName,omitempty"`
	Attrs     map[string]string  `json:"attrs,omitempty"`
	TokPerSec float64            `json:"tokPerSec,omitempty"`
	EvalCount int                `json:"evalCount,omitempty"`
	Duration  string             `json:"duration,omitempty"`
	Turn      int                `json:"turn,omitempty"`
	MaxTurns  int                `json:"maxTurns,omitempty"`
}

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
	speak    bool
	showThink  bool
	speakCmd   *exec.Cmd
	eventCb    func(OutputEvent)
}

func NewInterpreter(registry *ToolRegistry, client *OllamaClient, verbose bool, maxTurns int, speak bool) *Interpreter {
	return &Interpreter{
		registry:  registry,
		client:    client,
		verbose:   verbose,
		maxTurns:  maxTurns,
		speak:     speak,
		showThink: true,
	}
}

func (i *Interpreter) SpeakCmd() *exec.Cmd { return i.speakCmd }

func (i *Interpreter) Clear() {
	i.messages = nil
}

func (i *Interpreter) SetVerbose(v bool)    { i.verbose = v }
func (i *Interpreter) SetMaxTurns(n int)    { i.maxTurns = n }
func (i *Interpreter) SetSpeak(v bool)      { i.speak = v }
func (i *Interpreter) SetShowThink(v bool)  { i.showThink = v }
func (i *Interpreter) SetEventCb(cb func(OutputEvent)) { i.eventCb = cb }
func (i *Interpreter) emit(ev OutputEvent) {
	if i.eventCb != nil {
		i.eventCb(ev)
	}
}
func (i *Interpreter) Verbose() bool        { return i.verbose }
func (i *Interpreter) MaxTurns() int        { return i.maxTurns }
func (i *Interpreter) Speak() bool          { return i.speak }
func (i *Interpreter) ShowThink() bool      { return i.showThink }
func (i *Interpreter) MessageCount() int    { return len(i.messages) }

func speakText(text string) *exec.Cmd {
	// Try edge-tts first (natural voice, requires pip install edge-tts + ffmpeg)
	edgeCmd := exec.Command("edge-tts",
		"--voice", "en-US-JennyNeural",
		"--text", text,
		"--write-media", "/tmp/aiml_tts.mp3",
	)
	if err := edgeCmd.Run(); err == nil {
		playCmd := exec.Command("ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet", "/tmp/aiml_tts.mp3")
		playCmd.Start()
		go func() {
			playCmd.Wait()
			os.Remove("/tmp/aiml_tts.mp3")
		}()
		return playCmd
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("say", text)
	} else {
		cmd = exec.Command("espeak", text)
	}
	cmd.Start()
	return cmd
}

// Run executes one user query through the agentic loop
func (i *Interpreter) Run(systemPrompt, userInput string) {
	i.messages = append(i.messages, Message{Role: "user", Content: userInput})

	for turn := 0; turn < i.maxTurns; turn++ {
		i.emit(OutputEvent{Type: EventTurn, Turn: turn + 1, MaxTurns: i.maxTurns})

		streamStart := time.Now()
		result, err := i.client.ChatStream(systemPrompt, i.messages, func(token string) {
			i.emit(OutputEvent{Type: EventToken, Content: token})
		})
		streamDur := time.Since(streamStart)
		raw := result.Content

		if err != nil {
			if raw == "" {
				i.emit(OutputEvent{Type: EventError, Content: fmt.Sprintf("Ollama error: %v", err)})
				return
			}
			i.emit(OutputEvent{Type: EventError, Content: fmt.Sprintf("Partial error: %v", err)})
		}

		if result.EvalCount > 0 {
			i.emit(OutputEvent{Type: EventStats, TokPerSec: result.TokPerSec, EvalCount: result.EvalCount, Duration: formatDuration(streamDur)})
		} else if raw != "" {
			words := len(strings.Fields(raw))
			i.emit(OutputEvent{Type: EventStats, TokPerSec: float64(words) / streamDur.Seconds(), Duration: formatDuration(streamDur)})
		}

		i.messages = append(i.messages, Message{Role: "assistant", Content: raw})

		nodes, err := ParseBlocks(raw)
		if err != nil {
			nodes = []*Node{{Tag: "text", Text: raw}}
		}

		done, toolResults, answer := i.processNodes(nodes)

		if len(toolResults) > 0 && answer != "" {
			answer = ""
		}

		for _, tr := range toolResults {
			if tr.result.Done {
				answer = tr.result.Output
				break
			}
		}

		if len(toolResults) > 0 && !done {
			feedback := buildFeedback(toolResults)
			i.messages = append(i.messages, Message{Role: "user", Content: feedback})
			i.emit(OutputEvent{Type: EventFeedback, Content: feedback})
		}

		if answer != "" {
			i.emit(OutputEvent{Type: EventAnswer, Content: answer})
			if i.speak {
				i.speakCmd = speakText(answer)
			}
		}

		if done || answer != "" {
			break
		}

		if len(toolResults) == 0 && answer == "" {
			i.emit(OutputEvent{Type: EventResponse, Content: raw})
			break
		}
	}

	// Always emit a terminal event so web clients know the loop finished
	i.emit(OutputEvent{Type: EventTurn, Content: "end"})
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
			if i.showThink {
				i.emit(OutputEvent{Type: EventThink, Content: node.Text})
			}

		case "answer":
			answerParts = append(answerParts, node.Text)

		case "loop":
			i.emit(OutputEvent{Type: EventThink, Content: "(continuing) " + truncate(node.Text, 80)})

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

				result := i.registry.Dispatch(tag, node.Attrs, node.Text)

				i.emit(OutputEvent{
					Type:     EventToolCall,
					ToolName: toolName,
					Attrs:    node.Attrs,
					Content:  node.Text,
				})

				if result.Error != "" {
					i.emit(OutputEvent{
						Type:     EventToolError,
						ToolName: toolName,
						Content:  result.Error,
					})
				} else {
					i.emit(OutputEvent{
						Type:     EventToolOutput,
						ToolName: toolName,
						Content:  result.Output,
					})
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