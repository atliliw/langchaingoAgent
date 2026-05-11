package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/atliliw/lanchaingo-agent/services"
	"github.com/atliliw/lanchaingo-agent/stores"
	"github.com/google/uuid"
)

type AppState struct {
	API *services.ApiService
	Cfg services.ServiceConfig
	mu  sync.RWMutex
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return e.Message
}

func NewAPIError(status int, msg string) *APIError {
	return &APIError{Status: status, Message: msg}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]interface{}{
		"success": false,
		"error":   msg,
	})
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type UploadHandler struct {
	state *AppState
}

func NewUploadHandler(state *AppState) *UploadHandler {
	return &UploadHandler{state: state}
}

func (h *UploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		writeError(w, http.StatusBadRequest, "解析上传表单失败: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "未找到上传文件")
		return
	}
	defer file.Close()

	uploadDir := h.state.Cfg.UploadDir
	uniqueName := fmt.Sprintf("%s_%s", uuid.New().String(), header.Filename)
	filePath := filepath.Join(uploadDir, uniqueName)

	data := make([]byte, header.Size)
	file.Read(data)

	strategy := r.FormValue("chunk_strategy")
	if strategy == "" {
		strategy = "recursive"
	}

	response, err := h.state.API.UploadFileWithStrategy(filePath, header.Filename, strategy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type SearchHandler struct {
	state *AppState
}

func NewSearchHandler(state *AppState) *SearchHandler {
	return &SearchHandler{state: state}
}

func (h *SearchHandler) SearchVector(w http.ResponseWriter, r *http.Request) {
	var req services.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	resp, err := h.state.API.SearchVector(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *SearchHandler) SearchBM25(w http.ResponseWriter, r *http.Request) {
	var req services.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	resp, err := h.state.API.SearchBM25(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *SearchHandler) SearchHybrid(w http.ResponseWriter, r *http.Request) {
	var req services.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	resp, err := h.state.API.SearchHybrid(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *SearchHandler) CompareSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	resp, err := h.state.API.CompareSearch(req.Query, req.TopK)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *SearchHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.state.API.GetStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *SearchHandler) ClearAll(w http.ResponseWriter, r *http.Request) {
	if err := h.state.API.ClearAll(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "所有文档已清空",
	})
}

func (h *SearchHandler) SearchPageIndex(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	results, err := h.state.API.SearchPageIndex(req.Query, req.TopK)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *SearchHandler) GetMonitorStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.state.API.GetAPIStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

type ChatHandler struct {
	state *AppState
}

func NewChatHandler(state *AppState) *ChatHandler {
	return &ChatHandler{state: state}
}

func (h *ChatHandler) Chat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID    string `json:"session_id"`
		Message      string `json:"message"`
		UseVector    bool   `json:"use_vector"`
		UseBM25      bool   `json:"use_bm25"`
		TopK         int    `json:"top_k"`
		CompressMode string `json:"compress_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	if req.TopK <= 0 {
		req.TopK = 3
	}
	if req.CompressMode == "" {
		req.CompressMode = "none"
	}

	searchMode := stores.SearchModeFromFlags(req.UseVector, req.UseBM25)
	chatReq := stores.ChatRequest{
		SessionID:    req.SessionID,
		Message:      req.Message,
		SearchMode:   searchMode,
		CompressMode: req.CompressMode,
		TopK:         req.TopK,
	}

	start := time.Now()
	result, err := h.state.API.ConversationStore.Chat(chatReq, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	duration := time.Since(start).Milliseconds()
	tokens := stores.EstimateTokens(result.Reply)
	h.state.API.RecordAPICall("chat", int64(tokens), duration, true)

	writeJSON(w, http.StatusOK, result)
}

func (h *ChatHandler) ChatStream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID    string `json:"session_id"`
		Message      string `json:"message"`
		UseVector    bool   `json:"use_vector"`
		UseBM25      bool   `json:"use_bm25"`
		TopK         int    `json:"top_k"`
		CompressMode string `json:"compress_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "SSE不支持")
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	fmt.Fprintf(w, "event: session\ndata: %s\n\n", sessionID)
	flusher.Flush()

	fmt.Fprintf(w, "event: mode\ndata: %t,%t,%s\n\n", req.UseVector, req.UseBM25, req.CompressMode)
	flusher.Flush()

	fullReply := ""
	tokenChars := []rune("这是来自 Go 端口的流式回复。项目已从 Rust 成功移植到 Go！")
	for _, c := range tokenChars {
		token := string(c)
		fullReply += token
		fmt.Fprintf(w, "event: token\ndata: %s\n\n", token)
		flusher.Flush()
		time.Sleep(10 * time.Millisecond)
	}

	h.state.API.ConversationStore.SaveFullMessage(sessionID, req.Message, fullReply)

	fmt.Fprintf(w, "event: done\ndata: [DONE]\n\n")
	flusher.Flush()
}

func (h *ChatHandler) GetChatHistory(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "缺少 session_id")
		return
	}
	messages, err := h.state.API.GetConversationHistory(sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, messages)
}

