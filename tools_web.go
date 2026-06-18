package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

func registerWebTools(r *ToolRegistry) {
	registerWeather(r)
	registerWebSearch(r)
	registerHackerNews(r)
	registerReadPage(r)
}

func registerWeather(r *ToolRegistry) {
	r.Register(&ToolDef{
		Name:        "weather",
		Description: "Get current weather conditions for a location",
		Args: []ArgDef{
			{Name: "location", Type: "string", Required: true, Description: "City name or location (e.g. 'London', 'New York')"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			loc := attrs["location"]
			if loc == "" {
				return ToolResult{Error: "weather requires a 'location' attribute. Example: <tool:weather location=\"Tokyo\"></tool:weather>"}
			}

			client := &http.Client{Timeout: 10 * time.Second}
			url := fmt.Sprintf("https://wttr.in/%s?format=%%C+%%t+%%w+%%h&lang=en", url.PathEscape(loc))
			resp, err := client.Get(url)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot fetch weather: %v", err)}
			}
			defer resp.Body.Close()

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot read response: %v", err)}
			}

			text := strings.TrimSpace(string(data))
			if text == "" || strings.Contains(text, "Unknown location") {
				return ToolResult{Output: fmt.Sprintf("No weather data found for '%s'", loc)}
			}
			return ToolResult{Output: fmt.Sprintf("Weather in %s: %s", loc, text)}
		},
	})
}

func registerWebSearch(r *ToolRegistry) {
	r.Register(&ToolDef{
		Name:        "web_search",
		Description: "Search the web via DuckDuckGo. Returns titles, snippets, and URLs.",
		Args: []ArgDef{
			{Name: "query", Type: "string", Required: true, Description: "Search query"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			q := attrs["query"]
			if q == "" {
				q = strings.TrimSpace(body)
			}
			if q == "" {
				return ToolResult{Error: "web_search requires a 'query' attribute. Example: <tool:web_search query=\"golang tutorials\"></tool:web_search>"}
			}

			client := &http.Client{Timeout: 10 * time.Second}

			// Try DuckDuckGo HTML (non-JS) first, fall back to lite
			form := url.Values{"q": {q}}
			resp, err := client.PostForm("https://html.duckduckgo.com/html/", form)
			if err != nil {
				resp, err = client.PostForm("https://lite.duckduckgo.com/lite/", form)
				if err != nil {
					return ToolResult{Error: fmt.Sprintf("Search request failed: %v", err)}
				}
			}
			defer resp.Body.Close()

			html, err := io.ReadAll(resp.Body)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Read search results failed: %v", err)}
			}

			results := parseDDGHTML(string(html))
			if len(results) == 0 {
				return ToolResult{Output: fmt.Sprintf("No search results for '%s' — try a different query or use read_page.", q)}
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "Search results for '%s':\n\n", q)
			for i, r := range results {
				fmt.Fprintf(&sb, "%d. %s\n", i+1, r.Title)
				fmt.Fprintf(&sb, "   %s\n", r.URL)
				if r.Snippet != "" {
					fmt.Fprintf(&sb, "   %s\n", r.Snippet)
				}
				sb.WriteByte('\n')
			}

			return ToolResult{Output: strings.TrimSpace(sb.String())}
		},
	})
}

type ddgResult struct {
	Title   string
	URL     string
	Snippet string
}

