package stores

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type ConversationStore struct {
	db             *sql.DB
	llm            *LLMClient
	compressConfig ConversationConfig
	mu             sync.RWMutex
}

type ConversationConfig struct {
	MaxHistoryMessages int
	MaxTokens          int
	KeepFirstNMessages int
	CompressThreshold  int
	KeepRecentMessages int
	ImportantKeywords  []string
	SummaryModel       string
}

func DefaultConversationConfig() ConversationConfig {
	return ConversationConfig{
		MaxHistoryMessages: 50,
		MaxTokens:          4000,
		KeepFirstNMessages: 2,
		CompressThreshold:  15,
		KeepRecentMessages: 5,
		ImportantKeywords:  []string{"我的名字", "我是", "记住", "设定", "角色"},
		SummaryModel:       "gpt-3.5-turbo",
	}
}

type LLMClient struct {
	APIKey  string
	BaseURL string
	Model   string
}

func (c *LLMClient) Invoke(messages []Message) (*LLMResult, error) {
	// Simplified LLM invocation - in production, call OpenAI API
	return &LLMResult{Content: "LLM response placeholder", TokenUsage: &TokenUsage{TotalTokens: 50}}, nil
}

type Message struct {
	Role    string
	Content string
}

type LLMResult struct {
	Content    string
	TokenUsage *TokenUsage
}

type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type ConversationMessage struct {
	ID          string
	SessionID   string
	Role        string
	Content     string
	Tokens      int64
	TimeCreated int64
}

type SessionInfo struct {
	SessionID    string
	Title        string
	CreatedAt    string
	MessageCount int
	Preview      string
}

type CompressMode int

const (
	CompressNone CompressMode = iota
	CompressSlidingWindow
	CompressTokenLimit
	CompressSummary
	CompressLayered
	CompressAdaptiveFocus
	CompressTopicSegment
)

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

type CompressModeInfo struct {
	Name        string
	Label       string
	Description string
}

