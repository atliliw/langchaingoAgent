package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/atliliw/lanchaingo-agent/stores"
	"github.com/atliliw/lanchaingo-agent/utils"
	"github.com/google/uuid"
)

type ApiService struct {
	VectorStore       *stores.QdrantStore
	BM25Store         *stores.BM25Store
	HybridStore       *stores.HybridStore
	ConversationStore *stores.ConversationStore
	PageIndexStore    *stores.PageIndexStore
	Config            ServiceConfig
	mu                sync.RWMutex
}

type ServiceConfig struct {
	ServerAddr    string
	QdrantURL     string
	QdrantColl    string
	QdrantSize    int
	QdrantDist    string
	MongoURI      string
	MongoDB       string
	MongoParent   string
	MongoChunk    string
	SQLitePath    string
	ChunkSize     int
	ChunkOverlap  int
	MinScore      float64
	DefaultTopK   int
	UploadDir     string
	OpenAIAPIKey  string
	OpenAIBaseURL string
	ChatModel     string
	EmbedModel    string
}

type UploadResponse struct {
	Success       bool     `json:"success"`
	DocumentCount int      `json:"document_count"`
	ChunkCount    int      `json:"chunk_count"`
	Message       string   `json:"message"`
	DocumentIDs   []string `json:"document_ids"`
	ChunkStrategy string   `json:"chunk_strategy"`
}

type SearchRequest struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
}

type SearchResponse struct {
	Query       string              `json:"query"`
	Mode        string              `json:"mode"`
	Results     []SearchResultItem  `json:"results"`
	TotalCount  int                 `json:"total_count"`
}

type SearchResultItem struct {
	ID       *string     `json:"id,omitempty"`
	Content  string      `json:"content"`
	Score    float64     `json:"score"`
	Source   *string     `json:"source,omitempty"`
	Metadata interface{} `json:"metadata"`
}

type CompareResponse struct {
	Query         string             `json:"query"`
	VectorResults []SearchResultItem  `json:"vector_results"`
	BM25Results   []SearchResultItem  `json:"bm25_results"`
	HybridResults []SearchResultItem  `json:"hybrid_results"`
	Comparison    SearchComparison    `json:"comparison"`
}

type SearchComparison struct {
	VectorTop1Score float64 `json:"vector_top1_score"`
	BM25Top1Score   float64 `json:"bm25_top1_score"`
	HybridTop1Score float64 `json:"hybrid_top1_score"`
	OverlapCount    int     `json:"overlap_count"`
	UniqueVector    int     `json:"unique_vector"`
	UniqueBM25      int     `json:"unique_bm25"`
	UniqueHybrid    int     `json:"unique_hybrid"`
}

type StatsResponse struct {
	TotalDocuments       int    `json:"total_documents"`
	VectorSize           int    `json:"vector_size"`
	BM25Chunks           int    `json:"bm25_chunks"`
	BM25Persisted        bool   `json:"bm25_persisted"`
	CollectionName       string `json:"collection_name"`
	ConversationSessions int    `json:"conversation_sessions"`
}

func NewApiService(cfg ServiceConfig) (*ApiService, error) {
	qdrantCfg := stores.QdrantStoreConfig{
		URL:            cfg.QdrantURL,
		CollectionName: cfg.QdrantColl,
		VectorSize:     cfg.QdrantSize,
		Distance:       cfg.QdrantDist,
		MinScore:       cfg.MinScore,
	}
	vectorStore, err := stores.NewQdrantStore(qdrantCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to init QdrantStore: %w", err)
	}

	bm25Store, err := stores.NewBM25Store(cfg.MongoURI, cfg.MongoDB, cfg.MongoParent, cfg.MongoChunk)
	if err != nil {
		return nil, fmt.Errorf("failed to init BM25Store: %w", err)
	}

	hybridStore := stores.NewHybridStore(bm25Store, vectorStore, cfg.DefaultTopK)

	convConfig := stores.DefaultConversationConfig()
	llmClient := &stores.LLMClient{
		APIKey:  cfg.OpenAIAPIKey,
		BaseURL: cfg.OpenAIBaseURL,
		Model:   cfg.ChatModel,
	}
	conversationStore, err := stores.NewConversationStore(cfg.SQLitePath, llmClient, convConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to init ConversationStore: %w", err)
	}

	pageIndexStore, err := stores.NewPageIndexStore(cfg.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("failed to init PageIndexStore: %w", err)
	}

	return &ApiService{
		VectorStore:       vectorStore,
		BM25Store:         bm25Store,
		HybridStore:       hybridStore,
		ConversationStore: conversationStore,
		PageIndexStore:    pageIndexStore,
		Config:            cfg,
	}, nil
}

