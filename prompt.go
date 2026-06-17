package main

import (
	"fmt"
	"strings"
)

// BuildSystemPrompt creates the system prompt that teaches the AI the DSL
func BuildSystemPrompt(registry *ToolRegistry) string {
	var sb strings.Builder

	sb.WriteString(`You control this system through tool calls. Every output must contain <tool:...>.

<think>reasoning</think>
<tool:NAME key="val">body</tool:NAME>

RULES:
- Start with <think>. Then call exactly ONE tool per message.
- NEVER answer from memory. You have no knowledge of the current internet or filesystem.
- Always use tools to read files, search the web, run commands, etc.
- When done, call <tool:ok>your final answer</tool:ok>. The body of ok() is shown to the user.

Tools:`)

	for _, tool := range registry.List() {
		sb.WriteString(fmt.Sprintf("- %s", tool.Name))
		if tool.Description != "" {
			sb.WriteString(": " + tool.Description)
		}
		if len(tool.Args) > 0 {
			var reqs []string
			for _, arg := range tool.Args {
				if arg.Required {
					reqs = append(reqs, arg.Name)
				}
			}
			if len(reqs) > 0 {
				sb.WriteString(" (req: " + strings.Join(reqs, ", ") + ")")
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(`
EXAMPLES:
<think>I need to see what's in main.go.</think>
<tool:read_file path="main.go"></tool:read_file>

<think>Let me search the current directory.</think>
<tool:list_dir path="."></tool:list_dir>

<think>I'll find where foo is defined.</think>
<tool:grep_file path="main.go" pattern="foo"></tool:grep_file>

<think>I need to replace the old function name.</think>
<tool:edit_file path="main.go" old="oldFunc()" new="newFunc()"></tool:edit_file>

<think>The task is done.</think>
<tool:ok>Done! I renamed the function.</tool:ok>
`)

	return sb.String()
}