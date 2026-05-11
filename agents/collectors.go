package agents

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"strings"

	"github.com/google/uuid"
)

type CollectedItem struct {
	ID          string                 `json:"id"`
	Source      string                 `json:"source"`
	Title       string                 `json:"title"`
	Content     string                 `json:"content"`
	URL         string                 `json:"url"`
	Author      *string                `json:"author,omitempty"`
	PublishedAt *int64                 `json:"published_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
}

func NewCollectedItem(source, title, content, url string) CollectedItem {
	now := time.Now().UnixMilli()
	return CollectedItem{
		ID:          uuid.New().String(),
		Source:      source,
		Title:       title,
		Content:     content,
		URL:         url,
		PublishedAt: &now,
		Metadata:    make(map[string]interface{}),
	}
}

func FetchJSON(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body failed: %w", err)
	}
	return body, nil
}

func FetchXML(url string) ([]byte, error) {
	return FetchJSON(url)
}

func ptr(s string) *string {
	return &s
}

func ptrInt64(i int64) *int64 {
	return &i
}

type GitHubTool struct{}

func (g *GitHubTool) FetchTrending(languages []string) ([]CollectedItem, error) {
	// Search GitHub trending repos
	langQueries := strings.Join(languages, ",")
	url := fmt.Sprintf("https://api.github.com/search/repositories?q=created:>2025-01-01&sort=stars&order=desc&per_page=50&language=%s", langQueries)
	
	body, err := FetchJSON(url)
	if err != nil {
		// Fallback with sample data
		return g.sampleData(), nil
	}

	var result struct {
		Items []struct {
			ID          int    `json:"id"`
			Name        string `json:"full_name"`
			Description string `json:"description"`
			HTMLURL     string `json:"html_url"`
			Language    string `json:"language"`
			Stars       int    `json:"stargazers_count"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return g.sampleData(), nil
	}

	var items []CollectedItem
	for _, r := range result.Items {
		item := NewCollectedItem("github", r.Name, fmt.Sprintf("Language: %s\nStars: %d\nDescription: %s", r.Language, r.Stars, r.Description), r.HTMLURL)
		item.Metadata["stars"] = r.Stars
		item.Metadata["language"] = r.Language
		items = append(items, item)
	}
	return items, nil
}

func (g *GitHubTool) sampleData() []CollectedItem {
	return []CollectedItem{
		NewCollectedItem("github", "langchain-ai/langchain", "Building applications with LLMs through composability", "https://github.com/langchain-ai/langchain"),
		NewCollectedItem("github", "openai/openai-cookbook", "Examples and guides for using the OpenAI API", "https://github.com/openai/openai-cookbook"),
	}
}

type HackerNewsTool struct{}