func (s *ApiService) UploadFile(filePath string, originalName string) (*UploadResponse, error) {
	return s.UploadFileWithStrategy(filePath, originalName, "recursive")
}

func (s *ApiService) UploadFileWithStrategy(filePath string, originalName string, strategy string) (*UploadResponse, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	content := string(data)

	processor := utils.NewDocumentProcessor(s.Config.ChunkSize, s.Config.ChunkOverlap)
	originalDocs, chunks, _ := processor.ProcessFile(content, originalName)

	chunkDocs := make([]stores.Document, len(chunks))
	for i, chunk := range chunks {
		chunkDocs[i] = stores.Document{
			ID:      fmt.Sprintf("%s_%d", uuid.New().String(), i),
			Content: chunk,
			Metadata: map[string]string{
				"original_filename": originalName,
				"upload_time":       time.Now().Format(time.RFC3339),
			},
		}
	}

	vectorIDs, err := s.VectorStore.AddDocuments(chunkDocs)
	if err != nil {
		return nil, fmt.Errorf("failed to add to vector store: %w", err)
	}

	parentDocs := make([]stores.Document, len(originalDocs))
	for i, doc := range originalDocs {
		parentDocs[i] = stores.Document{
			ID:      fmt.Sprintf("parent_%s_%d", uuid.New().String(), i),
			Content: doc.Content,
			Metadata: map[string]string{
				"original_filename": originalName,
				"upload_time":       time.Now().Format(time.RFC3339),
				"chunk_count":       fmt.Sprintf("%d", len(chunks)),
			},
		}
	}
	if err := s.BM25Store.AddDocuments(parentDocs); err != nil {
		return nil, fmt.Errorf("failed to add to BM25 store: %w", err)
	}

	return &UploadResponse{
		Success:       true,
		DocumentCount: len(originalDocs),
		ChunkCount:    len(vectorIDs),
		Message:       fmt.Sprintf("成功上传 %d 个原始文档，分割为 %d 个chunks", len(originalDocs), len(vectorIDs)),
		DocumentIDs:   vectorIDs,
		ChunkStrategy: strategy,
	}, nil
}

func (s *ApiService) SearchVector(req SearchRequest) (*SearchResponse, error) {
	results, err := s.VectorStore.Search(req.Query, req.TopK)
	if err != nil {
		return nil, err
	}
	var items []SearchResultItem
	for _, r := range results {
		id := r.ID
		source := "vector"
		items = append(items, SearchResultItem{
			ID:      &id,
			Content: r.Document.Content,
			Score:   r.Score,
			Source:  &source,
		})
	}
	return &SearchResponse{
		Query:      req.Query,
		Mode:       "vector",
		Results:    items,
		TotalCount: len(items),
	}, nil
}

func (s *ApiService) SearchBM25(req SearchRequest) (*SearchResponse, error) {
	results, err := s.BM25Store.Search(req.Query, req.TopK)
	if err != nil {
		return nil, err
	}
	var items []SearchResultItem
	for _, r := range results {
		source := "bm25"
		items = append(items, SearchResultItem{
			ID:      &r.ID,
			Content: r.Content,
			Score:   r.Score,
			Source:  &source,
		})
	}
	return &SearchResponse{
		Query:      req.Query,
		Mode:       "bm25",
		Results:    items,
		TotalCount: len(items),
	}, nil
}

func (s *ApiService) SearchHybrid(req SearchRequest) (*SearchResponse, error) {
	results, err := s.HybridStore.Search(req.Query, req.TopK)
	if err != nil {
		return nil, err
	}
	var items []SearchResultItem
	for _, r := range results {
		source := r.Source
		items = append(items, SearchResultItem{
			ID:      &r.ID,
			Content: r.Content,
			Score:   float64(r.RRFScore),
			Source:  &source,
		})
	}
	return &SearchResponse{
		Query:      req.Query,
		Mode:       "hybrid",
		Results:    items,
		TotalCount: len(items),
	}, nil
}

