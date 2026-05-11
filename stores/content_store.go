package stores

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type ContentStore struct {
	db *sql.DB
}

type AggregatedContent struct {
	ID          string
	Source      string
	Title       string
	Content     string
	URL         string
	Author      *string
	PublishedAt *int64
	CollectedAt int64
	Summary     *string
	Keywords    []string
	Metadata    map[string]interface{}
}

type CollectedItem struct {
	ID          string
	Source      string
	Title       string
	Content     string
	URL         string
	Author      *string
	PublishedAt *int64
	Metadata    map[string]interface{}
}

func NewContentStore(dbPath string) (*ContentStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=rwc")
	if err != nil {
		return nil, fmt.Errorf("failed to open content db: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS aggregate_content (
			id TEXT PRIMARY KEY,
			source TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			url TEXT NOT NULL,
			author TEXT,
			published_at INTEGER,
			collected_at INTEGER NOT NULL,
			summary TEXT,
			keywords TEXT,
			metadata TEXT
		)
	`); err != nil {
		return nil, fmt.Errorf("failed to create content table: %w", err)
	}

	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_content_source ON aggregate_content(source)`); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_content_collected ON aggregate_content(collected_at)`); err != nil {
		return nil, err
	}

	return &ContentStore{db: db}, nil
}

func (s *ContentStore) SaveItemWithSummary(item CollectedItem, summary string) error {
	keywordsJSON, _ := json.Marshal([]string{})
	metadataJSON, _ := json.Marshal(item.Metadata)

	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO aggregate_content 
		(id, source, title, content, url, author, published_at, collected_at, summary, keywords, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Source, item.Title, item.Content, item.URL,
		item.Author, item.PublishedAt, time.Now().UnixMilli(),
		summary, string(keywordsJSON), string(metadataJSON),
	)
	return err
}

func (s *ContentStore) List(source *string, limit, offset int) (int, []AggregatedContent, error) {
	var total int
	var rows *sql.Rows
	var err error

	if source != nil {
		err = s.db.QueryRow("SELECT COUNT(*) FROM aggregate_content WHERE source = ?", *source).Scan(&total)
		if err != nil {
			return 0, nil, err
		}
		rows, err = s.db.Query(
			`SELECT id, source, title, content, url, author, published_at, collected_at, summary, keywords, metadata
			 FROM aggregate_content WHERE source = ? ORDER BY collected_at DESC LIMIT ? OFFSET ?`,
			*source, limit, offset,
		)
	} else {
		err = s.db.QueryRow("SELECT COUNT(*) FROM aggregate_content").Scan(&total)
		if err != nil {
			return 0, nil, err
		}
		rows, err = s.db.Query(
			`SELECT id, source, title, content, url, author, published_at, collected_at, summary, keywords, metadata
			 FROM aggregate_content ORDER BY collected_at DESC LIMIT ? OFFSET ?`,
			limit, offset,
		)
	}
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	items := make([]AggregatedContent, 0)
	for rows.Next() {
		var item AggregatedContent
		var author, summary, keywordsStr, metadataStr sql.NullString
		var publishedAt sql.NullInt64
		err := rows.Scan(
			&item.ID, &item.Source, &item.Title, &item.Content, &item.URL,
			&author, &publishedAt, &item.CollectedAt, &summary, &keywordsStr, &metadataStr,
		)
		if err != nil {
			continue
		}
		if author.Valid {
			item.Author = &author.String
		}
		if publishedAt.Valid {
			item.PublishedAt = &publishedAt.Int64
		}
		if summary.Valid {
			item.Summary = &summary.String
		}
		if keywordsStr.Valid {
			json.Unmarshal([]byte(keywordsStr.String), &item.Keywords)
		}
		if metadataStr.Valid {
			json.Unmarshal([]byte(metadataStr.String), &item.Metadata)
		}
		items = append(items, item)
	}
	return total, items, nil
}

func (s *ContentStore) Search(query string, topK int) ([]AggregateSearchResult, error) {
	_, allItems, err := s.List(nil, 100, 0)
	if err != nil {
		return nil, err
	}

	type scored struct {
		score float64
		item  AggregatedContent
	}
	var scoredItems []scored
	for _, item := range allItems {
		text := item.Title + " " + item.Content
		score := simpleSimilarity(query, text)
		scoredItems = append(scoredItems, scored{score: score, item: item})
	}

	for i := 0; i < len(scoredItems); i++ {
		for j := i + 1; j < len(scoredItems); j++ {
			if scoredItems[j].score > scoredItems[i].score {
				scoredItems[i], scoredItems[j] = scoredItems[j], scoredItems[i]
			}
		}
	}

	if len(scoredItems) > topK {
		scoredItems = scoredItems[:topK]
	}

	var results []AggregateSearchResult
	for _, s := range scoredItems {
		results = append(results, AggregateSearchResult{
			ID:      s.item.ID,
			Source:  s.item.Source,
			Title:   s.item.Title,
			Content: s.item.Content,
			URL:     s.item.URL,
			Score:   float32(s.score),
			Summary: s.item.Summary,
		})
	}
	return results, nil
}

func (s *ContentStore) Stats() (int, map[string]int, *int64, error) {
	var total int
	err := s.db.QueryRow("SELECT COUNT(*) FROM aggregate_content").Scan(&total)
	if err != nil {
		return 0, nil, nil, err
	}

	rows, err := s.db.Query("SELECT source, COUNT(*) FROM aggregate_content GROUP BY source")
	if err != nil {
		return 0, nil, nil, err
	}
	defer rows.Close()

	bySource := make(map[string]int)
	for rows.Next() {
		var source string
		var count int
		rows.Scan(&source, &count)
		bySource[source] = count
	}

	var last *int64
	var lastVal sql.NullInt64
	s.db.QueryRow("SELECT MAX(collected_at) FROM aggregate_content").Scan(&lastVal)
	if lastVal.Valid {
		last = &lastVal.Int64
	}

	return total, bySource, last, nil
}

type AggregateSearchResult struct {
	ID      string
	Source  string
	Title   string
	Content string
	URL     string
	Score   float32
	Summary *string
}

func simpleSimilarity(query, text string) float64 {
	qWords := strings.Fields(query)
	if len(qWords) == 0 {
		return 0
	}
	matches := 0
	tLower := strings.ToLower(text)
	for _, q := range qWords {
		if strings.Contains(tLower, strings.ToLower(q)) {
			matches++
		}
	}
	return float64(matches) / float64(len(qWords))
}

func (s *ContentStore) Close() error {
	return s.db.Close()
}