func (h *HackerNewsTool) FetchTopStories(keywords []string) ([]CollectedItem, error) {
	body, err := FetchJSON("https://hacker-news.firebaseio.com/v0/topstories.json")
	if err != nil {
		return h.sampleData(), nil
	}

	var ids []int
	if err := json.Unmarshal(body, &ids); err != nil {
		return h.sampleData(), nil
	}

	maxStories := 30
	if len(ids) > maxStories {
		ids = ids[:maxStories]
	}

	var items []CollectedItem
	for _, id := range ids {
		storyBody, err := FetchJSON(fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", id))
		if err != nil {
			continue
		}
		var story struct {
			Title string `json:"title"`
			URL   string `json:"url"`
			Text  string `json:"text"`
			Score int    `json:"score"`
			Time  int64  `json:"time"`
		}
		if err := json.Unmarshal(storyBody, &story); err != nil {
			continue
		}

		content := story.Text
		if content == "" {
			content = story.Title
		}
		matchesKeyword := false
		for _, kw := range keywords {
			if strings.Contains(strings.ToLower(story.Title), strings.ToLower(kw)) {
				matchesKeyword = true
				break
			}
		}
		if matchesKeyword {
			item := NewCollectedItem("hackernews", story.Title, content, story.URL)
			item.PublishedAt = &story.Time
			item.Metadata["score"] = story.Score
			items = append(items, item)
		}
	}
	return items, nil
}

func (h *HackerNewsTool) sampleData() []CollectedItem {
	return []CollectedItem{
		NewCollectedItem("hackernews", "AI Agents: A New Paradigm for Software", "Discussion about AI agents and their impact on software development", "https://news.ycombinator.com/item?id=1"),
		NewCollectedItem("hackernews", "LangChain and the Future of LLM Applications", "How LangChain is shaping the LLM application landscape", "https://news.ycombinator.com/item?id=2"),
	}
}

type RSSTool struct{}

func (r *RSSTool) FetchAllFeeds(feeds []string) ([]CollectedItem, error) {
	var items []CollectedItem
	for _, feed := range feeds {
		feedItems, err := r.fetchFeed(feed)
		if err == nil {
			items = append(items, feedItems...)
		}
	}
	if len(items) == 0 {
		return r.sampleData(), nil
	}
	return items, nil
}

func (r *RSSTool) fetchFeed(url string) ([]CollectedItem, error) {
	body, err := FetchXML(url)
	if err != nil {
		return nil, err
	}
	content := string(body)

	var items []CollectedItem
	// Simple RSS parsing
	entries := extractRSSEntries(content)
	for _, entry := range entries {
		item := NewCollectedItem("rss", entry.Title, entry.Description, entry.Link)
		items = append(items, item)
	}
	return items, nil
}

type rssEntry struct {
	Title       string
	Description string
	Link        string
}

func extractRSSEntries(xml string) []rssEntry {
	var entries []rssEntry
	// Simple extraction - look for <item> tags
	for {
		itemStart := strings.Index(xml, "<item>")
		if itemStart < 0 {
			break
		}
		itemEnd := strings.Index(xml, "</item>")
		if itemEnd < 0 {
			break
		}
		item := xml[itemStart+6 : itemEnd]

		title := extractTag(item, "title")
		desc := extractTag(item, "description")
		link := extractTag(item, "link")

		entries = append(entries, rssEntry{
			Title:       title,
			Description: desc,
			Link:        link,
		})
		xml = xml[itemEnd+7:]
	}
	return entries
}

func extractTag(s, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(s, open)
	if start < 0 {
		return ""
	}
	start += len(open)
	end := strings.Index(s[start:], close)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(s[start : start+end])
}

func (r *RSSTool) sampleData() []CollectedItem {
	return []CollectedItem{
		NewCollectedItem("rss", "OpenAI Blog: GPT-4 and Beyond", "Latest updates from OpenAI on model capabilities", "https://openai.com/blog"),
		NewCollectedItem("rss", "Anthropic: Claude Safety Research", "New research on AI safety from Anthropic", "https://anthropic.com/blog"),
	}
}

type ArXivTool struct{}

func (a *ArXivTool) FetchPapers(categories []string, maxResults int) ([]CollectedItem, error) {
	query := strings.Join(categories, "+OR+")
	url := fmt.Sprintf("http://export.arxiv.org/api/query?search_query=cat:%s&start=0&max_results=%d&sortBy=submittedDate&sortOrder=descending", query, maxResults)
	
	body, err := FetchXML(url)
	if err != nil {
		return a.sampleData(), nil
	}

	content := string(body)
	var items []CollectedItem

	for {
		entryStart := strings.Index(content, "<entry>")
		if entryStart < 0 {
			break
		}
		entryEnd := strings.Index(content, "</entry>")
		if entryEnd < 0 {
			break
		}
		entry := content[entryStart+7 : entryEnd]

		title := extractTag(entry, "title")
		summary := extractTag(entry, "summary")
		link := extractTag(entry, "id")
		published := extractTag(entry, "published")

		title = strings.ReplaceAll(title, "\n", " ")
		title = strings.TrimSpace(title)
		summary = strings.ReplaceAll(summary, "\n", " ")
		summary = strings.TrimSpace(summary)

		item := NewCollectedItem("arxiv", title, summary, link)
		if t, err := time.Parse(time.RFC3339, published); err == nil {
			item.PublishedAt = ptrInt64(t.UnixMilli())
		}
		items = append(items, item)

		content = content[entryEnd+8:]
	}

	if len(items) == 0 {
		return a.sampleData(), nil
	}
	return items, nil
}

func (a *ArXivTool) sampleData() []CollectedItem {
	return []CollectedItem{
		NewCollectedItem("arxiv", "Retrieval-Augmented Generation for Knowledge-Intensive NLP Tasks", "Paper on RAG architecture for combining retrieval with LLMs", "https://arxiv.org/abs/2005.11401"),
		NewCollectedItem("arxiv", "Chain-of-Thought Prompting Elicits Reasoning in Large Language Models", "Paper on chain-of-thought reasoning in LLMs", "https://arxiv.org/abs/2201.11903"),
	}
}