func (s *ApiService) CompareSearch(query string, topK int) (*CompareResponse, error) {
	vectorResults, err := s.VectorStore.Search(query, topK)
	if err != nil {
		return nil, err
	}
	bm25Results, err := s.BM25Store.Search(query, topK)
	if err != nil {
		return nil, err
	}
	hybridResults, err := s.HybridStore.Search(query, topK)
	if err != nil {
		return nil, err
	}

	vectorItems := make([]SearchResultItem, len(vectorResults))
	for i, r := range vectorResults {
		src := "vector"
		vectorItems[i] = SearchResultItem{
			ID: &r.ID, Content: r.Document.Content, Score: r.Score, Source: &src,
		}
	}

	bm25Items := make([]SearchResultItem, len(bm25Results))
	for i, r := range bm25Results {
		src := "bm25"
		bm25Items[i] = SearchResultItem{
			ID: &r.ID, Content: r.Content, Score: r.Score, Source: &src,
		}
	}

	hybridItems := make([]SearchResultItem, len(hybridResults))
	for i, r := range hybridResults {
		src := r.Source
		hybridItems[i] = SearchResultItem{
			ID: &r.ID, Content: r.Content, Score: float64(r.RRFScore), Source: &src,
		}
	}

	vectorTop1 := 0.0
	bm25Top1 := 0.0
	hybridTop1 := 0.0
	if len(vectorItems) > 0 {
		vectorTop1 = vectorItems[0].Score
	}
	if len(bm25Items) > 0 {
		bm25Top1 = bm25Items[0].Score
	}
	if len(hybridItems) > 0 {
		hybridTop1 = hybridItems[0].Score
	}

	return &CompareResponse{
		Query:         query,
		VectorResults: vectorItems,
		BM25Results:   bm25Items,
		HybridResults: hybridItems,
		Comparison: SearchComparison{
			VectorTop1Score: vectorTop1,
			BM25Top1Score:   bm25Top1,
			HybridTop1Score: hybridTop1,
			OverlapCount:    0,
			UniqueVector:    len(vectorItems),
			UniqueBM25:      len(bm25Items),
			UniqueHybrid:    len(hybridItems),
		},
	}, nil
}

func (s *ApiService) GetStats() (*StatsResponse, error) {
	sessions, err := s.ConversationStore.GetSessions()
	if err != nil {
		sessions = nil
	}
	sessionCount := 0
	if sessions != nil {
		sessionCount = len(sessions)
	}
	return &StatsResponse{
		TotalDocuments:       s.VectorStore.Count(),
		VectorSize:           s.VectorStore.VectorSize(),
		BM25Chunks:           s.BM25Store.Count(),
		BM25Persisted:        s.BM25Store.IsMongo(),
		CollectionName:       s.Config.QdrantColl,
		ConversationSessions: sessionCount,
	}, nil
}

func (s *ApiService) ClearAll() error {
	if err := s.VectorStore.Clear(); err != nil {
		return err
	}
	if err := s.BM25Store.Clear(); err != nil {
		return err
	}
	return s.ConversationStore.ClearAll()
}

