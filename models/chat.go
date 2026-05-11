package models

import "encoding/json"

// ChatRequest — POST /api/chat
type ChatRequest struct {
	SessionID    *string `json:"session_id"`
	Message      string  `json:"message"`
	UseVector    bool    `json:"use_vector,omitempty"`
	UseBM25      bool    `json:"use_bm25,omitempty"`
	TopK         int     `json:"top_k,omitempty"`
	CompressMode string  `json:"compress_mode,omitempty"`
}

// GetTopK returns the top_k value or default 3
func (r *ChatRequest) GetTopK() int {
	if r.TopK <= 0 {
		return 3
	}
	return r.TopK
}

// GetCompressMode returns the compress mode or default "none"
func (r *ChatRequest) GetCompressMode() string {
	if r.CompressMode == "" {
		return "none"
	}
	return r.CompressMode
}

// ChatResponse — LLM generated reply with sources
type ChatResponse struct {
	SessionID       string       `json:"session_id"`
	Reply           string       `json:"reply"`
	Sources         []SourceInfo `json:"sources"`
	Compressed      bool         `json:"compressed"`
	CompressionInfo *string      `json:"compression_info,omitempty"`
}

// SourceInfo — RAG retrieved source document
type SourceInfo struct {
	Content string  `json:"content"`
	Score   float32 `json:"score"`
	Source  string  `json:"source"`
}

// SessionInfo — One session in the session list
type SessionInfo struct {
	SessionID    string `json:"session_id"`
	Title        string `json:"title"`
	CreatedAt    string `json:"created_at"`
	MessageCount int    `json:"message_count"`
	Preview      string `json:"preview"`
}

// ConversationMessage — One message in a conversation
type ConversationMessage struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id"`
	Role        string `json:"role"` // user / assistant / summary
	Content     string `json:"content"`
	Tokens      int64  `json:"tokens"`
	TimeCreated int64  `json:"time_created"` // millisecond timestamp
}

// Session — Database session record
type Session struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	MessageCount  int64  `json:"message_count"`
	TokensUsed    int64  `json:"tokens_used"`
	TimeCreated   int64  `json:"time_created"`
	TimeUpdated   int64  `json:"time_updated"`
}

// CompressModeInfo — Compression mode info for frontend display
type CompressModeInfo struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// CompressMode — Conversation compression mode
type CompressMode int

const (
	CompressNone           CompressMode = iota
	CompressSlidingWindow
	CompressTokenLimit
	CompressSummary
	CompressLayered
	CompressAdaptiveFocus
	CompressTopicSegment
)

func ParseCompressMode(s string) CompressMode {
	switch s {
	case "none":
		return CompressNone
	case "sliding_window":
		return CompressSlidingWindow
	case "token_limit":
		return CompressTokenLimit
	case "summary":
		return CompressSummary
	case "layered":
		return CompressLayered
	case "afm", "adaptive_focus":
		return CompressAdaptiveFocus
	case "topic", "topic_segment", "episodic":
		return CompressTopicSegment
	default:
		return CompressNone
	}
}

// SearchMode — Knowledge retrieval mode
type SearchMode int

const (
	SearchNone   SearchMode = iota
	SearchVector
	SearchBM25
	SearchHybrid
)

func SearchModeFromFlags(useVector, useBM25 bool) SearchMode {
	switch {
	case !useVector && !useBM25:
		return SearchNone
	case useVector && !useBM25:
		return SearchVector
	case !useVector && useBM25:
		return SearchBM25
	default:
		return SearchHybrid
	}
}

// EditMessageRequest — PUT /api/chat/message/:message_id
type EditMessageRequest struct {
	Content string `json:"content"`
}

// RegenerateResponse — POST /api/chat/message/:message_id/regenerate
type RegenerateResponse struct {
	MessageID string `json:"message_id"`
	Reply     string `json:"reply"`
}

// SessionExport — GET /api/chat/session/:session_id/export
type SessionExport struct {
	SessionID string                `json:"session_id"`
	Title     string                `json:"title"`
	CreatedAt string                `json:"created_at"`
	Messages  []ConversationMessage `json:"messages"`
}

// ImportMessage — Single message in import format
type ImportMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SessionImport — POST /api/chat/session/import
type SessionImport struct {
	Title    *string         `json:"title,omitempty"`
	Messages []ImportMessage `json:"messages"`
}

// SessionSearchRequest — POST /api/chat/sessions/search
type SessionSearchRequest struct {
	Query string `json:"query"`
}

// BranchRequest — POST /api/chat/session/branch
type BranchRequest struct {
	SessionID      string `json:"session_id"`
	FromMessageID  string `json:"from_message_id"`
}

// BranchResponse — Branch session response
type BranchResponse struct {
	NewSessionID string `json:"new_session_id"`
	Title        string `json:"title"`
	MessageCount int    `json:"message_count"`
}

// SetContextRequest — PUT /api/chat/context/:session_id
type SetContextRequest struct {
	Context string `json:"context"`
}

// GetCompressModes returns all available compression modes info
func GetCompressModes() []CompressModeInfo {
	return []CompressModeInfo{
		{Name: "none", Label: "不压缩", Description: "保留完整历史（可能超出token限制）"},
		{Name: "sliding_window", Label: "滑动窗口", Description: "只保留最近N条消息"},
		{Name: "token_limit", Label: "Token限制", Description: "控制总token数量"},
		{Name: "summary", Label: "摘要压缩", Description: "旧消息压缩为摘要"},
		{Name: "layered", Label: "分层压缩", Description: "保护重要+摘要+最近"},
		{Name: "afm", Label: "AFM自适应", Description: "LLM分类+F量完整保留+C精简+P占位"},
		{Name: "topic", Label: "话题分段", Description: "LLM检测话题边界，每段独立摘要"},
	}
}

// Helper to marshal/unmarshal JSON responses
type JSONResponse map[string]interface{}

func JSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