func parseDDGHTML(html string) []ddgResult {
	var results []ddgResult

	// Match result blocks: <h2 class="result__title">...<a class="result__a" href="URL">TITLE</a>...
	blockRe := regexp.MustCompile(`<h2[^>]*class="[^"]*result__title[^"]*"[^>]*>.*?<a[^>]*href="([^"]+)"[^>]*>(.*?)</a>.*?</h2>`)
	snippetRe := regexp.MustCompile(`<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)

	blocks := blockRe.FindAllStringSubmatch(html, -1)
	snippets := snippetRe.FindAllStringSubmatch(html, -1)

	for _, m := range blocks {
		href := m[1]
		title := stripTags(m[2])
		if href == "" || title == "" {
			continue
		}
		// Skip DDG internal links
		if strings.HasPrefix(href, "/") || strings.HasPrefix(href, "#") {
			continue
		}
		// Decode HTML entities in URL
		href = entityReplacer.Replace(href)
		results = append(results, ddgResult{Title: title, URL: href})
	}

	// Attach snippets to matching results
	snipped := 0
	for _, s := range snippets {
		snip := strings.TrimSpace(stripTags(s[1]))
		snip = strings.ReplaceAll(snip, "\n", " ")
		snip = regexp.MustCompile(`\s+`).ReplaceAllString(snip, " ")
		if snip != "" && snipped < len(results) {
			results[snipped].Snippet = snip
			snipped++
		}
	}

	if len(results) > 10 {
		results = results[:10]
	}
	return results
}

// Fallback parser for lite.duckduckgo.com/lite/ format
func parseDDGLite(html string) []ddgResult {
	var results []ddgResult

	linkRe := regexp.MustCompile(`<a[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
	snipRe := regexp.MustCompile(`<td class="result-snippet">(.*?)</td>`)

	snippets := snipRe.FindAllStringSubmatch(html, -1)
	links := linkRe.FindAllStringSubmatch(html, -1)

	var searchLinks []ddgResult
	for _, m := range links {
		href := m[1]
		title := stripTags(m[2])
		if href == "" || title == "" {
			continue
		}
		if strings.HasPrefix(href, "/") || strings.HasPrefix(href, "#") {
			continue
		}
		if strings.Contains(href, "duckduckgo.com") {
			continue
		}
		searchLinks = append(searchLinks, ddgResult{Title: title, URL: href})
	}

	for i, s := range snippets {
		snip := strings.TrimSpace(stripTags(s[1]))
		snip = strings.ReplaceAll(snip, "\n", " ")
		snip = regexp.MustCompile(`\s+`).ReplaceAllString(snip, " ")
		if i < len(searchLinks) {
			searchLinks[i].Snippet = snip
		}
	}

	results = searchLinks
	if len(results) > 10 {
		results = results[:10]
	}
	return results
}

func registerHackerNews(r *ToolRegistry) {
	r.Register(&ToolDef{
		Name:        "hackernews",
		Description: "Get HN stories listing, or fetch a single post by id (includes title, URL, points, author, text body, and top comments).",
		Args: []ArgDef{
			{Name: "id", Type: "string", Required: false, Description: "Post ID to fetch full details (e.g. '48550936')"},
			{Name: "count", Type: "string", Required: false, Description: "Number of stories (default: 5, max: 30)"},
			{Name: "type", Type: "string", Required: false, Description: "Story type: top (default), new, best"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			// Single post fetch
			if idStr := attrs["id"]; idStr != "" {
				return fetchHNPost(idStr)
			}

			count := 5
			if c := attrs["count"]; c != "" {
				fmt.Sscanf(c, "%d", &count)
			}
			if count < 1 {
				count = 1
			}
			if count > 30 {
				count = 30
			}

			storyType := attrs["type"]
			if storyType == "" {
				storyType = "top"
			}

			client := &http.Client{Timeout: 15 * time.Second}

			idsURL := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/%sstories.json", storyType)
			resp, err := client.Get(idsURL)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot fetch HN stories: %v", err)}
			}

			var ids []int
			if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
				resp.Body.Close()
				return ToolResult{Error: fmt.Sprintf("Parse HN IDs: %v", err)}
			}
			resp.Body.Close()

			if len(ids) == 0 {
				return ToolResult{Output: "(no stories found)"}
			}
			if count > len(ids) {
				count = len(ids)
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "Top %d Hacker News (%s stories):\n\n", count, storyType)

			for i := 0; i < count; i++ {
				itemURL := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", ids[i])
				resp, err := client.Get(itemURL)
				if err != nil {
					continue
				}

				var item struct {
					Title       string `json:"title"`
					URL         string `json:"url"`
					Score       int    `json:"score"`
					By          string `json:"by"`
					Descendants int    `json:"descendants"`
				}

				if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
					resp.Body.Close()
					continue
				}
				resp.Body.Close()

				if item.Title == "" {
					continue
				}

				fmt.Fprintf(&sb, "%d. %s\n", i+1, item.Title)
				if item.URL != "" {
					fmt.Fprintf(&sb, "   %s\n", item.URL)
				}
				fmt.Fprintf(&sb, "   ▲%d  by %s  💬%d comments\n", item.Score, item.By, item.Descendants)
				sb.WriteByte('\n')
			}

			return ToolResult{Output: strings.TrimSpace(sb.String())}
		},
	})
}

type hnItem struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Text        string   `json:"text"`
	Score       int      `json:"score"`
	By          string   `json:"by"`
	Descendants int      `json:"descendants"`
	Kids        []int    `json:"kids"`
	Type        string   `json:"type"`
}

