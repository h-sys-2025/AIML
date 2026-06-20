package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func registerSystemTools(r *ToolRegistry) {
	// bash
	r.Register(&ToolDef{
		Name:        "bash",
		Description: "Run a shell command. Output shows the command, stdout/stderr, and exit code (0=success).",
		Args: []ArgDef{
			{Name: "cmd", Type: "string", Required: true, Description: "Shell command to run"},
			{Name: "timeout", Type: "int", Required: false, Description: "Timeout in seconds (default: 30)"},
			{Name: "dir", Type: "string", Required: false, Description: "Working directory for the command"},
		},
		Examples: []string{
			`<tool:bash cmd="ls -la"></tool:bash>`,
			`<tool:bash cmd="go build ./..." dir="/home/project"></tool:bash>`,
			`<tool:bash cmd="ping -c 4 google.com" timeout="10"></tool:bash>`,
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			cmd := attrs["cmd"]
			if cmd == "" {
				cmd = strings.TrimSpace(body)
			}
			if cmd == "" {
				return ToolResult{Error: "bash requires a 'cmd' attribute. Example: <tool:bash cmd=\"ls -la\"></tool:bash>"}
			}

			timeoutSecs := 30
			if t := attrs["timeout"]; t != "" {
				fmt.Sscanf(t, "%d", &timeoutSecs)
			}

			var c *exec.Cmd
			if runtime.GOOS == "windows" {
				c = exec.Command("cmd", "/C", cmd)
			} else {
				c = exec.Command("sh", "-c", cmd)
			}

			if dir := attrs["dir"]; dir != "" {
				c.Dir = dir
			}

			var outBuf, errBuf bytes.Buffer
			c.Stdout = &outBuf
			c.Stderr = &errBuf

			done := make(chan error, 1)
			if err := c.Start(); err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot start command: %v", err)}
			}

			go func() { done <- c.Wait() }()

			select {
			case err := <-done:
				stdout := outBuf.String()
				stderr := errBuf.String()

				exitCode := 0
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						exitCode = exitErr.ExitCode()
					} else {
						exitCode = -1
					}
				}

				var sb strings.Builder
				fmt.Fprintf(&sb, "$ %s\n", cmd)
				if stdout != "" {
					sb.WriteString(stdout)
					if !strings.HasSuffix(stdout, "\n") {
						sb.WriteByte('\n')
					}
				}
				if stderr != "" {
					sb.WriteString("[stderr]\n" + stderr)
					if !strings.HasSuffix(stderr, "\n") {
						sb.WriteByte('\n')
					}
				}
				fmt.Fprintf(&sb, "→ exit %d", exitCode)

				out := sb.String()
				if len(out) > 6000 {
					out = out[:6000] + "\n... (output truncated)"
				}
				return ToolResult{Output: out}

			case <-time.After(time.Duration(timeoutSecs) * time.Second):
				c.Process.Kill()
				return ToolResult{Error: fmt.Sprintf("Command timed out after %d seconds", timeoutSecs)}
			}
		},
	})

	// env_get
	r.Register(&ToolDef{
		Name:        "env_get",
		Description: "Read an environment variable",
		Args: []ArgDef{
			{Name: "name", Type: "string", Required: true, Description: "Environment variable name"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			name := attrs["name"]
			if name == "" {
				return ToolResult{Error: "env_get requires a 'name' attribute."}
			}
			val, ok := os.LookupEnv(name)
			if !ok {
				return ToolResult{Output: fmt.Sprintf("Environment variable '%s' is not set.", name)}
			}
			return ToolResult{Output: fmt.Sprintf("%s=%s", name, val)}
		},
	})

	// env_set
	r.Register(&ToolDef{
		Name:        "env_set",
		Description: "Set an environment variable for this session",
		Args: []ArgDef{
			{Name: "name", Type: "string", Required: true, Description: "Variable name"},
			{Name: "value", Type: "string", Required: true, Description: "Variable value"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			name := attrs["name"]
			value := attrs["value"]
			if name == "" {
				return ToolResult{Error: "env_set requires 'name' and 'value' attributes."}
			}
			os.Setenv(name, value)
			return ToolResult{Output: fmt.Sprintf("Set %s=%s", name, value)}
		},
	})

	// pwd
	r.Register(&ToolDef{
		Name:        "pwd",
		Description: "Print the current working directory",
		Args:        []ArgDef{},
		Handler: func(attrs map[string]string, body string) ToolResult {
			dir, err := os.Getwd()
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot get working directory: %v", err)}
			}
			return ToolResult{Output: dir}
		},
	})

	// cd
	r.Register(&ToolDef{
		Name:        "cd",
		Description: "Change the current working directory",
		Args: []ArgDef{
			{Name: "path", Type: "string", Required: true, Description: "Directory to change to"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			path := attrs["path"]
			if path == "" {
				return ToolResult{Error: "cd requires a 'path' attribute."}
			}
			if err := os.Chdir(path); err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot change to '%s': %v", path, err)}
			}
			dir, _ := os.Getwd()
			return ToolResult{Output: fmt.Sprintf("Changed to: %s", dir)}
		},
	})

	// sys_info
	r.Register(&ToolDef{
		Name:        "sys_info",
		Description: "Get system information (OS, architecture, hostname)",
		Args:        []ArgDef{},
		Handler: func(attrs map[string]string, body string) ToolResult {
			hostname, _ := os.Hostname()
			wd, _ := os.Getwd()
			return ToolResult{Output: fmt.Sprintf(
				"OS: %s\nArch: %s\nHostname: %s\nCPUs: %d\nWorking Dir: %s\nGo version: %s",
				runtime.GOOS, runtime.GOARCH, hostname,
				runtime.NumCPU(), wd, runtime.Version(),
			)}
		},
	})

	// ok
	r.Register(&ToolDef{
		Name:        "ok",
		Description: "Call this when the task is complete and you have the final answer. The body text will be shown as the result.",
		Args:        []ArgDef{},
		Examples: []string{
			`<tool:ok>Done! The file has been created.</tool:ok>`,
			`<tool:ok>It's 22 degrees and sunny in Tokyo.</tool:ok>`,
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			out := strings.TrimSpace(body)
			if out == "" {
				out = "Done."
			}
			return ToolResult{Output: out, Done: true}
		},
	})

	// continue
	r.Register(&ToolDef{
		Name:        "continue",
		Description: "Show a message to the user and keep the agentic loop running. Use this for progress updates, asking questions, or when you need more info before finishing.",
		Args:        []ArgDef{},
		Examples: []string{
			`<tool:continue>I'm looking that up now...</tool:continue>`,
			`<tool:continue>Let me check the weather for both days.</tool:continue>`,
			`<tool:continue>Do you want the short or long version?</tool:continue>`,
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			text := strings.TrimSpace(body)
			if text == "" {
				text = "..."
			}
			return ToolResult{Output: text}
		},
	})

	// thinking
	r.Register(&ToolDef{
		Name:        "thinking",
		Description: "Show your internal reasoning / thought process. Use this BEFORE every tool call to explain what you're doing and why.",
		Args:        []ArgDef{},
		Examples: []string{
			`<tool:thinking>The user wants the weather, I'll search for Tokyo.</tool:thinking>`,
			`<tool:thinking>Step 1 is done, now moving to step 2.</tool:thinking>`,
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			text := strings.TrimSpace(body)
			if text == "" {
				text = "(thinking...)"
			}
			return ToolResult{Output: "💭 " + text}
		},
	})

	// which
	r.Register(&ToolDef{
		Name:        "which",
		Description: "Find the path of a command/binary",
		Args: []ArgDef{
			{Name: "cmd", Type: "string", Required: true, Description: "Command name to find"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			cmd := attrs["cmd"]
			if cmd == "" {
				return ToolResult{Error: "which requires a 'cmd' attribute."}
			}
			path, err := exec.LookPath(cmd)
			if err != nil {
				return ToolResult{Output: fmt.Sprintf("'%s' not found in PATH", cmd)}
			}
			return ToolResult{Output: path}
		},
	})

	// help
	r.Register(&ToolDef{
		Name:        "help",
		Description: "Show detailed help for a tool: its parameters, types, and usage examples.",
		Args: []ArgDef{
			{Name: "tool", Type: "string", Required: false, Description: "Tool name to get help for (leave empty for list of all tools)"},
		},
		Examples: []string{
			`<tool:help tool="web_search">`,
			`<tool:help tool="bash">`,
			`<tool:help>  (lists all tools)`,
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			toolName := attrs["tool"]
			if toolName == "" {
				toolName = strings.TrimSpace(body)
			}
			return ToolResult{Output: r.Help(toolName)}
		},
	})
}