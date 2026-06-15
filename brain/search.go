package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ============================================================================
// WEB SEARCH — DuckDuckGo HTML (no API key needed, no CAPTCHA)
// ============================================================================

// SearchResult represents a single search result
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// WebSearch performs a web search using DuckDuckGo HTML and returns results
func WebSearch(query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 5
	}

	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "fr-FR,fr;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 500*1024)) // 500KB max
	if err != nil {
		return nil, err
	}

	return parseDDGResults(string(body), maxResults), nil
}

// parseDDGResults extracts search results from DuckDuckGo HTML
func parseDDGResults(html string, maxResults int) []SearchResult {
	var results []SearchResult

	// Pattern: <a class="result__a" href="...">Title</a>
	// and: <a class="result__snippet" ...>Snippet</a>
	titleRe := regexp.MustCompile(`<a[^>]*class="result__a"[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`<a[^>]*class="result__snippet"[^>]*>(.*?)</a>`)

	titles := titleRe.FindAllStringSubmatch(html, maxResults*2)
	snippets := snippetRe.FindAllStringSubmatch(html, maxResults*2)

	for i, match := range titles {
		if i >= maxResults {
			break
		}

		resultURL := match[1]
		// DuckDuckGo wraps URLs in their redirect
		if strings.Contains(resultURL, "uddg=") {
			if u, err := url.Parse(resultURL); err == nil {
				if actual := u.Query().Get("uddg"); actual != "" {
					resultURL = actual
				}
			}
		}

		title := stripHTMLSimple(match[2])
		snippet := ""
		if i < len(snippets) {
			snippet = stripHTMLSimple(snippets[i][1])
		}

		if title != "" && resultURL != "" {
			results = append(results, SearchResult{
				Title:   title,
				URL:     resultURL,
				Snippet: snippet,
			})
		}
	}

	return results
}

// stripHTMLSimple removes HTML tags (local helper, avoids collision with tools.go)
func stripHTMLSimple(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	s = re.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#x27;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return strings.TrimSpace(s)
}

// FormatSearchResults formats search results for WhatsApp display
func FormatSearchResults(query string, results []SearchResult) string {
	if len(results) == 0 {
		return RenderWhatsApp(ResponseStyle{
			Title:      "Recherche Web",
			TitleEmoji: "🔍",
			Sections: []Section{
				{Content: fmt.Sprintf("Aucun resultat pour: *%s*", query)},
			},
		})
	}

	var items []string
	for i, r := range results {
		if i >= 5 {
			break
		}
		item := fmt.Sprintf("*%s*\n    %s\n    _%s_", r.Title, r.Snippet, r.URL)
		items = append(items, item)
	}

	return RenderWhatsApp(ResponseStyle{
		Title:      "Recherche Web",
		TitleEmoji: "🔍",
		Sections: []Section{
			{
				Title:      fmt.Sprintf("Resultats pour \"%s\"", query),
				TitleEmoji: "🌐",
				Items:      items,
			},
		},
		Footer: "Powered by DuckDuckGo",
	})
}