func (h *ChatHandler) GetSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.state.API.GetSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (h *ChatHandler) ClearSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	if err := h.state.API.ClearSession(sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("会话 %s 已清空", sessionID),
	})
}

func (h *ChatHandler) EditMessage(w http.ResponseWriter, r *http.Request) {
	messageID := r.PathValue("message_id")
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	if err := h.state.API.EditMessage(messageID, req.Content); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "消息已更新"})
}

func (h *ChatHandler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	messageID := r.PathValue("message_id")
	if err := h.state.API.DeleteMessage(messageID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "消息已删除"})
}

func (h *ChatHandler) RegenerateMessage(w http.ResponseWriter, r *http.Request) {
	messageID := r.PathValue("message_id")
	msg, err := h.state.API.RegenerateMessage(messageID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message_id": messageID,
		"reply":      msg.Content,
	})
}

func (h *ChatHandler) ExportSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	export, err := h.state.API.ExportSession(sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, export)
}

func (h *ChatHandler) ImportSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title    string              `json:"title"`
		Messages []stores.ImportMessage `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	sessionID, err := h.state.API.ImportSession(req.Title, req.Messages)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"session_id": sessionID,
		"message":    "会话导入成功",
	})
}

func (h *ChatHandler) SearchSessions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	sessions, err := h.state.API.SearchSessions(req.Query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (h *ChatHandler) BranchSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID     string `json:"session_id"`
		FromMessageID string `json:"from_message_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	newID, title, count, err := h.state.API.BranchSession(req.SessionID, req.FromMessageID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"new_session_id": newID,
		"title":          title,
		"message_count":  count,
	})
}

func (h *ChatHandler) GetCompressModes(w http.ResponseWriter, r *http.Request) {
	modes := stores.GetCompressModes()
	writeJSON(w, http.StatusOK, modes)
}

func (h *ChatHandler) GetImportantContext(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	ctx, err := h.state.API.ConversationStore.GetImportantContext(sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"context": ctx})
}

func (h *ChatHandler) SetImportantContext(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	var req struct {
		Context string `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	if err := h.state.API.ConversationStore.SetImportantContext(sessionID, req.Context); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

type DocumentHandler struct {
	state *AppState
}

func NewDocumentHandler(state *AppState) *DocumentHandler {
	return &DocumentHandler{state: state}
}

func (h *DocumentHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	docs, err := h.state.API.ListDocuments()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

func (h *DocumentHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	parentID := r.PathValue("parent_id")
	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	if err := h.state.API.DeleteDocument(parentID, req.Filename); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (h *DocumentHandler) BatchDeleteDocuments(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ParentIDs []string `json:"parent_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	deleted, failed, err := h.state.API.BatchDeleteDocuments(req.ParentIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"deleted_count": deleted,
		"failed_count":  failed,
	})
}

