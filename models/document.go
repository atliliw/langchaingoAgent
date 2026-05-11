package models

// BM25SearchResult — BM25 retrieval result item
type BM25SearchResult struct {
	ID       string  `json:"id"`
	Content  string  `json:"content"`
	Score    float32 `json:"score"`
	ParentID string  `json:"parent_id"`
	IsMerged bool    `json:"is_merged"`
}

// DeleteDocumentResponse — POST /api/documents/:parent_id
type DeleteDocumentResponse struct {
	Success            bool   `json:"success"`
	ParentID           string `json:"parent_id"`
	BM25ChunksDeleted  bool   `json:"bm25_chunks_deleted"`
	VectorCountDeleted int    `json:"vector_count_deleted"`
	Message            string `json:"message"`
}

// DeleteDocumentRequest — Delete document request body
type DeleteDocumentRequest struct {
	Filename string `json:"filename"`
}

// DocumentInfo — Document listing info
type DocumentInfo struct {
	ID             string            `json:"id"`
	Title          string            `json:"title"`
	ContentPreview string            `json:"content_preview"`
	ChunkCount     int               `json:"chunk_count"`
	Metadata       map[string]string `json:"metadata"`
}

// BatchDeleteRequest — POST /api/documents/batch-delete
type BatchDeleteRequest struct {
	ParentIDs []string `json:"parent_ids"`
}

// BatchDeleteResponse — Batch delete response
type BatchDeleteResponse struct {
	Success      bool   `json:"success"`
	DeletedCount int    `json:"deleted_count"`
	FailedCount  int    `json:"failed_count"`
	Message      string `json:"message"`
}

// DocumentTagRequest — POST /api/documents/tags
type DocumentTagRequest struct {
	ParentID string   `json:"parent_id"`
	Tags     []string `json:"tags"`
}

// DocumentTagInfo — Document tag info
type DocumentTagInfo struct {
	ParentID string   `json:"parent_id"`
	Tags     []string `json:"tags"`
}
