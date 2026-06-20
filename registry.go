package main

import (
	"fmt"
	"strings"
)

// ArgDef describes a tool argument
type ArgDef struct {
	Name        string
	Type        string // string, int, bool
	Required    bool
	Description string
}

// ToolDef defines a tool
type ToolDef struct {
	Name        string
	Description string
	Args        []ArgDef
	Examples    []string // 2-3 usage examples showing the XML format
	Handler     func(attrs map[string]string, body string) ToolResult
}

// ToolResult is returned by every tool call
type ToolResult struct {
	Output string
	Error  string
	Done   bool // if true, stop the agentic loop
}

// ToolRegistry holds all registered tools
type ToolRegistry struct {
	tools map[string]*ToolDef
	order []string
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: map[string]*ToolDef{}}
}

func (r *ToolRegistry) Register(t *ToolDef) {
	r.tools[t.Name] = t
	r.order = append(r.order, t.Name)
}

func (r *ToolRegistry) Get(name string) (*ToolDef, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *ToolRegistry) List() []*ToolDef {
	out := make([]*ToolDef, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.tools[name])
	}
	return out
}

func (r *ToolRegistry) Dispatch(tag string, attrs map[string]string, body string) ToolResult {
	// strip "tool:" prefix
	name := tag
	if len(tag) > 5 && tag[:5] == "tool:" {
		name = tag[5:]
	}

	tool, ok := r.Get(name)
	if !ok {
		return ToolResult{
			Error: fmt.Sprintf("Unknown tool '%s'. Available tools: %s", name, r.listNames()),
		}
	}

	// Check required args
	for _, arg := range tool.Args {
		if arg.Required {
			if _, exists := attrs[arg.Name]; !exists {
				if body == "" || arg.Name != "content" {
					return ToolResult{
						Error: fmt.Sprintf("Tool '%s' requires argument '%s' (%s). Example: <%s %s=\"value\"></%s>",
							name, arg.Name, arg.Description, tag, arg.Name, tag),
					}
				}
			}
		}
	}

	return tool.Handler(attrs, body)
}

func RegisterAllTools(r *ToolRegistry) {
	registerFileTools(r)
	registerSystemTools(r)
	registerWebTools(r)
	registerExtraTools(r)
}

func (r *ToolRegistry) listNames() string {
	names := make([]string, 0, len(r.order))
	for _, n := range r.order {
		names = append(names, n)
	}
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

// Help returns a formatted help string for a tool (or all tools if name is empty).
func (r *ToolRegistry) Help(name string) string {
	if name == "" {
		var sb strings.Builder
		sb.WriteString("Available tools:\n")
		for _, t := range r.List() {
			sb.WriteString(fmt.Sprintf("\n  %s — %s\n", t.Name, t.Description))
		}
		sb.WriteString("\nUse <tool:help tool=\"TOOL_NAME\"> for details on a specific tool.")
		return sb.String()
	}

	t, ok := r.Get(name)
	if !ok {
		return fmt.Sprintf("Unknown tool '%s'. Use <tool:help> for a list.", name)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s — %s\n\n", t.Name, t.Description)

	if len(t.Args) > 0 {
		sb.WriteString("Parameters:\n")
		for _, a := range t.Args {
			req := ""
			if a.Required {
				req = " (required)"
			}
			fmt.Fprintf(&sb, "  %s (%s)%s — %s\n", a.Name, a.Type, req, a.Description)
		}
	} else {
		sb.WriteString("Parameters: none\n")
	}

	if len(t.Examples) > 0 {
		sb.WriteString("\nExamples:\n")
		for _, ex := range t.Examples {
			fmt.Fprintf(&sb, "  %s\n", ex)
		}
	} else {
		// Auto-generate a basic example from args
		var exAttrs, exTag string
		exTag = "tool:" + t.Name
		for _, a := range t.Args {
			if a.Required {
				exAttrs += fmt.Sprintf(` %s="<%s>"`, a.Name, a.Name)
			}
		}
		fmt.Fprintf(&sb, "\nExample:\n  <%s%s></%s>\n", exTag, exAttrs, exTag)
	}

	return sb.String()
}