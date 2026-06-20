# AIML — Agentic AI powered by Ollama

A Go-based agentic AI assistant that uses local LLMs via Ollama. The agent can reason, run shell commands, search the web, read files, and more — all through a structured tool-calling interface.

## Quick Start

```bash
# Build
go build -o aiml .

# Run (CLI)
./aiml -model qwen2.5:3b

# Run with web UI
./aiml -web -model qwen2.5:3b
```

Open http://localhost:6769 when using `-web`.

## Features

- **Tool-based agent loop** — AI calls tools via `<tool:name args="">` XML tags, sees results, and continues until done
- **Web UI** — sidebar with all tools listed, click to insert into context, thinking in gray italic, tool calls in boxes, answers in bold
- **Reasoning blocks** — `<tool:thinking>` shown in green in CLI, gray italic in web UI
- **Streaming output** — real-time token display
- **Speech (TTS)** — `edge-tts` with natural voice, fallback to `espeak`
- **Speech-to-text** — microphone input via `sox` + `whisper`
- **Raw mode** — bypasses Ollama's chat template for precise control
- **Think toggle** — runtime `/think` / `/nothink` or `-think` / `-nothink` flags

## CLI Commands

| Command | Description |
|---------|-------------|
| `/think` | Enable reasoning blocks |
| `/nothink` | Disable reasoning blocks |
| `/clear` | Clear conversation history |
| `/show` | Show current settings |
| `/set <key> <value>` | Change setting (model, host, verbose, turns, think) |
| `/help` | Show help |
| `exit` | Quit |

## Flags

```
-model <name>      Ollama model (default: qwen2.5:3b)
-host <url>        Ollama host (default: http://localhost:11434)
-turns <n>         Max agentic turns (default: 20)
-verbose           Show raw output
-speak             Read answers aloud via TTS
-listen            Microphone input
-think             Enable reasoning
-nothink           Disable reasoning (default)
-web               Start web UI on :6769
-web-addr <addr>   Web UI address (default: :6769)
-system <file>     Override system prompt
-list-tools        List all available tools
```

## Tools

The agent has access to these tools (click any in the web UI sidebar for details + examples):

### System
- `bash` — Run shell commands
- `help` — Show tool documentation
- `continue` — Non-loop-ending message
- `thinking` — Reasoning block
- `ok` — Final answer (ends the loop)
- `todo` — Track task progress

### Filesystem
- `read_file` — Read file contents
- `write_file` — Write/create files
- `edit_file` — Edit existing files
- `delete_file` — Delete files
- `list_dir` — List directory contents
- `grep` — Search file contents
- `search` — Glob file search

### Web
- `web_search` — DuckDuckGo search, saves results to `search_results/*.md`
- `hackernews` — Hacker News stories + post details
- `read_page` — Fetch and strip HTML from URLs
- `weather` — Current weather via wttr.in

### Extra
- `speak` — Speak text aloud
- `listen` — Capture microphone input

## Web UI

Run with `-web` to start the web interface at `:6769`:

- **Sidebar** — lists all tools; click any to see params, types, examples, and insert the XML into your message
- **Chat display** — user messages in blue bubbles, AI thinking in gray italic, tool calls in bordered boxes, answers in bold
- **Persistent context** — conversation history is maintained across messages

## Architecture

```
main.go          — CLI entry point, flag parsing, input loop
interpreter.go   — Agent loop: calls Ollama, parses tool XML, dispatches, feeds back results
ollama.go        — HTTP client for Ollama API (streaming, think param, raw mode)
parser.go        — XML tag lexer/parser for <tool:name> <think> <answer> syntax
registry.go      — Tool registration and dispatch
prompt.go        — System prompt builder
web.go           — HTTP server + HTML/CSS/JS web UI
tools_*.go       — Tool implementations (system, fs, web, extra)
```

The agent loop:
1. User message → appended to history
2. ChatStream → Ollama generates response with streaming tokens
3. ParseBlocks → splits response into nodes (think, tool calls, text, answer)
4. processNodes → dispatches tool calls, collects results
5. Results fed back as `<tool_results>` for next turn
6. Loop ends when `<tool:ok>` is called or max turns reached
