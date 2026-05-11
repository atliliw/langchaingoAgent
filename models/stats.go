package models

// StatsResponse — GET /api/stats response
type StatsResponse struct {
	TotalDocuments       int    `json:"total_documents"`
	VectorSize           int    `json:"vector_size"`
	BM25Chunks           int    `json:"bm25_chunks"`
	BM25Persisted        bool   `json:"bm25_persisted"`
	CollectionName       string `json:"collection_name"`
	ConversationSessions int    `json:"conversation_sessions"`
}

// ApiStatsSummary — API call statistics summary
type ApiStatsSummary struct {
	TotalCalls      int64           `json:"total_calls"`
	TotalTokens     int64           `json:"total_tokens"`
	TotalDurationMs int64           `json:"total_duration_ms"`
	SuccessCount    int64           `json:"success_count"`
	CallsToday      int64           `json:"calls_today"`
	TokensToday     int64           `json:"tokens_today"`
	AvgDurationTodayMs int64        `json:"avg_duration_today_ms"`
	CallsThisWeek   int64           `json:"calls_this_week"`
	TokensThisWeek  int64           `json:"tokens_this_week"`
	APITypes        []ApiTypeStats  `json:"api_types"`
}

// ApiTypeStats — Per-API-type statistics
type ApiTypeStats struct {
	APIType     string `json:"api_type"`
	CallCount   int64  `json:"call_count"`
	TokensUsed  int64  `json:"tokens_used"`
	AvgDurationMs int64 `json:"avg_duration_ms"`
}

// RecentCall — Recent API call record
type RecentCall struct {
	ID          int64  `json:"id"`
	APIType     string `json:"api_type"`
	TokensUsed  int64  `json:"tokens_used"`
	DurationMs  int64  `json:"duration_ms"`
	Success     bool   `json:"success"`
	TimeCreated string `json:"time_created"`
}
