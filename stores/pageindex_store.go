package stores

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type PageIndexStore struct {
	DB *sql.DB
}

type PageIndexDoc struct {
	DocID string
	Title string
}

type PageIndexNode struct {
	NodeID       string
	ParentNodeID *string
	Title        string
	Content      string
	Level        int
	Summary      string
}

type PageIndexSearchResult struct {
	DocID         string `json:"doc_id"`
	DocTitle      string `json:"doc_title"`
	Title         string `json:"title"`
	ContentPreview string `json:"content_preview"`
	Path          string `json:"path"`
	Level         int    `json:"level"`
	Summary       string `json:"summary"`
}

type PNode struct {
	ID       string
	Title    string
	Content  string
	Level    int
	Summary  string
	Children []*PNode
	Parent   *PNode
}

func NewPageIndexStore(dbPath string) (*PageIndexStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=rwc")
	if err != nil {
		return nil, fmt.Errorf("failed to open pageindex db: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pageindex_docs (
			doc_id TEXT PRIMARY KEY,
			title TEXT NOT NULL
		)
	`); err != nil {
		return nil, err
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pageindex_nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			doc_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			parent_node_id TEXT,
			title TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			level INTEGER NOT NULL DEFAULT 0,
			summary TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (doc_id) REFERENCES pageindex_docs(doc_id)
		)
	`); err != nil {
		return nil, err
	}

	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_pi_nodes_doc ON pageindex_nodes(doc_id)`); err != nil {
		return nil, err
	}

	// FTS5 for full-text search
	_, _ = db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS pageindex_fts USING fts5(
			doc_id, title, content, summary, content='pageindex_nodes', content_rowid='id'
		)
	`)

	return &PageIndexStore{DB: db}, nil
}

func (s *PageIndexStore) BuildTree(docID, title, text string) error {
	if _, err := s.DB.Exec("INSERT OR REPLACE INTO pageindex_docs (doc_id, title) VALUES (?, ?)", docID, title); err != nil {
		return err
	}

	root := ParseTree(docID, title, text)
	return s.saveNode(s.DB, docID, root, nil)
}

func (s *PageIndexStore) saveNode(db *sql.DB, docID string, node *PNode, parentNodeID *string) error {
	_, err := db.Exec(
		`INSERT INTO pageindex_nodes (doc_id, node_id, parent_node_id, title, content, level, summary)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		docID, node.ID, parentNodeID, node.Title, node.Content, node.Level, node.Summary,
	)
	if err != nil {
		return err
	}
	for _, child := range node.Children {
		if err := s.saveNode(db, docID, child, &node.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *PageIndexStore) Search(query string, topK int) ([]PageIndexSearchResult, error) {
	rows, err := s.DB.Query(
		`SELECT n.doc_id, d.title, n.title, n.content, n.level, n.summary
		 FROM pageindex_fts f
		 JOIN pageindex_nodes n ON f.doc_id = n.doc_id AND f.title = n.title
		 JOIN pageindex_docs d ON n.doc_id = d.doc_id
		 WHERE pageindex_fts MATCH ? LIMIT ?`,
		query, topK,
	)
	if err != nil {
		// Fallback to LIKE search if FTS fails
		return s.searchLike(query, topK)
	}
	defer rows.Close()

	results := make([]PageIndexSearchResult, 0)
	for rows.Next() {
		var r PageIndexSearchResult
		rows.Scan(&r.DocID, &r.DocTitle, &r.Title, &r.ContentPreview, &r.Level, &r.Summary)
		if len([]rune(r.ContentPreview)) > 200 {
			r.ContentPreview = string([]rune(r.ContentPreview)[:200])
		}
		results = append(results, r)
	}
	return results, nil
}

func (s *PageIndexStore) searchLike(query string, topK int) ([]PageIndexSearchResult, error) {
	likeQ := "%" + query + "%"
	rows, err := s.DB.Query(
		`SELECT n.doc_id, d.title, n.title, n.content, n.level, n.summary
		 FROM pageindex_nodes n
		 JOIN pageindex_docs d ON n.doc_id = d.doc_id
		 WHERE n.content LIKE ? OR n.title LIKE ?
		 LIMIT ?`,
		likeQ, likeQ, topK,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]PageIndexSearchResult, 0)
	for rows.Next() {
		var r PageIndexSearchResult
		rows.Scan(&r.DocID, &r.DocTitle, &r.Title, &r.ContentPreview, &r.Level, &r.Summary)
		if len([]rune(r.ContentPreview)) > 200 {
			r.ContentPreview = string([]rune(r.ContentPreview)[:200])
		}
		results = append(results, r)
	}
	return results, nil
}

func (s *PageIndexStore) ListDocs() ([]PageIndexDoc, error) {
	rows, err := s.DB.Query("SELECT doc_id, title FROM pageindex_docs ORDER BY doc_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs := make([]PageIndexDoc, 0)
	for rows.Next() {
		var doc PageIndexDoc
		rows.Scan(&doc.DocID, &doc.Title)
		docs = append(docs, doc)
	}
	return docs, nil
}

func (s *PageIndexStore) DeleteDoc(docID string) error {
	s.DB.Exec("DELETE FROM pageindex_nodes WHERE doc_id = ?", docID)
	_, err := s.DB.Exec("DELETE FROM pageindex_docs WHERE doc_id = ?", docID)
	return err
}

func ParseTree(docID, title, text string) *PNode {
	root := &PNode{
		ID:    docID + "_root",
		Title: title,
		Level: 0,
	}
	lines := strings.Split(text, "\n")
	var stack []*PNode
	current := root

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		level := countHeadingLevel(trimmed)
		if level > 0 {
			cleanTitle := strings.TrimLeft(trimmed, "# ")
			node := &PNode{
				ID:    fmt.Sprintf("%s_h%d", docID, level),
				Title: cleanTitle,
				Level: level,
			}
			for len(stack) >= level {
				stack = stack[:len(stack)-1]
				if len(stack) > 0 {
					current = stack[len(stack)-1]
				} else {
					current = root
				}
			}
			current.Children = append(current.Children, node)
			node.Parent = current
			current = node
			stack = append(stack, current)
		} else {
			if current != root {
				if current.Content != "" {
					current.Content += "\n"
				}
				current.Content += trimmed
			} else {
				leaf := &PNode{
					ID:    fmt.Sprintf("%s_p", docID),
					Title: trimmed,
					Level: 1,
				}
				root.Children = append(root.Children, leaf)
				leaf.Parent = root
			}
		}
	}
	return root
}

func countHeadingLevel(line string) int {
	count := 0
	for _, r := range line {
		if r == '#' {
			count++
		} else {
			break
		}
	}
	if count > 0 && len(line) > count && line[count] == ' ' {
		return count
	}
	return 0
}