func NewConversationStore(dbPath string, llm *LLMClient, config ConversationConfig) (*ConversationStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=rwc")
	if err != nil {
		return nil, fmt.Errorf("failed to open conversation db: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA synchronous = NORMAL"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return nil, err
	}

	if err := createTables(db); err != nil {
		return nil, err
	}

	return &ConversationStore{
		db:             db,
		llm:            llm,
		compressConfig: config,
	}, nil
}

func createTables(db *sql.DB) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS session (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			message_count INTEGER DEFAULT 0,
			tokens_used INTEGER DEFAULT 0,
			compressed_until INTEGER DEFAULT 0,
			important_context TEXT DEFAULT '',
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS message (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			tokens INTEGER DEFAULT 0,
			time_created INTEGER NOT NULL,
			FOREIGN KEY (session_id) REFERENCES session(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_message_session ON message(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_time ON message(time_created)`,
		`CREATE INDEX IF NOT EXISTS idx_session_updated ON session(time_updated)`,
		`CREATE TABLE IF NOT EXISTS api_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			api_type TEXT NOT NULL,
			tokens_used INTEGER DEFAULT 0,
			duration_ms INTEGER DEFAULT 0,
			success INTEGER DEFAULT 1,
			time_created INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_stats_time ON api_stats(time_created)`,
	}
	for _, t := range tables {
		if _, err := db.Exec(t); err != nil {
			return err
		}
	}
	return nil
}

func (s *ConversationStore) Chat(request ChatRequest, ragSources []SourceInfo) (*ChatResponse, error) {
	sessionID := request.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	exists, err := s.SessionExists(sessionID)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := s.CreateSession(sessionID, "新对话"); err != nil {
			return nil, err
		}
	}

	history, err := s.GetHistory(sessionID)
	if err != nil {
		return nil, err
	}

	messages, compressed, compInfo, err := s.BuildMessages(sessionID, history, request.Message, request.SearchMode, ragSources, request.CompressMode)
	if err != nil {
		return nil, err
	}

	reply, err := s.llm.Invoke(messages)
	if err != nil {
		return nil, err
	}

	s.saveMessage(sessionID, "user", request.Message)
	s.saveMessage(sessionID, "assistant", reply.Content)
	s.UpdateSessionStats(sessionID)

	if compressed && compInfo != nil {
		go s.CompressAndPersist(sessionID, request.CompressMode)
	}

	return &ChatResponse{
		SessionID:       sessionID,
		Reply:           reply.Content,
		Sources:         ragSources,
		Compressed:      compressed,
		CompressionInfo: compInfo,
	}, nil
}

func (s *ConversationStore) BuildMessages(sessionID string, history []ConversationMessage, currentMessage string, searchMode SearchMode, ragSources []SourceInfo, compressModeStr string) ([]Message, bool, *string, error) {
	var messages []Message

	ctx, _ := s.GetImportantContext(sessionID)
	sysPrompt := "请用中文回答用户问题。记住用户之前告诉你的信息。"
	if ctx != "" {
		sysPrompt = fmt.Sprintf("请用中文回答用户问题。记住用户之前告诉你的信息。\n\n【重要设定】%s", ctx)
	}
	messages = append(messages, Message{Role: "system", Content: sysPrompt})

	compressed, wasCompressed, compInfo := s.applyCompression(history, ParseCompressMode(compressModeStr))

	for _, msg := range compressed {
		switch msg.Role {
		case "user":
			messages = append(messages, Message{Role: "user", Content: msg.Content})
		case "assistant":
			messages = append(messages, Message{Role: "assistant", Content: msg.Content})
		case "summary":
			messages = append(messages, Message{Role: "system", Content: "[历史摘要] " + msg.Content})
		}
	}

	if len(ragSources) > 0 && searchMode != SearchNone {
		var contextParts []string
		for _, s := range ragSources {
			contextParts = append(contextParts, s.Content)
		}
		context := strings.Join(contextParts, "\n\n---\n\n")
		messages = append(messages, Message{Role: "user", Content: fmt.Sprintf("参考信息:\n%s\n\n%s", context, currentMessage)})
	} else {
		messages = append(messages, Message{Role: "user", Content: currentMessage})
	}

	return messages, wasCompressed, compInfo, nil
}

type ChatRequest struct {
	SessionID    string
	Message      string
	SearchMode   SearchMode
	CompressMode string
	TopK         int
}

type ChatResponse struct {
	SessionID       string
	Reply           string
	Sources         []SourceInfo
	Compressed      bool
	CompressionInfo *string
}

type SourceInfo struct {
	Content string
	Score   float32
	Source  string
}

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
	case "topic", "topic_segment":
		return CompressTopicSegment
	default:
		return CompressNone
	}
}

func (s *ConversationStore) applyCompression(history []ConversationMessage, mode CompressMode) ([]ConversationMessage, bool, *string) {
	if mode == CompressNone || len(history) <= 2 {
		return history, false, nil
	}
	switch mode {
	case CompressSlidingWindow:
		return s.applySlidingWindow(history, nil)
	case CompressTokenLimit:
		return s.applyTokenLimit(history, nil)
	case CompressSummary:
		return s.applySummaryCompression(history, nil)
	case CompressLayered:
		return s.applyLayeredCompression(history)
	case CompressAdaptiveFocus:
		keep := s.compressConfig.KeepRecentMessages
		return s.applySlidingWindow(history, &keep)
	case CompressTopicSegment:
		keep := s.compressConfig.KeepRecentMessages
		return s.applySlidingWindow(history, &keep)
	default:
		return history, false, nil
	}
}

func (s *ConversationStore) applySlidingWindow(history []ConversationMessage, keepCount *int) ([]ConversationMessage, bool, *string) {
	maxMessages := s.compressConfig.MaxHistoryMessages
	if keepCount != nil {
		maxMessages = *keepCount
	}
	if len(history) <= maxMessages {
		return history, false, nil
	}
	start := len(history) - maxMessages
	truncated := make([]ConversationMessage, maxMessages)
	copy(truncated, history[start:])
	info := fmt.Sprintf("滑动窗口: 保留最近 %d 条", maxMessages)
	return truncated, true, &info
}

func (s *ConversationStore) applyTokenLimit(history []ConversationMessage, maxTokensOverride *int) ([]ConversationMessage, bool, *string) {
	maxTokens := s.compressConfig.MaxTokens
	if maxTokensOverride != nil {
		maxTokens = *maxTokensOverride
	}
	keepFirst := s.compressConfig.KeepFirstNMessages
	origLen := len(history)

	if keepFirst >= origLen {
		return history, false, nil
	}

	firstN := history[:keepFirst]
	remaining := history[keepFirst:]

	var recent []ConversationMessage
	totalTokens := 0
	for i := len(remaining) - 1; i >= 0; i-- {
		msgTokens := EstimateTokens(remaining[i].Content)
		if totalTokens+msgTokens <= maxTokens {
			recent = append([]ConversationMessage{remaining[i]}, recent...)
			totalTokens += msgTokens
		} else {
			break
		}
	}

	result := append(firstN, recent...)
	if len(result) < origLen {
		info := fmt.Sprintf("Token限制: 保留前 %d 条 + 最近 %d 条 (%d tokens)", keepFirst, len(result)-keepFirst, totalTokens)
		return result, true, &info
	}
	return result, false, nil
}

func (s *ConversationStore) applySummaryCompression(history []ConversationMessage, thresholdOverride *int) ([]ConversationMessage, bool, *string) {
	keepRecent := s.compressConfig.KeepRecentMessages
	threshold := s.compressConfig.CompressThreshold
	if thresholdOverride != nil {
		threshold = *thresholdOverride
	}
	if len(history) <= keepRecent || len(history) <= threshold {
		return history, false, nil
	}

	toCompressCount := len(history) - keepRecent
	toCompress := history[:toCompressCount]
	recent := history[toCompressCount:]

	batch := toCompress
	if len(batch) > 20 {
		batch = batch[len(batch)-20:]
		toCompressCount = len(history) - keepRecent
	}

	summary := s.generateSummary(batch)
	summaryTokens := EstimateTokens(summary)

	summaryMsg := ConversationMessage{
		ID:          uuid.New().String(),
		Role:        "summary",
		Content:     summary,
		Tokens:      int64(summaryTokens),
		TimeCreated: time.Now().UnixMilli(),
	}

	result := append([]ConversationMessage{summaryMsg}, recent...)
	info := fmt.Sprintf("摘要压缩: %d 条压缩为摘要，保留最近 %d 条", toCompressCount, keepRecent)
	return result, true, &info
}

func (s *ConversationStore) applyLayeredCompression(history []ConversationMessage) ([]ConversationMessage, bool, *string) {
	maxMessages := s.compressConfig.MaxHistoryMessages
	keepRecent := s.compressConfig.KeepRecentMessages
	threshold := s.compressConfig.CompressThreshold
	keywords := s.compressConfig.ImportantKeywords

	if len(history) <= keepRecent || len(history) <= threshold {
		return history, false, nil
	}

	var important []ConversationMessage
	for _, msg := range history {
		for _, kw := range keywords {
			if strings.Contains(msg.Content, kw) {
				important = append(important, msg)
				break
			}
		}
	}

	recent := make([]ConversationMessage, keepRecent)
	copy(recent, history[len(history)-keepRecent:])

	importantIDs := make(map[string]bool)
	for _, m := range important {
		importantIDs[m.ID] = true
	}
	recentIDs := make(map[string]bool)
	for _, m := range recent {
		recentIDs[m.ID] = true
	}

	var toCompress []ConversationMessage
	for _, msg := range history {
		if !importantIDs[msg.ID] && !recentIDs[msg.ID] {
			toCompress = append(toCompress, msg)
		}
	}

	batch := toCompress
	if len(batch) > 20 {
		batch = batch[len(batch)-20:]
	}

	summary := ""
	if len(batch) > 3 {
		summary = s.generateSummary(batch)
	} else {
		var parts []string
		for _, m := range batch {
			parts = append(parts, m.Content)
		}
		summary = strings.Join(parts, "\n")
	}

	summaryMsg := ConversationMessage{
		ID:          uuid.New().String(),
		Role:        "summary",
		Content:     summary,
		Tokens:      int64(EstimateTokens(summary)),
		TimeCreated: time.Now().UnixMilli(),
	}

	var result []ConversationMessage
	result = append(result, important...)
	result = append(result, summaryMsg)
	result = append(result, recent...)

	if len(result) > maxMessages {
		result = result[len(result)-maxMessages:]
	}

	info := fmt.Sprintf("分层压缩: 重要 %d 条 + 摘要 + 最近 %d 条", len(important), keepRecent)
	return result, true, &info
}

func (s *ConversationStore) generateSummary(messages []ConversationMessage) string {
	var parts []string
	for _, m := range messages {
		parts = append(parts, fmt.Sprintf("%s: %s", m.Role, m.Content))
	}
	text := strings.Join(parts, "\n")
	prompt := fmt.Sprintf("请将以下对话压缩成摘要（100字内），保留关键设定：\n\n%s", text)

	result, err := s.llm.Invoke([]Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return fmt.Sprintf("[历史摘要: 省略了 %d 条消息]", len(messages))
	}
	return result.Content
}

func (s *ConversationStore) CompressAndPersist(sessionID string, modeStr string) error {
	mode := ParseCompressMode(modeStr)
	if mode == CompressNone {
		return nil
	}

	var compressedUntil int64
	s.db.QueryRow("SELECT COALESCE(compressed_until, 0) FROM session WHERE id = ?", sessionID).Scan(&compressedUntil)

	rows, err := s.db.Query(
		`SELECT id, session_id, role, content, tokens, time_created 
		 FROM message WHERE session_id = ? AND time_created > ? AND role != 'summary'
		 ORDER BY time_created`,
		sessionID, compressedUntil,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var uncompressd []ConversationMessage
	for rows.Next() {
		var msg ConversationMessage
		rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.Tokens, &msg.TimeCreated)
		uncompressd = append(uncompressd, msg)
	}

	if len(uncompressd) < 2 {
		return nil
	}

	keywords := s.compressConfig.ImportantKeywords
	if mode == CompressLayered || mode == CompressAdaptiveFocus || mode == CompressTopicSegment {
		var important []ConversationMessage
		for _, m := range uncompressd {
			for _, kw := range keywords {
				if strings.Contains(m.Content, kw) {
					important = append(important, m)
					break
				}
			}
		}
		if len(important) > 0 {
			s.extractAndSaveImportantContext(sessionID, important)
		}
	}

	newUntil := uncompressd[len(uncompressd)-1].TimeCreated

	var compressed []ConversationMessage
	var wasCompressed bool
	switch mode {
	case CompressSlidingWindow:
		compressed, wasCompressed, _ = s.applySlidingWindow(uncompressd, nil)
	case CompressTokenLimit:
		compressed, wasCompressed, _ = s.applyTokenLimit(uncompressd, nil)
	case CompressSummary:
		compressed, wasCompressed, _ = s.applySummaryCompression(uncompressd, nil)
	case CompressLayered:
		compressed, wasCompressed, _ = s.applyLayeredCompression(uncompressd)
	default:
		compressed, wasCompressed, _ = s.applySlidingWindow(uncompressd, nil)
	}

	if !wasCompressed {
		return nil
	}

	for _, msg := range compressed {
		if msg.Role == "summary" {
			var exists int
			s.db.QueryRow("SELECT COUNT(*) FROM message WHERE id = ?", msg.ID).Scan(&exists)
			if exists == 0 {
				s.db.Exec(
					"INSERT INTO message (id, session_id, role, content, tokens, time_created) VALUES (?, ?, ?, ?, ?, ?)",
					msg.ID, sessionID, msg.Role, msg.Content, msg.Tokens, msg.TimeCreated,
				)
			}
		}
	}

	s.db.Exec("UPDATE session SET compressed_until = ? WHERE id = ?", newUntil, sessionID)
	s.UpdateSessionStats(sessionID)
	s.mergeOldSummaries(sessionID)
	return nil
}

func (s *ConversationStore) extractAndSaveImportantContext(sessionID string, important []ConversationMessage) {
	var parts []string
	for _, m := range important {
		parts = append(parts, fmt.Sprintf("%d(用户): %s", m.TimeCreated, m.Content))
	}
	text := strings.Join(parts, "\n")
	prompt := fmt.Sprintf("从以下对话中提取关键设定和重要信息（50字以内），只输出要点：\n\n%s", text)

	result, err := s.llm.Invoke([]Message{{Role: "user", Content: prompt}})
	if err != nil {
		return
	}
	old := s.GetImportantContextSync(sessionID)
	newCtx := result.Content
	if old != "" {
		newCtx = old + "；" + result.Content
	}
	s.db.Exec("UPDATE session SET important_context = ? WHERE id = ?", newCtx, sessionID)
}

func (s *ConversationStore) GetImportantContext(sessionID string) (string, error) {
	var ctx string
	err := s.db.QueryRow("SELECT COALESCE(important_context, '') FROM session WHERE id = ?", sessionID).Scan(&ctx)
	if err != nil {
		return "", err
	}
	return ctx, nil
}

func (s *ConversationStore) GetImportantContextSync(sessionID string) string {
	ctx, _ := s.GetImportantContext(sessionID)
	return ctx
}

func (s *ConversationStore) SetImportantContext(sessionID, context string) error {
	_, err := s.db.Exec("UPDATE session SET important_context = ? WHERE id = ?", context, sessionID)
	return err
}

func (s *ConversationStore) mergeOldSummaries(sessionID string) {
	rows, err := s.db.Query(
		"SELECT id, content, time_created FROM message WHERE session_id = ? AND role = 'summary' ORDER BY time_created",
		sessionID,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	type summary struct {
		ID          string
		Content     string
		TimeCreated int64
	}
	var summaries []summary
	for rows.Next() {
		var s summary
		rows.Scan(&s.ID, &s.Content, &s.TimeCreated)
		summaries = append(summaries, s)
	}

	if len(summaries) <= 3 {
		return
	}

	s1, s2 := summaries[0], summaries[1]
	prompt := fmt.Sprintf("合并以下两条对话摘要为一条（100字内），去重保留关键信息：\n\n1: %s\n2: %s", s1.Content, s2.Content)
	result, err := s.llm.Invoke([]Message{{Role: "user", Content: prompt}})
	if err != nil {
		return
	}
	merged := truncateStr(result.Content, 200)

	s.db.Exec("DELETE FROM message WHERE id = ?", s1.ID)
	s.db.Exec("DELETE FROM message WHERE id = ?", s2.ID)
	s.db.Exec(
		"INSERT INTO message (id, session_id, role, content, tokens, time_created) VALUES (?, ?, 'summary', ?, ?, ?)",
		uuid.New().String(), sessionID, merged, EstimateTokens(merged), s2.TimeCreated,
	)
}

func (s *ConversationStore) SessionExists(sessionID string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM session WHERE id = ?", sessionID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *ConversationStore) CreateSession(sessionID, title string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		"INSERT INTO session (id, title, time_created, time_updated) VALUES (?, ?, ?, ?)",
		sessionID, title, now, now,
	)
	return err
}

func (s *ConversationStore) UpdateSessionTitle(sessionID, title string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec("UPDATE session SET title = ?, time_updated = ? WHERE id = ?", title, now, sessionID)
	return err
}

func (s *ConversationStore) saveMessage(sessionID, role, content string) error {
	id := uuid.New().String()
	now := time.Now().UnixMilli()
	tokens := EstimateTokens(content)

	var msgCount int
	s.db.QueryRow("SELECT COUNT(*) FROM message WHERE session_id = ?", sessionID).Scan(&msgCount)
	if msgCount == 0 && role == "user" {
		title := generateTitle(content)
		s.UpdateSessionTitle(sessionID, title)
	}

	_, err := s.db.Exec(
		"INSERT INTO message (id, session_id, role, content, tokens, time_created) VALUES (?, ?, ?, ?, ?, ?)",
		id, sessionID, role, content, tokens, now,
	)
	return err
}

func (s *ConversationStore) SaveFullMessage(sessionID, userMsg, assistantMsg string) error {
	s.saveMessage(sessionID, "user", userMsg)
	s.saveMessage(sessionID, "assistant", assistantMsg)
	s.UpdateSessionStats(sessionID)
	return nil
}

func (s *ConversationStore) GetHistory(sessionID string) ([]ConversationMessage, error) {
	var compressedUntil int64
	s.db.QueryRow("SELECT COALESCE(compressed_until, 0) FROM session WHERE id = ?", sessionID).Scan(&compressedUntil)

	rows, err := s.db.Query(
		`SELECT id, session_id, role, content, tokens, time_created 
		 FROM message WHERE session_id = ? AND (time_created > ? OR role = 'summary')
		 ORDER BY time_created`,
		sessionID, compressedUntil,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ConversationMessage
	for rows.Next() {
		var msg ConversationMessage
		rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.Tokens, &msg.TimeCreated)
		messages = append(messages, msg)
	}
	return messages, nil
}

func (s *ConversationStore) GetSessions() ([]SessionInfo, error) {
	rows, err := s.db.Query(
		"SELECT id, title, time_created, message_count FROM session ORDER BY time_updated DESC LIMIT 20",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []SessionInfo
	for rows.Next() {
		var info SessionInfo
		var timeCreated int64
		rows.Scan(&info.SessionID, &info.Title, &timeCreated, &info.MessageCount)
		info.CreatedAt = time.UnixMilli(timeCreated).Format(time.RFC3339)
		sessions = append(sessions, info)
	}
	return sessions, nil
}

func (s *ConversationStore) UpdateSessionStats(sessionID string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		`UPDATE session SET 
			time_updated = ?,
			message_count = (SELECT COUNT(*) FROM message WHERE session_id = ?),
			tokens_used = (SELECT COALESCE(SUM(tokens), 0) FROM message WHERE session_id = ?)
		 WHERE id = ?`,
		now, sessionID, sessionID, sessionID,
	)
	return err
}

func (s *ConversationStore) ClearSession(sessionID string) error {
	s.db.Exec("DELETE FROM message WHERE session_id = ?", sessionID)
	_, err := s.db.Exec("DELETE FROM session WHERE id = ?", sessionID)
	return err
}

func (s *ConversationStore) ClearAll() error {
	s.db.Exec("DELETE FROM message")
	_, err := s.db.Exec("DELETE FROM session")
	return err
}

func (s *ConversationStore) EditMessage(messageID, content string) error {
	tokens := EstimateTokens(content)
	_, err := s.db.Exec("UPDATE message SET content = ?, tokens = ? WHERE id = ?", content, tokens, messageID)
	return err
}

func (s *ConversationStore) DeleteMessage(messageID string) error {
	_, err := s.db.Exec("DELETE FROM message WHERE id = ?", messageID)
	return err
}

func (s *ConversationStore) GetSessionInfo(sessionID string) (*SessionInfo, error) {
	var info SessionInfo
	var timeCreated int64
	err := s.db.QueryRow(
		"SELECT id, title, time_created, message_count FROM session WHERE id = ?", sessionID,
	).Scan(&info.SessionID, &info.Title, &timeCreated, &info.MessageCount)
	if err != nil {
		return nil, err
	}
	info.CreatedAt = time.UnixMilli(timeCreated).Format(time.RFC3339)
	return &info, nil
}

func (s *ConversationStore) RegenerateMessage(messageID string) (string, string, string, error) {
	var sessionID, role string
	err := s.db.QueryRow("SELECT session_id, role FROM message WHERE id = ?", messageID).Scan(&sessionID, &role)
	if err != nil {
		return "", "", "", err
	}
	if role != "assistant" {
		return "", "", "", fmt.Errorf("只能重新生成AI回复")
	}

	s.DeleteMessage(messageID)

	history, _ := s.GetHistory(sessionID)
	var lastUserMsg *ConversationMessage
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			lastUserMsg = &history[i]
			break
		}
	}

	if lastUserMsg == nil {
		return "", "", "", fmt.Errorf("没有找到用户消息")
	}

	result, err := s.llm.Invoke([]Message{
		{Role: "system", Content: "请用中文回答用户问题。"},
		{Role: "user", Content: lastUserMsg.Content},
	})
	if err != nil {
		return "", "", "", err
	}

	newID := uuid.New().String()
	s.saveMessage(sessionID, "assistant", result.Content)
	return sessionID, newID, result.Content, nil
}

func (s *ConversationStore) ImportSession(importData SessionImport) (string, error) {
	sessionID := uuid.New().String()
	title := "导入的会话"
	if importData.Title != "" {
		title = importData.Title
	}
	s.CreateSession(sessionID, title)

	for _, msg := range importData.Messages {
		s.saveMessage(sessionID, msg.Role, msg.Content)
	}
	s.UpdateSessionStats(sessionID)
	return sessionID, nil
}

func (s *ConversationStore) BranchSession(sessionID, fromMessageID string) (string, string, int, error) {
	history, err := s.GetHistory(sessionID)
	if err != nil {
		return "", "", 0, err
	}

	var branchIdx int = -1
	for i, msg := range history {
		if msg.ID == fromMessageID {
			branchIdx = i
			break
		}
	}
	if branchIdx < 0 {
		return "", "", 0, fmt.Errorf("消息 %s 未找到", fromMessageID)
	}

	newSessionID := uuid.New().String()
	title := generateTitle(history[branchIdx].Content)
	s.CreateSession(newSessionID, title)

	for i := 0; i <= branchIdx; i++ {
		s.saveMessage(newSessionID, history[i].Role, history[i].Content)
	}
	s.UpdateSessionStats(newSessionID)

	info, _ := s.GetSessionInfo(newSessionID)
	return newSessionID, title, info.MessageCount, nil
}

func (s *ConversationStore) SearchSessions(query string) ([]SessionInfo, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT s.id, s.title, s.time_created, s.message_count 
		 FROM session s JOIN message m ON s.id = m.session_id 
		 WHERE m.content LIKE ? OR s.title LIKE ?
		 ORDER BY s.time_updated DESC LIMIT 20`,
		"%"+query+"%", "%"+query+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []SessionInfo
	for rows.Next() {
		var info SessionInfo
		var timeCreated int64
		rows.Scan(&info.SessionID, &info.Title, &timeCreated, &info.MessageCount)
		info.CreatedAt = time.UnixMilli(timeCreated).Format(time.RFC3339)
		sessions = append(sessions, info)
	}
	return sessions, nil
}

func (s *ConversationStore) RecordAPICall(apiType string, tokens int64, durationMs int64, success bool) error {
	now := time.Now().UnixMilli()
	successInt := 0
	if success {
		successInt = 1
	}
	_, err := s.db.Exec(
		"INSERT INTO api_stats (api_type, tokens_used, duration_ms, success, time_created) VALUES (?, ?, ?, ?, ?)",
		apiType, tokens, durationMs, successInt, now,
	)
	return err
}

type ApiStatsSummary struct {
	TotalCalls      int64          `json:"total_calls"`
	TotalTokens     int64          `json:"total_tokens"`
	TotalDurationMs int64          `json:"total_duration_ms"`
	SuccessCount    int64          `json:"success_count"`
	CallsToday      int64          `json:"calls_today"`
	TokensToday     int64          `json:"tokens_today"`
	AvgDurationTodayMs int64      `json:"avg_duration_today_ms"`
	CallsThisWeek   int64          `json:"calls_this_week"`
	TokensThisWeek  int64          `json:"tokens_this_week"`
	APITypes        []ApiTypeStats `json:"api_types"`
}

type ApiTypeStats struct {
	APIType       string `json:"api_type"`
	CallCount     int64  `json:"call_count"`
	TokensUsed    int64  `json:"tokens_used"`
	AvgDurationMs int64  `json:"avg_duration_ms"`
}

type RecentCall struct {
	ID          int64  `json:"id"`
	APIType     string `json:"api_type"`
	TokensUsed  int64  `json:"tokens_used"`
	DurationMs  int64  `json:"duration_ms"`
	Success     bool   `json:"success"`
	TimeCreated string `json:"time_created"`
}

func (s *ConversationStore) GetAPIStats() (*ApiStatsSummary, error) {
	now := time.Now().UnixMilli()
	dayAgo := now - 86400000
	weekAgo := now - 604800000

	var stats ApiStatsSummary
	s.db.QueryRow(
		"SELECT COUNT(*), COALESCE(SUM(tokens_used),0), COALESCE(SUM(duration_ms),0), SUM(CASE WHEN success=1 THEN 1 ELSE 0 END) FROM api_stats",
	).Scan(&stats.TotalCalls, &stats.TotalTokens, &stats.TotalDurationMs, &stats.SuccessCount)

	s.db.QueryRow(
		"SELECT COUNT(*), COALESCE(SUM(tokens_used),0), COALESCE(AVG(duration_ms),0) FROM api_stats WHERE time_created > ?",
		dayAgo,
	).Scan(&stats.CallsToday, &stats.TokensToday, &stats.AvgDurationTodayMs)

	s.db.QueryRow(
		"SELECT COUNT(*), COALESCE(SUM(tokens_used),0) FROM api_stats WHERE time_created > ?",
		weekAgo,
	).Scan(&stats.CallsThisWeek, &stats.TokensThisWeek)

	rows, err := s.db.Query(
		"SELECT api_type, COUNT(*), COALESCE(SUM(tokens_used),0), COALESCE(AVG(duration_ms),0) FROM api_stats GROUP BY api_type",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var t ApiTypeStats
		rows.Scan(&t.APIType, &t.CallCount, &t.TokensUsed, &t.AvgDurationMs)
		stats.APITypes = append(stats.APITypes, t)
	}
	return &stats, nil
}

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

func EstimateTokens(text string) int {
	runes := []rune(text)
	chineseCount := 0
	for _, r := range runes {
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseCount++
		}
	}
	return chineseCount*2 + (len(runes)-chineseCount)/4 + 1
}

func generateTitle(content string) string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) > 50 {
		return string(runes[:50]) + "..."
	}
	return string(runes)
}

func truncateStr(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n])
	}
	return s
}

type SessionImport struct {
	Title    string
	Messages []ImportMessage
}

type ImportMessage struct {
	Role    string
	Content string
}

type SessionExport struct {
	SessionID string
	Title     string
	CreatedAt string
	Messages  []ConversationMessage
}

// JSON helpers
func (s *ConversationStore) ToJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
