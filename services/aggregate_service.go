package services

import (
	"github.com/atliliw/lanchaingo-agent/agents"
	"github.com/atliliw/lanchaingo-agent/stores"
)

type AggregateService struct {
	ContentStore *stores.ContentStore
	Config       ServiceConfig
}

type CollectRequest struct {
	Sources []string `json:"sources"`
	Force   bool     `json:"force"`
}

type CollectResponse struct {
	Success        bool            `json:"success"`
	CollectedCount int             `json:"collected_count"`
	Records        []CollectRecord `json:"records"`
}

type CollectRecord struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
	Status string `json:"status"`
}

type AggregateListResponse struct {
	Total int                      `json:"total"`
	Items []stores.AggregatedContent `json:"items"`
}

type AggregateSearchResponse struct {
	Results []stores.AggregateSearchResult `json:"results"`
}

type AggregateStatsResponse struct {
	TotalItems      int            `json:"total_items"`
	BySource        map[string]int `json:"by_source"`
	LastCollectedAt *int64         `json:"last_collected_at,omitempty"`
	KeywordsCount   int            `json:"keywords_count"`
}

func NewAggregateService(cfg ServiceConfig) (*AggregateService, error) {
	contentStore, err := stores.NewContentStore(cfg.SQLitePath)
	if err != nil {
		return nil, err
	}
	return &AggregateService{
		ContentStore: contentStore,
		Config:       cfg,
	}, nil
}

func (s *AggregateService) Collect(req CollectRequest) (*CollectResponse, error) {
	sources := req.Sources
	if len(sources) == 0 {
		sources = []string{"github", "hackernews", "rss", "arxiv"}
	}

	var records []CollectRecord
	var allItems []agents.CollectedItem

	for _, source := range sources {
		var items []agents.CollectedItem
		var err error

		switch source {
		case "github":
			tool := &agents.GitHubTool{}
			items, err = tool.FetchTrending([]string{"rust", "python"})
		case "hackernews":
			tool := &agents.HackerNewsTool{}
			items, err = tool.FetchTopStories([]string{"ai", "agent", "llm", "gpt", "langchain"})
		case "rss":
			tool := &agents.RSSTool{}
			feeds := []string{
				"https://openai.com/blog/rss.xml",
				"https://www.anthropic.com/index/rss.xml",
			}
			items, err = tool.FetchAllFeeds(feeds)
		case "arxiv":
			tool := &agents.ArXivTool{}
			items, err = tool.FetchPapers([]string{"cs.AI", "cs.CL"}, 10)
		}

		status := "success"
		if err != nil {
			status = err.Error()
		}

		records = append(records, CollectRecord{
			Source: source,
			Count:  len(items),
			Status: status,
		})
		allItems = append(allItems, items...)
	}

	for _, item := range allItems {
		summary := truncateStr(item.Content, 200)
		collectedItem := stores.CollectedItem{
			ID:          item.ID,
			Source:      item.Source,
			Title:       item.Title,
			Content:     item.Content,
			URL:         item.URL,
			Author:      item.Author,
			PublishedAt: item.PublishedAt,
			Metadata:    item.Metadata,
		}
		if err := s.ContentStore.SaveItemWithSummary(collectedItem, summary); err != nil {
			continue
		}
	}

	return &CollectResponse{
		Success:        true,
		CollectedCount: len(allItems),
		Records:        records,
	}, nil
}

func (s *AggregateService) List(source *string, limit, offset int) (*AggregateListResponse, error) {
	total, items, err := s.ContentStore.List(source, limit, offset)
	if err != nil {
		return nil, err
	}
	return &AggregateListResponse{
		Total: total,
		Items: items,
	}, nil
}

func (s *AggregateService) Search(query string, topK int) (*AggregateSearchResponse, error) {
	results, err := s.ContentStore.Search(query, topK)
	if err != nil {
		return nil, err
	}
	return &AggregateSearchResponse{Results: results}, nil
}

func (s *AggregateService) Stats() (*AggregateStatsResponse, error) {
	total, bySource, lastCollected, err := s.ContentStore.Stats()
	if err != nil {
		return nil, err
	}
	return &AggregateStatsResponse{
		TotalItems:      total,
		BySource:        bySource,
		LastCollectedAt: lastCollected,
		KeywordsCount:   0,
	}, nil
}

// truncateStr is defined in agent_executor.go