func fetchHNPost(idStr string) ToolResult {
	client := &http.Client{Timeout: 15 * time.Second}
	url := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%s.json", idStr)
	resp, err := client.Get(url)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("Cannot fetch HN item: %v", err)}
	}
	defer resp.Body.Close()

	var item hnItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return ToolResult{Error: fmt.Sprintf("Parse HN item: %v", err)}
	}
	if item.Title == "" && item.Text == "" {
		return ToolResult{Output: fmt.Sprintf("Item %s not found or has no content.", idStr)}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Title: %s\n", item.Title)
	if item.URL != "" {
		fmt.Fprintf(&sb, "URL: %s\n", item.URL)
	}
	fmt.Fprintf(&sb, "▲ %d  by %s  💬 %d comments\n", item.Score, item.By, item.Descendants)
	if item.Text != "" {
		stripped := stripHTML(item.Text)
		fmt.Fprintf(&sb, "\n---\n%s\n---\n", stripped)
	}

	// Fetch top-level comments
	if len(item.Kids) > 0 {
		maxComments := 5
		if len(item.Kids) < maxComments {
			maxComments = len(item.Kids)
		}
		fmt.Fprintf(&sb, "\nTop comments:\n")
		for i := 0; i < maxComments; i++ {
			commentURL := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", item.Kids[i])
			cresp, err := client.Get(commentURL)
			if err != nil {
				continue
			}
			var comment hnItem
			if err := json.NewDecoder(cresp.Body).Decode(&comment); err != nil {
				cresp.Body.Close()
				continue
			}
			cresp.Body.Close()
			if comment.By == "" || comment.Text == "" {
				continue
			}
			commentText := stripHTML(comment.Text)
			if len(commentText) > 500 {
				commentText = commentText[:500] + "..."
			}
			fmt.Fprintf(&sb, "\n  %s:\n  %s\n", comment.By, commentText)
		}
	}

	return ToolResult{Output: strings.TrimSpace(sb.String())}
}

func registerReadPage(r *ToolRegistry) {
	r.Register(&ToolDef{
		Name:        "read_page",
		Description: "Fetch a webpage and return its text content with HTML stripped.",
		Args: []ArgDef{
			{Name: "url", Type: "string", Required: true, Description: "Full URL to fetch (including https://)"},
		},
		Handler: func(attrs map[string]string, body string) ToolResult {
			pageURL := attrs["url"]
			if pageURL == "" {
				return ToolResult{Error: "read_page requires a 'url' attribute. Example: <tool:read_page url=\"https://example.com\"></tool:read_page>"}
			}

			client := &http.Client{Timeout: 15 * time.Second}
			req, err := http.NewRequest("GET", pageURL, nil)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Invalid URL: %v", err)}
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AIML-Agent/1.0)")

			resp, err := client.Do(req)
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Cannot fetch page: %v", err)}
			}
			defer resp.Body.Close()

			data, err := io.ReadAll(io.LimitReader(resp.Body, 200*1024))
			if err != nil {
				return ToolResult{Error: fmt.Sprintf("Read error: %v", err)}
			}

			text := stripHTML(string(data))
			text = strings.TrimSpace(text)
			if len(text) > 8000 {
				text = text[:8000] + "\n\n[... truncated at 8000 chars ...]"
			}
			if text == "" {
				return ToolResult{Output: "(page appears to be empty or requires JavaScript)"}
			}

			return ToolResult{Output: fmt.Sprintf("URL: %s\nStatus: %s\n---\n%s", pageURL, resp.Status, text)}
		},
	})
}

var (
	tagRe          = regexp.MustCompile(`<[^>]*>`)
	scriptStyleRe  = regexp.MustCompile(`(?i)<script[^>]*>[\s\S]*?</script>|<style[^>]*>[\s\S]*?</style>`)
	spaceRe        = regexp.MustCompile(`\s+`)
	entityReplacer = strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
		"&#x27;", "'",
		"&nbsp;", " ",
	)
)

func stripHTML(s string) string {
	s = scriptStyleRe.ReplaceAllString(s, "")
	s = tagRe.ReplaceAllString(s, "")
	s = entityReplacer.Replace(s)
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func stripTags(s string) string {
	return strings.TrimSpace(tagRe.ReplaceAllString(s, ""))
}
