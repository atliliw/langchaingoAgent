package models

// AggregatedContent — One collected content item
type AggregatedContent struct {
	ID          string                 `json:"id"`
	Source      string                 `json:"source"`
	Title       string                 `json:"title"`
	Content     string                 `json:"content"`
	URL         string                 `json:"url"`
	Author      *string                `json:"author,omitempty"`
	PublishedAt *int64                 `json:"published_at,omitempty"`
	CollectedAt int64                  `json:"collected_at"`
	Summary     *string                `json:"summary,omitempty"`
	Keywords    []string               `json:"keywords"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// CollectRequest — POST /api/aggregate/collect
type CollectRequest struct {
	Sources *[]string `json:"sources,omitempty"`
	Force   *bool     `json:"force,omitempty"`
}

// CollectResponse — Collect response
type CollectResponse struct {
	Success        bool            `json:"success"`
	CollectedCount int             `json:"collected_count"`
	Records        []CollectRecord `json:"records"`
}

// CollectRecord — One source collection record
type CollectRecord struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
	Status string `json:"status"`
}

// AggregateListQuery — GET /api/aggregate/list query params
type AggregateListQuery struct {
	Source *string `json:"source,omitempty"`
	Limit  *int    `json:"limit,omitempty"`
	Offset *int    `json:"offset,omitempty"`
}

// AggregateListResponse — List response
type AggregateListResponse struct {
	Total int                 `json:"total"`
	Items []AggregatedContent `json:"items"`
}

// AggregateSearchRequest — POST /api/aggregate/search
type AggregateSearchRequest struct {
	Query string `json:"query"`
	TopK  *int   `json:"top_k,omitempty"`
}

// AggregateSearchResult — One search result in aggregated content
type AggregateSearchResult struct {
	ID      string  `json:"id"`
	Source  string  `json:"source"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	URL     string  `json:"url"`
	Score   float32 `json:"score"`
	Summary *string `json:"summary,omitempty"`
}

// AggregateSearchResponse — Search response
type AggregateSearchResponse struct {
	Results []AggregateSearchResult `json:"results"`
}

// AggregateStatsResponse — Collection statistics
type AggregateStatsResponse struct {
	TotalItems      int                 `json:"total_items"`
	BySource        map[string]int      `json:"by_source"`
	LastCollectedAt *int64              `json:"last_collected_at,omitempty"`
	KeywordsCount   int                 `json:"keywords_count"`
}

// KeywordsResponse — Keywords with counts
type KeywordsResponse struct {
	Keywords []KeywordInfo `json:"keywords"`
}

// KeywordInfo — One keyword
type KeywordInfo struct {
	Keyword string `json:"keyword"`
	Count   int    `json:"count"`
}

// CollectedItem — Raw collected item from data sources
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
