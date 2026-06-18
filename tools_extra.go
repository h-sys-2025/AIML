package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

var todos []string

func registerExtraTools(r *ToolRegistry) {
	// todo
	r.Register(&ToolDef{
		Name:        "todo",
		Description: "Manage a todo list. Actions: add <item>, done <id>, list, remove <id>, clear. Use this to create a plan and work through it step by step.",
		Args: []ArgDef{
			{Name: "action", Type: "string", Required: true, Description: "add, done, list, remove, or clear"},
			{Name: "item", Type: "string", Required: false, Description: "Text for 'add' action"},
			{Name: "id", Type: "string", Required: false, Description: "Todo number for 'done' or 'remove'"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			action := attrs["action"]
			switch action {
			case "add":
				item := attrs["item"]
				if item == "" {
					item = strings.TrimSpace(body)
				}
				if item == "" {
					return ToolResult{Error: "todo add needs an 'item' attribute or body text."}
				}
				todos = append(todos, item)
				return ToolResult{Output: fmt.Sprintf("📋 Added todo #%d: %s", len(todos), item)}

			case "done":
				idStr := attrs["id"]
				if idStr == "" {
					return ToolResult{Error: "todo done needs an 'id' attribute."}
				}
				id, err := strconv.Atoi(idStr)
				if err != nil || id < 1 || id > len(todos) {
					return ToolResult{Error: fmt.Sprintf("Invalid todo id '%s'. Current todos: %d", idStr, len(todos))}
				}
				item := todos[id-1]
				todos = append(todos[:id-1], todos[id:]...)
				return ToolResult{Output: fmt.Sprintf("✅ Done: %s", item)}

			case "list":
				if len(todos) == 0 {
					return ToolResult{Output: "📋 Todo list is empty."}
				}
				var sb strings.Builder
				sb.WriteString("📋 Todo list:\n")
				for i, item := range todos {
					fmt.Fprintf(&sb, "  %d. %s\n", i+1, item)
				}
				return ToolResult{Output: strings.TrimSpace(sb.String())}

			case "remove":
				idStr := attrs["id"]
				if idStr == "" {
					return ToolResult{Error: "todo remove needs an 'id' attribute."}
				}
				id, err := strconv.Atoi(idStr)
				if err != nil || id < 1 || id > len(todos) {
					return ToolResult{Error: fmt.Sprintf("Invalid todo id '%s'.", idStr)}
				}
				item := todos[id-1]
				todos = append(todos[:id-1], todos[id:]...)
				return ToolResult{Output: fmt.Sprintf("🗑️ Removed: %s", item)}

			case "clear":
				count := len(todos)
				todos = nil
				return ToolResult{Output: fmt.Sprintf("🧹 Cleared %d todos.", count)}

			default:
				return ToolResult{Error: fmt.Sprintf("Unknown action '%s'. Use: add, done, list, remove, clear", action)}
			}
		},
	})

	// speak - TTS via espeak or say
	r.Register(&ToolDef{
		Name:        "speak",
		Description: "Read text aloud using text-to-speech (espeak on Linux, say on macOS). Use ONLY when the user explicitly asked for spoken output or when using -speak mode.",
		Args: []ArgDef{
			{Name: "text", Type: "string", Required: true, Description: "Text to speak aloud"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			text := attrs["text"]
			if text == "" {
				text = strings.TrimSpace(body)
			}
			if text == "" {
				return ToolResult{Error: "speak needs a 'text' attribute or body content."}
			}

			var cmd *exec.Cmd
			if runtime.GOOS == "darwin" {
				cmd = exec.Command("say", text)
			} else {
				cmd = exec.Command("espeak", text)
			}

			if err := cmd.Run(); err != nil {
				return ToolResult{Error: fmt.Sprintf("TTS command failed (is espeak/say installed?): %v", err)}
			}
			return ToolResult{Output: fmt.Sprintf("🔊 Spoke %d characters.", len(text))}
		},
	})

	// listen - STT via whisper
	r.Register(&ToolDef{
		Name:        "listen",
		Description: "Record microphone input and transcribe it using 'whisper'. Run 'whisper --help' first to ensure it is installed.",
		Args: []ArgDef{
			{Name: "duration", Type: "string", Required: false, Description: "Seconds to record (default: 5)"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			dur := "5"
			if d := attrs["duration"]; d != "" {
				dur = d
			}

			// Use sox/rec to record, then whisper to transcribe
			recordCmd := fmt.Sprintf("rec -q -r 16000 -c 1 -b 16 /tmp/stt_input.wav trim 0 %s 2>/dev/null", dur)
			if err := exec.Command("sh", "-c", recordCmd).Run(); err != nil {
				return ToolResult{Error: fmt.Sprintf("Recording failed (is 'sox' installed?): %v", err)}
			}

			transcript, err := exec.Command("sh", "-c", "whisper /tmp/stt_input.wav --output_format txt --output_dir /tmp 2>/dev/null && cat /tmp/stt_input.txt").Output()
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Transcription failed (is 'whisper' installed?): %v", err)}
			}

			result := strings.TrimSpace(string(transcript))
			if result == "" {
				result = "(no speech detected)"
			}
			return ToolResult{Output: fmt.Sprintf("🎤 Heard: %s", result)}
		},
	})
}