func (s *ApiService) SearchKnowledgeBase(query string, topK int) string {
	results, err := s.VectorStore.SearchRAG(query, topK)
	if err != nil || len(results) == 0 {
		return ""
	}
	var filtered []stores.SearchResult
	for _, r := range results {
		if r.Score >= 0.3 {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	var parts []string
	for _, r := range filtered {
		parts = append(parts, fmt.Sprintf("[相关性 %.1f%%]\n%s", r.Score*100, r.Document.Content))
	}
	return stringsJoin(parts, "\n\n---\n\n")
}

func (s *ApiService) ListDocuments() ([]stores.DocumentInfo, error) {
	return s.BM25Store.ListDocuments()
}

func (s *ApiService) DeleteDocument(parentID, filename string) error {
	if err := s.BM25Store.DeleteDocument(parentID); err != nil {
		return err
	}
	_, err := s.VectorStore.DeleteByMetadata("original_filename", filename)
	return err
}

func (s *ApiService) BatchDeleteDocuments(parentIDs []string) (int, int, error) {
	deleted := 0
	failed := 0
	for _, pid := range parentIDs {
		info, err := s.BM25Store.GetDocumentInfo(pid)
		if err != nil || info == nil {
			failed++
			continue
		}
		if err := s.BM25Store.DeleteDocument(pid); err != nil {
			failed++
			continue
		}
		s.VectorStore.DeleteByMetadata("original_filename", info.Filename)
		deleted++
	}
	return deleted, failed, nil
}

func (s *ApiService) AddDocumentTags(parentID string, tags []string) error {
	return s.BM25Store.AddDocumentTags(parentID, tags)
}

func (s *ApiService) GetDocumentsByTag(tag string) ([]stores.DocumentInfo, error) {
	return s.BM25Store.GetDocumentsByTag(tag)
}

func (s *ApiService) ListPageIndexDocs() ([]stores.PageIndexDoc, error) {
	return s.PageIndexStore.ListDocs()
}

func (s *ApiService) DeletePageIndexDoc(docID string) error {
	return s.PageIndexStore.DeleteDoc(docID)
}

func (s *ApiService) SearchPageIndex(query string, topK int) ([]stores.PageIndexSearchResult, error) {
	return s.PageIndexStore.Search(query, topK)
}

func (s *ApiService) RecordAPICall(apiType string, tokens, durationMs int64, success bool) error {
	return s.ConversationStore.RecordAPICall(apiType, tokens, durationMs, success)
}

func (s *ApiService) GetConversationHistory(sessionID string) ([]stores.ConversationMessage, error) {
	return s.ConversationStore.GetHistory(sessionID)
}

func (s *ApiService) GetSessions() ([]stores.SessionInfo, error) {
	return s.ConversationStore.GetSessions()
}

func (s *ApiService) ClearSession(sessionID string) error {
	return s.ConversationStore.ClearSession(sessionID)
}

func (s *ApiService) EditMessage(messageID, content string) error {
	return s.ConversationStore.EditMessage(messageID, content)
}

func (s *ApiService) DeleteMessage(messageID string) error {
	return s.ConversationStore.DeleteMessage(messageID)
}

func (s *ApiService) RegenerateMessage(messageID string) (*stores.ConversationMessage, error) {
	_, _, reply, err := s.ConversationStore.RegenerateMessage(messageID)
	if err != nil {
		return nil, err
	}
	return &stores.ConversationMessage{Content: reply}, nil
}

func (s *ApiService) ExportSession(sessionID string) (*stores.SessionExport, error) {
	info, err := s.ConversationStore.GetSessionInfo(sessionID)
	if err != nil {
		return nil, err
	}
	messages, err := s.ConversationStore.GetHistory(sessionID)
	if err != nil {
		return nil, err
	}
	return &stores.SessionExport{
		SessionID: sessionID,
		Title:     info.Title,
		CreatedAt: info.CreatedAt,
		Messages:  messages,
	}, nil
}

func (s *ApiService) ImportSession(title string, messages []stores.ImportMessage) (string, error) {
	return s.ConversationStore.ImportSession(stores.SessionImport{
		Title:    title,
		Messages: messages,
	})
}

func (s *ApiService) SearchSessions(query string) ([]stores.SessionInfo, error) {
	return s.ConversationStore.SearchSessions(query)
}

func (s *ApiService) BranchSession(sessionID, fromMessageID string) (string, string, int, error) {
	return s.ConversationStore.BranchSession(sessionID, fromMessageID)
}

func (s *ApiService) GetAPIStats() (*stores.ApiStatsSummary, error) {
	return s.ConversationStore.GetAPIStats()
}

func (s *ApiService) EnsureUploadDir() error {
	dir := filepath.Dir(s.Config.UploadDir + string(filepath.Separator))
	if dir == "." {
		dir = s.Config.UploadDir
	}
	return os.MkdirAll(dir, 0755)
}

func stringsJoin(elems []string, sep string) string {
	result := ""
	for i, e := range elems {
		if i > 0 {
			result += sep
		}
		result += e
	}
	return result
}