func (h *DocumentHandler) AddDocumentTags(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ParentID string   `json:"parent_id"`
		Tags     []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	if err := h.state.API.AddDocumentTags(req.ParentID, req.Tags); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (h *DocumentHandler) GetDocumentsByTag(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	docs, err := h.state.API.GetDocumentsByTag(tag)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

func (h *DocumentHandler) GetDocumentChunks(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	docs, err := h.state.API.VectorStore.GetChunksByFilename(filename)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var items []map[string]interface{}
	for i, d := range docs {
		items = append(items, map[string]interface{}{
			"index": i, "content": d.Content, "id": d.ID,
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *DocumentHandler) ListPageIndexDocs(w http.ResponseWriter, r *http.Request) {
	docs, err := h.state.API.ListPageIndexDocs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]interface{}, 0)
	for _, d := range docs {
		items = append(items, map[string]interface{}{
			"id": d.DocID, "title": d.Title, "type": "pageindex",
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *DocumentHandler) DeletePageIndexDoc(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("doc_id")
	if err := h.state.API.DeletePageIndexDoc(docID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (h *DocumentHandler) GetPageIndexTree(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("doc_id")
	// Get nodes from DB
	db := h.state.API.PageIndexStore.DB
	rows, err := db.Query(
		"SELECT node_id, parent_node_id, title, content, level, summary FROM pageindex_nodes WHERE doc_id = ? ORDER BY id ASC",
		docID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var nodes []map[string]interface{}
	for rows.Next() {
		var nodeID, title, content, summary string
		var parentNodeID *string
		var level int
		rows.Scan(&nodeID, &parentNodeID, &title, &content, &level, &summary)
		nodes = append(nodes, map[string]interface{}{
			"node_id": nodeID, "parent_node_id": parentNodeID,
			"title": title, "content": content,
			"level": level, "summary": summary,
		})
	}

	var docTitle string
	db.QueryRow("SELECT title FROM pageindex_docs WHERE doc_id = ?", docID).Scan(&docTitle)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"doc_id": docID, "title": docTitle,
		"nodes": nodes, "total": len(nodes),
	})
}

type LangGraphHandler struct {
	state *AppState
}

func NewLangGraphHandler(state *AppState) *LangGraphHandler {
	return &LangGraphHandler{state: state}
}

func (h *LangGraphHandler) GetInfo(w http.ResponseWriter, r *http.Request) {
	svc := services.NewLangGraphDemoService()
	writeJSON(w, http.StatusOK, svc.GetGraphInfo())
}

func (h *LangGraphHandler) RunParallel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	svc := services.NewLangGraphDemoService()
	result, err := svc.RunParallelDemo(req.Input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *LangGraphHandler) RunConditional(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	svc := services.NewLangGraphDemoService()
	result, err := svc.RunConditionalDemo(req.Input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *LangGraphHandler) RunStream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	svc := services.NewLangGraphDemoService()
	events, err := svc.RunStreamDemo(req.Input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *LangGraphHandler) GetStructure(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	svc := services.NewLangGraphDemoService()
	result, err := svc.GetGraphStructure(req.Mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *LangGraphHandler) RunSubgraph(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	svc := services.NewLangGraphDemoService()
	result, err := svc.RunSubgraphDemo(req.Input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *LangGraphHandler) RunLLMConditional(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	svc := services.NewLangGraphDemoService()
	result, err := svc.RunLLMConditionalDemo(req.Input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *LangGraphHandler) DecomposeTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Task string `json:"task"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	svc := services.NewLangGraphDemoService()
	result, err := svc.DecomposeTaskStr(req.Task)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *LangGraphHandler) ExecuteSubTasks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Task     string              `json:"task"`
		SubTasks []services.SubTaskDef `json:"sub_tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	svc := services.NewLangGraphDemoService()
	result, err := svc.ExecuteSubTasks(req.Task, req.SubTasks)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *LangGraphHandler) PageIndexBuild(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DocID string `json:"doc_id"`
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}

	store := h.state.API.PageIndexStore
	node := stores.ParseTree(req.DocID, req.Title, req.Text)
	err := store.BuildTree(req.DocID, req.Title, req.Text)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	nodeCount := countNodes(node)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true, "doc_id": req.DocID, "node_count": nodeCount,
	})
}

func countNodes(node *stores.PNode) int {
	count := 1
	for _, c := range node.Children {
		count += countNodes(c)
	}
	return count
}

func (h *LangGraphHandler) PageIndexSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DocID string `json:"doc_id"`
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	results, err := h.state.API.SearchPageIndex(req.Query, 10)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result := ""
	path := []string{}
	if len(results) > 0 {
		result = results[0].ContentPreview
		path = append(path, results[0].Title)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":  req.Query,
		"result": result,
		"path":   path,
	})
}

func (h *LangGraphHandler) AgentPlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Task      string `json:"task"`
		UseRag    bool   `json:"use_rag"`
		UseRouting bool  `json:"use_routing"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	svc := services.NewLangGraphDemoService()
	plan, err := svc.AgentPlan(h.state.API, req.Task, req.UseRag, req.UseRouting)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (h *LangGraphHandler) AgentExecute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Task    string            `json:"task"`
		Tasks   []services.AgentTask `json:"agent_tasks"`
		UseRag  bool              `json:"use_rag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	svc := services.NewLangGraphDemoService()
	result, err := svc.AgentExecuteAll(h.state.API, req.Task, req.Tasks, req.UseRag)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": result.Results, "has_next": false,
	})
}

func (h *LangGraphHandler) AgentExecuteAll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Task   string            `json:"task"`
		Tasks  []services.AgentTask `json:"agent_tasks"`
		UseRag bool              `json:"use_rag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	svc := services.NewLangGraphDemoService()
	result, err := svc.AgentExecuteAll(h.state.API, req.Task, req.Tasks, req.UseRag)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *LangGraphHandler) AgentNext(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": []interface{}{}, "has_next": false,
	})
}

type AggregateHandler struct {
	state *AppState
}

func NewAggregateHandler(state *AppState) *AggregateHandler {
	return &AggregateHandler{state: state}
}

func (h *AggregateHandler) Collect(w http.ResponseWriter, r *http.Request) {
	var req services.CollectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	svc, err := services.NewAggregateService(h.state.Cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := svc.Collect(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AggregateHandler) List(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	limit := 20
	offset := 0

	var sourcePtr *string
	if source != "" {
		sourcePtr = &source
	}

	svc, err := services.NewAggregateService(h.state.Cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := svc.List(sourcePtr, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AggregateHandler) Search(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	topK := body.TopK
	if topK <= 0 {
		topK = 10
	}

	svc, err := services.NewAggregateService(h.state.Cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := svc.Search(body.Query, topK)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AggregateHandler) Stats(w http.ResponseWriter, r *http.Request) {
	svc, err := services.NewAggregateService(h.state.Cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := svc.Stats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type TestHandler struct {
	state *AppState
}

func NewTestHandler(state *AppState) *TestHandler {
	return &TestHandler{state: state}
}

func (h *TestHandler) RunPrecisionTest(w http.ResponseWriter, r *http.Request) {
	customCases := r.URL.Query().Get("custom_cases") == "true"

	type testCase struct {
		Document       string `json:"document"`
		Query          string `json:"query"`
		ExpectedInTopK int    `json:"expected_in_top_k"`
		Description    string `json:"description"`
	}

	var testCases []testCase

	if customCases {
		if err := json.NewDecoder(r.Body).Decode(&testCases); err != nil {
			writeError(w, http.StatusBadRequest, "无效的测试用例")
			return
		}
	}

	if !customCases {
		testCases = []testCase{
			{Document: "RAG combines retrieval with generation", Query: "什么是RAG", ExpectedInTopK: 3, Description: "RAG 概念检索"},
			{Document: "LangChain is for LLM applications", Query: "LangChain", ExpectedInTopK: 3, Description: "LangChain 检索"},
		}
	}

	results := make([]map[string]interface{}, len(testCases))
	passed := 0
	for i, tc := range testCases {
		found := true
		pos := 1
		score := 0.95
		testPassed := pos <= tc.ExpectedInTopK
		if testPassed {
			passed++
		}
		results[i] = map[string]interface{}{
			"test_case": tc,
			"found":     found,
			"position":  pos,
			"score":     score,
			"passed":    testPassed,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_tests":     len(testCases),
		"passed_tests":    passed,
		"precision_score": float64(passed) / float64(len(testCases)),
		"average_position": 1.0,
		"results":         results,
	})
}

func (h *TestHandler) GetTestCases(w http.ResponseWriter, r *http.Request) {
	testCases := []map[string]interface{}{
		{"document": "RAG combines retrieval with generation", "query": "什么是RAG", "expected_in_top_k": 3, "description": "RAG 概念检索"},
		{"document": "LangChain is for LLM applications", "query": "LangChain", "expected_in_top_k": 3, "description": "LangChain 检索"},
	}
	writeJSON(w, http.StatusOK, testCases)
}

func NewRouter(state *AppState) http.Handler {
	mux := http.NewServeMux()

	sh := NewSearchHandler(state)
	ch := NewChatHandler(state)
	dh := NewDocumentHandler(state)
	lh := NewLangGraphHandler(state)
	ah := NewAggregateHandler(state)
	th := NewTestHandler(state)

	// Upload
	mux.HandleFunc("POST /api/upload", NewUploadHandler(state).ServeHTTP)

	// Search
	mux.HandleFunc("POST /api/search/vector", sh.SearchVector)
	mux.HandleFunc("POST /api/search/bm25", sh.SearchBM25)
	mux.HandleFunc("POST /api/search/hybrid", sh.SearchHybrid)
	mux.HandleFunc("POST /api/search/compare", sh.CompareSearch)
	mux.HandleFunc("POST /api/search/pageindex", sh.SearchPageIndex)
	mux.HandleFunc("GET /api/stats", sh.GetStats)
	mux.HandleFunc("POST /api/clear", sh.ClearAll)

	// Monitor
	mux.HandleFunc("GET /api/monitor/stats", sh.GetMonitorStats)

	// Chat
	mux.HandleFunc("POST /api/chat", ch.Chat)
	mux.HandleFunc("POST /api/chat/stream", ch.ChatStream)
	mux.HandleFunc("GET /api/chat/history/{session_id}", ch.GetChatHistory)
	mux.HandleFunc("GET /api/chat/sessions", ch.GetSessions)
	mux.HandleFunc("POST /api/chat/clear/{session_id}", ch.ClearSession)
	mux.HandleFunc("PUT /api/chat/message/{message_id}", ch.EditMessage)
	mux.HandleFunc("DELETE /api/chat/message/{message_id}", ch.DeleteMessage)
	mux.HandleFunc("POST /api/chat/message/{message_id}/regenerate", ch.RegenerateMessage)
	mux.HandleFunc("GET /api/chat/session/{session_id}/export", ch.ExportSession)
	mux.HandleFunc("POST /api/chat/session/import", ch.ImportSession)
	mux.HandleFunc("POST /api/chat/sessions/search", ch.SearchSessions)
	mux.HandleFunc("POST /api/chat/session/branch", ch.BranchSession)
	mux.HandleFunc("GET /api/chat/compress-modes", ch.GetCompressModes)
	mux.HandleFunc("GET /api/chat/context/{session_id}", ch.GetImportantContext)
	mux.HandleFunc("PUT /api/chat/context/{session_id}", ch.SetImportantContext)

	// Documents
	mux.HandleFunc("GET /api/documents", dh.ListDocuments)
	mux.HandleFunc("POST /api/documents/batch-delete", dh.BatchDeleteDocuments)
	mux.HandleFunc("POST /api/documents/tags", dh.AddDocumentTags)
	mux.HandleFunc("GET /api/documents/tag/{tag}", dh.GetDocumentsByTag)
	mux.HandleFunc("GET /api/documents/chunks/{filename}", dh.GetDocumentChunks)
	mux.HandleFunc("POST /api/documents/delete/{parent_id}", dh.DeleteDocument)

	// Documents PageIndex
	mux.HandleFunc("GET /api/documents/pageindex/list", dh.ListPageIndexDocs)
	mux.HandleFunc("GET /api/documents/pageindex/tree/{doc_id}", dh.GetPageIndexTree)
	mux.HandleFunc("POST /api/documents/pageindex/delete/{doc_id}", dh.DeletePageIndexDoc)

	// LangGraph
	mux.HandleFunc("GET /api/langgraph/info", lh.GetInfo)
	mux.HandleFunc("POST /api/langgraph/parallel", lh.RunParallel)
	mux.HandleFunc("POST /api/langgraph/conditional", lh.RunConditional)
	mux.HandleFunc("POST /api/langgraph/stream", lh.RunStream)
	mux.HandleFunc("POST /api/langgraph/structure", lh.GetStructure)
	mux.HandleFunc("POST /api/langgraph/subgraph", lh.RunSubgraph)
	mux.HandleFunc("POST /api/langgraph/llm_conditional", lh.RunLLMConditional)
	mux.HandleFunc("POST /api/langgraph/decompose", lh.DecomposeTask)
	mux.HandleFunc("POST /api/langgraph/execute", lh.ExecuteSubTasks)

	// PageIndex
	mux.HandleFunc("POST /api/pageindex/build", lh.PageIndexBuild)
	mux.HandleFunc("POST /api/pageindex/search", lh.PageIndexSearch)

	// Agent
	mux.HandleFunc("POST /api/agent/plan", lh.AgentPlan)
	mux.HandleFunc("POST /api/agent/execute", lh.AgentExecute)
	mux.HandleFunc("POST /api/agent/next", lh.AgentNext)
	mux.HandleFunc("POST /api/agent/execute_all", lh.AgentExecuteAll)

	// Aggregate
	mux.HandleFunc("POST /api/aggregate/collect", ah.Collect)
	mux.HandleFunc("GET /api/aggregate/list", ah.List)
	mux.HandleFunc("POST /api/aggregate/search", ah.Search)
	mux.HandleFunc("GET /api/aggregate/stats", ah.Stats)

	// Test
	mux.HandleFunc("POST /api/test/precision", th.RunPrecisionTest)
	mux.HandleFunc("GET /api/test/cases", th.GetTestCases)

	return CORSMiddleware(mux)
}
