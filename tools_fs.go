package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func registerFileTools(r *ToolRegistry) {
	// read_file
	r.Register(&ToolDef{
		Name:        "read_file",
		Description: "Read the contents of a file",
		Args: []ArgDef{
			{Name: "path", Type: "string", Required: true, Description: "Path to the file to read"},
			{Name: "lines", Type: "string", Required: false, Description: "Optional line range like '1-50'"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			path := attrs["path"]
			if path == "" {
				return ToolResult{Error: "read_file requires a 'path' attribute. Example: <tool:read_file path=\"myfile.txt\"></tool:read_file>"}
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot read '%s': %v. Check the path exists and you have permission.", path, err)}
			}

			content := string(data)
			lines := strings.Split(content, "\n")

			// Handle line range
			if lineRange, ok := attrs["lines"]; ok && lineRange != "" {
				var start, end int
				n, _ := fmt.Sscanf(lineRange, "%d-%d", &start, &end)
				if n == 2 && start > 0 && end >= start {
					if start > len(lines) {
						start = len(lines)
					}
					if end > len(lines) {
						end = len(lines)
					}
					lines = lines[start-1 : end]
					content = strings.Join(lines, "\n")
				}
			}

			// Truncate very large files
			if len(content) > 8000 {
				content = content[:8000] + "\n\n[... file truncated at 8000 chars. Use 'lines' attribute to read specific sections ...]"
			}

			return ToolResult{Output: fmt.Sprintf("File: %s (%d lines)\n---\n%s", path, len(lines), content)}
		},
	})

	// write_file
	r.Register(&ToolDef{
		Name:        "write_file",
		Description: "Write content to a file (creates or overwrites)",
		Args: []ArgDef{
			{Name: "path", Type: "string", Required: true, Description: "Path to write to"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			path := attrs["path"]
			if path == "" {
				return ToolResult{Error: "write_file requires a 'path' attribute. Example: <tool:write_file path=\"out.txt\">content here</tool:write_file>"}
			}

			// Create parent directories
			dir := filepath.Dir(path)
			if dir != "." && dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return ToolResult{Error: fmt.Sprintf("Cannot create directory '%s': %v", dir, err)}
				}
			}

			if err := os.WriteFile(path, []byte(body), 0644); err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot write '%s': %v", path, err)}
			}

			return ToolResult{Output: fmt.Sprintf("Written %d bytes to %s", len(body), path)}
		},
	})

	// append_file
	r.Register(&ToolDef{
		Name:        "append_file",
		Description: "Append content to an existing file (or create it)",
		Args: []ArgDef{
			{Name: "path", Type: "string", Required: true, Description: "Path to append to"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			path := attrs["path"]
			if path == "" {
				return ToolResult{Error: "append_file requires a 'path' attribute."}
			}

			f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot open '%s' for append: %v", path, err)}
			}
			defer f.Close()

			n, err := f.WriteString(body)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot append to '%s': %v", path, err)}
			}

			return ToolResult{Output: fmt.Sprintf("Appended %d bytes to %s", n, path)}
		},
	})

	// delete_file
	r.Register(&ToolDef{
		Name:        "delete_file",
		Description: "Delete a file",
		Args: []ArgDef{
			{Name: "path", Type: "string", Required: true, Description: "Path to the file to delete"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			path := attrs["path"]
			if path == "" {
				return ToolResult{Error: "delete_file requires a 'path' attribute."}
			}
			if err := os.Remove(path); err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot delete '%s': %v", path, err)}
			}
			return ToolResult{Output: fmt.Sprintf("Deleted: %s", path)}
		},
	})

	// list_dir
	r.Register(&ToolDef{
		Name:        "list_dir",
		Description: "List files and directories in a path",
		Args: []ArgDef{
			{Name: "path", Type: "string", Required: false, Description: "Directory path (defaults to current directory)"},
			{Name: "recursive", Type: "bool", Required: false, Description: "Set to 'true' to list recursively"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			path := attrs["path"]
			if path == "" {
				path = "."
			}

			recursive := attrs["recursive"] == "true"

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Directory: %s\n", path))

			if recursive {
				count := 0
				err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
					if err != nil {
						return nil
					}
					indent := strings.Repeat("  ", strings.Count(p, string(os.PathSeparator)))
					if info.IsDir() {
						sb.WriteString(fmt.Sprintf("%s📁 %s/\n", indent, info.Name()))
					} else {
						sb.WriteString(fmt.Sprintf("%s📄 %s (%d bytes)\n", indent, info.Name(), info.Size()))
					}
					count++
					if count > 200 {
						sb.WriteString("... (truncated at 200 entries)\n")
						return io.EOF
					}
					return nil
				})
				if err != nil && err != io.EOF {
					return ToolResult{Error: fmt.Sprintf("Cannot walk '%s': %v", path, err)}
				}
			} else {
				entries, err := os.ReadDir(path)
				if err != nil {
					return ToolResult{Error: fmt.Sprintf("Cannot read directory '%s': %v — does it exist?", path, err)}
				}
				for _, e := range entries {
					info, _ := e.Info()
					size := int64(0)
					if info != nil {
						size = info.Size()
					}
					if e.IsDir() {
						sb.WriteString(fmt.Sprintf("  📁 %s/\n", e.Name()))
					} else {
						sb.WriteString(fmt.Sprintf("  📄 %s (%d bytes)\n", e.Name(), size))
					}
				}
			}

			return ToolResult{Output: sb.String()}
		},
	})

	// make_dir
	r.Register(&ToolDef{
		Name:        "make_dir",
		Description: "Create a directory (and any parent directories)",
		Args: []ArgDef{
			{Name: "path", Type: "string", Required: true, Description: "Directory path to create"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			path := attrs["path"]
			if path == "" {
				return ToolResult{Error: "make_dir requires a 'path' attribute."}
			}
			if err := os.MkdirAll(path, 0755); err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot create directory '%s': %v", path, err)}
			}
			return ToolResult{Output: fmt.Sprintf("Created directory: %s", path)}
		},
	})

	// copy_file
	r.Register(&ToolDef{
		Name:        "copy_file",
		Description: "Copy a file from src to dst",
		Args: []ArgDef{
			{Name: "src", Type: "string", Required: true, Description: "Source file path"},
			{Name: "dst", Type: "string", Required: true, Description: "Destination file path"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			src := attrs["src"]
			dst := attrs["dst"]
			if src == "" || dst == "" {
				return ToolResult{Error: "copy_file requires 'src' and 'dst' attributes. Example: <tool:copy_file src=\"a.txt\" dst=\"b.txt\"></tool:copy_file>"}
			}

			in, err := os.Open(src)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot open source '%s': %v", src, err)}
			}
			defer in.Close()

			// Create dest dir if needed
			if dir := filepath.Dir(dst); dir != "." {
				os.MkdirAll(dir, 0755)
			}

			out, err := os.Create(dst)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot create destination '%s': %v", dst, err)}
			}
			defer out.Close()

			n, err := io.Copy(out, in)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Copy failed: %v", err)}
			}

			return ToolResult{Output: fmt.Sprintf("Copied %d bytes from %s to %s", n, src, dst)}
		},
	})

	// move_file
	r.Register(&ToolDef{
		Name:        "move_file",
		Description: "Move or rename a file",
		Args: []ArgDef{
			{Name: "src", Type: "string", Required: true, Description: "Source path"},
			{Name: "dst", Type: "string", Required: true, Description: "Destination path"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			src := attrs["src"]
			dst := attrs["dst"]
			if src == "" || dst == "" {
				return ToolResult{Error: "move_file requires 'src' and 'dst' attributes."}
			}
			if dir := filepath.Dir(dst); dir != "." {
				os.MkdirAll(dir, 0755)
			}
			if err := os.Rename(src, dst); err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot move '%s' to '%s': %v", src, dst, err)}
			}
			return ToolResult{Output: fmt.Sprintf("Moved %s → %s", src, dst)}
		},
	})

	// file_info
	r.Register(&ToolDef{
		Name:        "file_info",
		Description: "Get metadata about a file or directory (size, modified time, permissions)",
		Args: []ArgDef{
			{Name: "path", Type: "string", Required: true, Description: "Path to inspect"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			path := attrs["path"]
			if path == "" {
				return ToolResult{Error: "file_info requires a 'path' attribute."}
			}
			info, err := os.Stat(path)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot stat '%s': %v", path, err)}
			}
			kind := "file"
			if info.IsDir() {
				kind = "directory"
			}
			return ToolResult{Output: fmt.Sprintf(
				"Path: %s\nType: %s\nSize: %d bytes\nModified: %s\nPermissions: %s",
				path, kind, info.Size(), info.ModTime().Format("2006-01-02 15:04:05"), info.Mode().String(),
			)}
		},
	})

	// search_files
	r.Register(&ToolDef{
		Name:        "search_files",
		Description: "Search for files by name pattern in a directory",
		Args: []ArgDef{
			{Name: "dir", Type: "string", Required: false, Description: "Directory to search (default: current)"},
			{Name: "pattern", Type: "string", Required: true, Description: "Glob pattern like *.go or *.txt"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			dir := attrs["dir"]
			if dir == "" {
				dir = "."
			}
			pattern := attrs["pattern"]
			if pattern == "" {
				return ToolResult{Error: "search_files requires a 'pattern' attribute. Example: <tool:search_files pattern=\"*.go\"></tool:search_files>"}
			}

			var matches []string
			err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				matched, _ := filepath.Match(pattern, info.Name())
				if matched {
					matches = append(matches, p)
				}
				return nil
			})
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Search error: %v", err)}
			}

			if len(matches) == 0 {
				return ToolResult{Output: fmt.Sprintf("No files matching '%s' found in %s", pattern, dir)}
			}

			return ToolResult{Output: fmt.Sprintf("Found %d file(s):\n%s", len(matches), strings.Join(matches, "\n"))}
		},
	})

	// grep_file
	r.Register(&ToolDef{
		Name:        "grep_file",
		Description: "Search for text pattern inside a file and return matching lines",
		Args: []ArgDef{
			{Name: "path", Type: "string", Required: true, Description: "File to search in"},
			{Name: "pattern", Type: "string", Required: true, Description: "Text to search for (case-insensitive)"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			path := attrs["path"]
			pattern := attrs["pattern"]
			if path == "" || pattern == "" {
				return ToolResult{Error: "grep_file requires 'path' and 'pattern' attributes. Example: <tool:grep_file path=\"main.go\" pattern=\"func\"></tool:grep_file>"}
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot read '%s': %v", path, err)}
			}

			lines := strings.Split(string(data), "\n")
			patLower := strings.ToLower(pattern)
			var matches []string
			for i, line := range lines {
				if strings.Contains(strings.ToLower(line), patLower) {
					matches = append(matches, fmt.Sprintf("L%d: %s", i+1, line))
				}
			}

			if len(matches) == 0 {
				return ToolResult{Output: fmt.Sprintf("No lines containing '%s' found in %s", pattern, path)}
			}
			if len(matches) > 50 {
				matches = matches[:50]
				matches = append(matches, fmt.Sprintf("... (truncated, showing first 50 of many matches)"))
			}
			return ToolResult{Output: fmt.Sprintf("Matches for '%s' in %s:\n%s", pattern, path, strings.Join(matches, "\n"))}
		},
	})

	// edit_file
	r.Register(&ToolDef{
		Name:        "edit_file",
		Description: "Find exact text in a file and replace it. Use for surgical edits. Shows a diff of the change.",
		Args: []ArgDef{
			{Name: "path", Type: "string", Required: true, Description: "Path to the file to edit"},
			{Name: "old", Type: "string", Required: true, Description: "Exact text to find (must match exactly)"},
			{Name: "new", Type: "string", Required: true, Description: "Replacement text"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			path := attrs["path"]
			old := attrs["old"]
			newText := attrs["new"]
			if path == "" || old == "" {
				return ToolResult{Error: "edit_file requires 'path', 'old', and 'new' attributes. Example: <tool:edit_file path=\"main.go\" old=\"foo()\" new=\"bar()\"></tool:edit_file>"}
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot read '%s': %v", path, err)}
			}

			content := string(data)
			if !strings.Contains(content, old) {
				return ToolResult{Error: fmt.Sprintf("Cannot find the specified text in '%s'. The 'old' text must match exactly (including whitespace).", path)}
			}

			newContent := strings.Replace(content, old, newText, 1)
			if newContent == content {
				return ToolResult{Error: "No changes made (old and new text are identical)."}
			}

			if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot write '%s': %v", path, err)}
			}

			// Show a simple diff
			var sb strings.Builder
			fmt.Fprintf(&sb, "Edited: %s\n\n", path)
			fmt.Fprintf(&sb, "-%s\n", strings.SplitN(old, "\n", 2)[0])
			fmt.Fprintf(&sb, "+%s\n", strings.SplitN(newText, "\n", 2)[0])

			return ToolResult{Output: sb.String()}
		},
	})
}