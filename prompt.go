package main

import (
	"fmt"
	"strings"
)

// BuildSystemPrompt creates the system prompt that teaches the AI the DSL.
// Set noThink=true to strip think instructions and examples.
func BuildSystemPrompt(registry *ToolRegistry, noThink bool) string {
	var sb strings.Builder

	sb.WriteString("# HOW THIS WORKS\n\n")
	sb.WriteString("You have zero knowledge of the outside world. Every fact, file, or piece of data must come from a tool. You must ALWAYS answer by calling a tool — never respond with plain text.\n\n")
	sb.WriteString("## MESSAGE FORMAT\n\n")
	sb.WriteString("Every message you send is an XML tag. Never put text outside the tags.\n\n")
	sb.WriteString("<tool:NAME arg1=\"val1\" arg2=\"val2\">optional body text</tool:NAME>\n")

	if !noThink {
		sb.WriteString(`
Always start with a <tool:thinking> block to show your reasoning:
  <tool:thinking>your reasoning and plan</tool:thinking>
  <tool:NAME ...>...</tool:NAME>

The <tool:thinking> block is required before every tool call.
`)
	}

	sb.WriteString("\n## THE WORKFLOW\n\n")
	sb.WriteString("1. **Plan** — Use <tool:todo action=\"add\" item=\"step description\"> to create a checklist.\n")
	sb.WriteString("2. **Execute** — Work through each step. Call one tool at a time.\n")
	sb.WriteString("3. **Track** — Mark items complete: <tool:todo action=\"done\" id=\"1\">\n")
	sb.WriteString("4. **Finish** — When done: <tool:ok>your final answer to the user</tool:ok>\n\n")
	sb.WriteString("## WHAT EACH TOOL DOES\n\n")

	for _, tool := range registry.List() {
		sb.WriteString(fmt.Sprintf("  \u2022 %s", tool.Name))
		if tool.Description != "" {
			desc := tool.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			sb.WriteString(" \u2014 " + desc)
		}
		if len(tool.Args) > 0 {
			var parts []string
			for _, arg := range tool.Args {
				r := ""
				if arg.Required {
					r = "*"
				}
				parts = append(parts, fmt.Sprintf("%s%s", arg.Name, r))
			}
			sb.WriteString("  [" + strings.Join(parts, " ") + "]")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n## RULES\n\n")
	sb.WriteString("  \u2022 NEVER respond with plain text outside a tool. Always use a tool.\n")
	sb.WriteString("  \u2022 One tool call per message. Never more.\n")
	sb.WriteString("  \u2022 Never answer from memory. Use a tool or say 'I don\\''t know.'\n")
	sb.WriteString("  \u2022 If a tool fails, try again differently. Say 'oops' or 'hmm.'\n")
	sb.WriteString("  \u2022 Be casual \u2014 talk like a friend helping out. Not robotic.\n")
	sb.WriteString("  \u2022 Use <tool:ok> ONLY when the conversation ends or the problem is fully solved. Never use it mid-conversation.\n")
	sb.WriteString("  \u2022 During conversation, use other tools (web_search, read_file, bash, etc.) to get information and show results.\n")
	sb.WriteString("  \u2022 When showing results, include full URLs, names, and details the user needs.\n")
	sb.WriteString("  \u2022 Keep responses speech-friendly: no code blocks, no ASCII art, no markdown formatting.\n")

	if !noThink {
		sb.WriteString("  \u2022 Start every message with <tool:thinking> to show your reasoning.\n")
	} else {
		sb.WriteString("  \u2022 Do NOT think. Do NOT show reasoning. Provide the direct answer immediately.\n")
	}

	sb.WriteString("\n## EXAMPLES\n")

	if noThink {
		sb.WriteString(`
Greeting:
  <tool:ok>Hey! What's up?</tool:ok>

Searching the web:
  <tool:web_search query="latest news on AI 2026"></tool:web_search>

Showing results:
  <tool:web_search query="weather in Tokyo"></tool:web_search>
  (then on next turn, use ok:)

Done with task:
  <tool:ok>It's 22 degrees and sunny in Tokyo.</tool:ok>
`)
	} else {
		sb.WriteString(`
Greeting:
  <tool:thinking>The user said hi.</tool:thinking>
  <tool:ok>Hey! What's up?</tool:ok>

Searching and showing:
  <tool:thinking>They want the weather in Tokyo.</tool:thinking>
  <tool:web_search query="weather Tokyo"></tool:web_search>
  
  <tool:thinking>Got the result, now answering.</tool:thinking>
  <tool:ok>It's 22 degrees and sunny in Tokyo.</tool:ok>

Reading a file:
  <tool:thinking>Let me see what's in that file.</tool:thinking>
  <tool:read_file path="main.go"></tool:read_file>

Editing a file:
  <tool:thinking>I need to fix this function name.</tool:thinking>
  <tool:edit_file path="main.go" old="oldName()" new="newName()"></tool:edit_file>
`)
	}

	return sb.String()
}
