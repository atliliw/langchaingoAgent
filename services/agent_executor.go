package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// ── Available Tools ──

type ToolInfo struct {
	Name        string
	Description string
}

var AvailableTools = []ToolInfo{
	{Name: "llm_query", Description: "直接用 LLM 回答"},
	{Name: "web_search", Description: "搜索网络获取信息"},
	{Name: "weather", Description: "查询天气"},
	{Name: "code_execute", Description: "执行代码"},
	{Name: "read_file", Description: "读取文件"},
	{Name: "summarize", Description: "总结"},
	{Name: "rag_search", Description: "检索知识库（RAG）获取与任务相关的文档内容"},
}

// ── Agent Task Types ──

type AgentTask struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Tool          string            `json:"tool"`
	DependsOn     []string          `json:"depends_on"`
	InputTemplate string            `json:"input_template"`
	TaskType      string            `json:"task_type"`
	Routes        map[string]string `json:"routes"`
}

type AgentPlan struct {
	OriginalTask  string        `json:"original_task"`
	Tasks         []AgentTask   `json:"tasks"`
	GraphStructure interface{}  `json:"graph_structure"`
}

type AgentExecResult struct {
	TaskName     string `json:"task_name"`
	Tool         string `json:"tool"`
	InputSummary string `json:"input_summary"`
	Output       string `json:"output"`
	DurationMs   uint64 `json:"duration_ms"`
	Tokens       int    `json:"tokens"`
}

type AgentExecResponse struct {
	Results         []AgentExecResult `json:"results"`
	FinalAnswer     string            `json:"final_answer"`
	TotalDurationMs uint64            `json:"total_duration_ms"`
	TotalTokens     int               `json:"total_tokens"`
}

// ── In-Memory Session Store ──

type BatchState struct {
	Task            string
	All             []AgentTask
	Done            []AgentExecResult
	CompletedNames  map[string]bool
	Start           time.Time
	RagContext      string
	ReviewPending   []AgentTask
}

var (
	storeMu sync.Mutex
	store   = make(map[string]*BatchState)
)

// ── SQLite Persistence ──

func ensureAgentTables(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_sessions (
			session_id TEXT PRIMARY KEY,
			task TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			status TEXT NOT NULL DEFAULT 'running'
		)
	`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			task_name TEXT NOT NULL,
			tool TEXT NOT NULL DEFAULT '',
			output TEXT NOT NULL DEFAULT '',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			tokens INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (session_id) REFERENCES agent_sessions(session_id)
		)
	`)
	return err
}

func saveAgentSession(db *sql.DB, sessionID, task string) {
	db.Exec(`INSERT OR IGNORE INTO agent_sessions (session_id, task, status) VALUES (?, ?, 'running')`, sessionID, task)
}

func saveAgentResult(db *sql.DB, sessionID string, r *AgentExecResult) {
	db.Exec(`INSERT INTO agent_results (session_id, task_name, tool, output, duration_ms, tokens) VALUES (?, ?, ?, ?, ?, ?)`,
		sessionID, r.TaskName, r.Tool, r.Output, r.DurationMs, r.Tokens)
}

func updateAgentSessionStatus(db *sql.DB, sessionID, status string) {
	db.Exec("UPDATE agent_sessions SET status = ? WHERE session_id = ?", status, sessionID)
}

// ── Token Estimation ──

func estimateTokenUsage(prompt, response string) int {
	inputTokens := len([]rune(prompt))/2 + 1
	outputTokens := len([]rune(response))/2 + 1
	return inputTokens + outputTokens
}

// ── JSON Fixer ──

func fixJSON(input string) string {
	s := strings.TrimSpace(input)

	// 1. Remove BOM
	s = strings.TrimPrefix(s, "\uFEFF")

	// 2. Extract JSON boundaries
	start := -1
	end := -1
	for i, c := range s {
		if c == '[' || c == '{' {
			if start < 0 {
				start = i
			}
		}
		if c == ']' || c == '}' {
			end = i
		}
	}
	if start >= 0 && end >= 0 && start <= end {
		s = s[start : end+1]
	}

	// 3. Fix trailing commas
	for strings.Contains(s, ",]") || strings.Contains(s, ",}") {
		s = strings.ReplaceAll(s, ",]", "]")
		s = strings.ReplaceAll(s, ",}", "}")
	}

	// 4. Replace single quotes around keys with double quotes
	chars := []rune(s)
	for i := 0; i < len(chars); i++ {
		if chars[i] == '\'' {
			prev := ' '
			if i > 0 {
				prev = chars[i-1]
			}
			next := ' '
			if i+1 < len(chars) {
				next = chars[i+1]
			}
			if prev == '{' || prev == ',' || prev == ':' || next == ':' || next == ',' || next == '}' || next == ']' {
				chars[i] = '"'
			}
		}
	}
	s = string(chars)

	// 5. Add quotes to unquoted keys
	var result strings.Builder
	runes := []rune(s)
	for j := 0; j < len(runes); j++ {
		if (j == 0 || runes[j-1] == '{' || runes[j-1] == ',') && isAlpha(runes[j]) {
			k := j
			for k < len(runes) && (isAlphaNum(runes[k]) || runes[k] == '_') {
				k++
			}
			if k < len(runes) && runes[k] == ':' {
				result.WriteRune('"')
				result.WriteString(string(runes[j:k]))
				result.WriteRune('"')
				j = k - 1
				continue
			}
		}
		result.WriteRune(runes[j])
	}
	return result.String()
}

func isAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isAlphaNum(r rune) bool {
	return isAlpha(r) || (r >= '0' && r <= '9')
}

// ── AgentEngine ──

type AgentEngine struct{}

func parseTasks(s string) ([]AgentTask, error) {
	var tasks []AgentTask
	err := json.Unmarshal([]byte(s), &tasks)
	if err != nil {
		return nil, fmt.Errorf("JSON 格式错误 (%v): %s", err, truncateStr(s, 200))
	}
	return tasks, nil
}

// ── Plan ──

func (e *AgentEngine) Plan(config ServiceConfig, task string, ragContext string, useRag bool, useRouting bool) (*AgentPlan, error) {
	content := mockLLMPlan(task)

	tasks, err := parseTasks(content)
	if err != nil {
		// Try fix JSON
		fixed := fixJSON(content)
		tasks, err = parseTasks(fixed)
		if err != nil {
			// Fallback: generate default tasks
			tasks = generateDefaultTasks(task)
		}
	}

	if len(tasks) == 0 {
		tasks = generateDefaultTasks(task)
	}

	gs := buildGraph(tasks)
	return &AgentPlan{
		OriginalTask:  task,
		Tasks:         tasks,
		GraphStructure: gs,
	}, nil
}

func mockLLMPlan(task string) string {
	return fmt.Sprintf(`[
		{"name":"分析需求","description":"分析任务「%s」的需求","tool":"llm_query","task_type":"normal","depends_on":[],"input_template":""},
		{"name":"搜索信息","description":"收集相关信息","tool":"web_search","task_type":"normal","depends_on":["分析需求"],"input_template":""},
		{"name":"总结报告","description":"生成总结报告","tool":"llm_query","task_type":"normal","depends_on":["搜索信息"],"input_template":""}
	]`, task)
}

func generateDefaultTasks(task string) []AgentTask {
	return []AgentTask{
		{Name: "知识库检索", Description: "检索相关知识", Tool: "rag_search", DependsOn: []string{}, TaskType: "normal"},
		{Name: "信息分析", Description: "分析检索结果", Tool: "llm_query", DependsOn: []string{"知识库检索"}, TaskType: "normal"},
		{Name: "总结报告", Description: "生成总结报告", Tool: "llm_query", DependsOn: []string{"信息分析"}, TaskType: "normal"},
	}
}

func buildGraph(tasks []AgentTask) interface{} {
	names := make(map[string]bool)
	for _, t := range tasks {
		names[t.Name] = true
	}

	var nodes []string
	var edges []map[string]string

	for _, t := range tasks {
		nodes = append(nodes, t.Name)
	}

	// START -> tasks with no unresolved dependencies
	for _, t := range tasks {
		hasUnresolvedDep := false
		for _, d := range t.DependsOn {
			if names[d] {
				hasUnresolvedDep = true
				break
			}
		}
		if !hasUnresolvedDep {
			edges = append(edges, map[string]string{"source": "__start__", "target": t.Name, "type": "fixed"})
		}
	}

	// depends_on edges
	for _, t := range tasks {
		for _, d := range t.DependsOn {
			if names[d] {
				edges = append(edges, map[string]string{"source": d, "target": t.Name, "type": "fixed"})
			}
		}
		edges = append(edges, map[string]string{"source": t.Name, "target": "__end__", "type": "fixed"})
	}

	return map[string]interface{}{
		"entry_point": tasks[0].Name,
		"nodes":       nodes,
		"edges":       edges,
	}
}

// ── Batch Management ──

func readyBatch(tasks []AgentTask, done map[string]bool) []AgentTask {
	names := make(map[string]bool)
	for _, t := range tasks {
		names[t.Name] = true
	}
	var ready []AgentTask
	for _, t := range tasks {
		if done[t.Name] {
			continue
		}
		allResolved := true
		for _, d := range t.DependsOn {
			if names[d] && !done[d] {
				allResolved = false
				break
			}
		}
		if allResolved {
			ready = append(ready, t)
		}
	}
	return ready
}

// ── Build Batch Graph ──

func buildBatchGraph(config ServiceConfig, task string, batch []AgentTask, ctx string, rag string) ([]string, error) {
	// Simplified: return task names that will be executed
	var names []string
	for _, t := range batch {
		names = append(names, t.Name)
	}
	return names, nil
}

// ── Run Batch ──

func runBatch(config ServiceConfig, task string, batch []AgentTask, context []AgentExecResult, ragContext string) ([]AgentExecResult, error) {
	ctxStr := ""
	for _, r := range context {
		ctxStr += fmt.Sprintf("【%s】\n%s\n\n", r.TaskName, r.Output)
	}

	var results []AgentExecResult
	for _, at := range batch {
		start := time.Now()

		var output string
		var tokens int

		switch at.TaskType {
		case "human_review":
			output = fmt.Sprintf("⏸️ 待人工审批：%s", at.Description)
			tokens = 0

		case "decision":
			routeOptions := make([]string, 0, len(at.Routes))
			for k := range at.Routes {
				routeOptions = append(routeOptions, "「"+k+"」")
			}
			prompt := fmt.Sprintf("基于以下信息做出判断，只返回决策结果（%s），不要多余内容。\n\n%s\n\n当前决策：%s",
				strings.Join(routeOptions, " 或 "), ctxStr, at.Description)
			output = mockLLMResponse(prompt)
			tokens = estimateTokenUsage(prompt, output)

		default:
			switch at.Tool {
			case "llm_query", "":
				prompt := fmt.Sprintf("任务：%s\n当前子任务：%s\n\n前置结果：\n%s\n\n请执行当前子任务并输出结果。",
					task, at.Description, ctxStr)
				output = mockLLMResponse(prompt)
				tokens = estimateTokenUsage(prompt, output)

			case "web_search":
				prompt := fmt.Sprintf("任务：%s\n当前子任务：%s\n\n请基于你的知识回答。",
					task, at.Description)
				output = mockLLMResponse(prompt)
				tokens = estimateTokenUsage(prompt, output)

			case "weather":
				cityPrompt := fmt.Sprintf("任务：%s\n当前子任务：%s\n\n请输出要查询天气的城市名，不要多余内容。",
					task, at.Description)
				city := mockLLMResponse(cityPrompt)
				city = strings.TrimSpace(city)
				if city == "" {
					city = at.Description
				}
				tool := NewWeatherTool()
				wResult, err := tool.Invoke(WeatherInput{City: city})
				if err != nil {
					output = fmt.Sprintf("天气查询失败: %v", err)
				} else {
					output = fmt.Sprintf("%s的天气：%s", city, wResult.Weather)
				}
				tokens = 0

			case "rag_search":
				query := at.InputTemplate
				if query == "" {
					query = at.Name
				}
				if ragContext != "" {
					output = fmt.Sprintf("知识库检索到以下相关信息（搜索词：%s）：\n\n%s", query, ragContext)
				} else {
					output = fmt.Sprintf("知识库中未找到相关文档（搜索词：%s）", query)
				}
				tokens = len([]rune(query))

			default:
				prompt := fmt.Sprintf("任务：%s\n子任务：%s\n(工具:%s 不可用，请直接用 LLM 执行)\n\n上下文：\n%s\n\n输出结果。",
					task, at.Description, at.Tool, ctxStr)
				output = mockLLMResponse(prompt)
				tokens = estimateTokenUsage(prompt, output)
			}
		}

		elapsed := uint64(time.Since(start).Milliseconds())
		results = append(results, AgentExecResult{
			TaskName:     at.Name,
			Tool:         at.Tool,
			InputSummary: at.Description,
			Output:       output,
			DurationMs:   elapsed,
			Tokens:       tokens,
		})
	}

	return results, nil
}

func mockLLMResponse(prompt string) string {
	promptRunes := []rune(prompt)
	if len(promptRunes) > 100 {
		promptRunes = promptRunes[:100]
	}
	responses := []string{
		"已完成任务分析，结果如下：基于输入信息，可以得出以下结论...",
		"任务执行成功。关键发现：需要进一步调研相关内容。",
		"分析完成。主要结论：当前信息充分，可以继续进行下一步。",
		"搜索完成。找到以下相关信息：该领域的最新进展包括多个重要方向。",
	}
	return responses[rand.Intn(len(responses))]
}

// ── Parse Decision ──

func parseDecision(output string) *string {
	lower := strings.ToLower(output)
	keywords := []string{"通过", "充分", "enough", "yes", "不通过", "不充分", "not enough", "no", "tech", "general", "other"}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return &kw
		}
	}
	if strings.Contains(lower, "充足") || strings.Contains(lower, "足够") || strings.Contains(lower, "够") {
		s := "充分"
		return &s
	}
	if strings.Contains(lower, "不足") || strings.Contains(lower, "缺少") {
		s := "不充分"
		return &s
	}
	return nil
}

// ── Skip Downstream ──

func skipDownstream(tasks []AgentTask, start string, done map[string]bool, results *[]AgentExecResult) {
	if done[start] {
		return
	}
	done[start] = true
	*results = append(*results, AgentExecResult{
		TaskName: start,
		Tool:     "",
		Output:   "⏭️ 已跳过（决策未选中该路径）",
	})
	for _, t := range tasks {
		for _, d := range t.DependsOn {
			if d == start {
				skipDownstream(tasks, t.Name, done, results)
				break
			}
		}
	}
}

// ── Execute Batch Start ──

func (e *AgentEngine) ExecuteBatchStart(config ServiceConfig, task string, agentTasks []AgentTask, ragContext string, db *sql.DB) (string, []AgentExecResult, bool, error) {
	sid := uuid.New().String()
	done := make(map[string]bool)
	batch := readyBatch(agentTasks, done)
	if len(batch) == 0 {
		return "", nil, false, fmt.Errorf("没有可执行的任务")
	}

	results, err := runBatch(config, task, batch, nil, ragContext)
	if err != nil {
		return "", nil, false, err
	}

	var skippedResults []AgentExecResult
	for _, r := range results {
		if decided := parseDecision(r.Output); decided != nil {
			for _, t := range agentTasks {
				if t.Name == r.TaskName {
					for routeKey, nextTask := range t.Routes {
						if routeKey != *decided {
							skipDownstream(agentTasks, nextTask, done, &skippedResults)
						}
					}
				}
			}
		}
	}

	var reviewPending []AgentTask
	for _, t := range batch {
		if t.TaskType == "human_review" {
			reviewPending = append(reviewPending, t)
		}
	}

	completedNames := make(map[string]bool)
	for _, r := range results {
		isReview := false
		for _, p := range reviewPending {
			if p.Name == r.TaskName {
				isReview = true
				break
			}
		}
		if !isReview {
			completedNames[r.TaskName] = true
		}
	}
	var doneResults []AgentExecResult
	for _, r := range results {
		isReview := false
		for _, p := range reviewPending {
			if p.Name == r.TaskName {
				isReview = true
				break
			}
		}
		if !isReview {
			doneResults = append(doneResults, r)
		}
	}
	doneResults = append(doneResults, skippedResults...)

	if db != nil {
		ensureAgentTables(db)
		saveAgentSession(db, sid, task)
		for _, r := range doneResults {
			saveAgentResult(db, sid, &r)
		}
	}

	hasMore := len(reviewPending) == 0
	if hasMore {
		remaining := readyBatch(agentTasks, completedNames)
		hasMore = false
		for _, t := range remaining {
			if !completedNames[t.Name] {
				hasMore = true
				break
			}
		}
	}

	storeMu.Lock()
	store[sid] = &BatchState{
		Task:           task,
		All:            agentTasks,
		Done:           doneResults,
		CompletedNames: completedNames,
		Start:          time.Now(),
		RagContext:     ragContext,
		ReviewPending:  reviewPending,
	}
	storeMu.Unlock()

	return sid, results, hasMore, nil
}

// ── Execute Batch Next ──

func (e *AgentEngine) ExecuteBatchNext(config ServiceConfig, sid string, db *sql.DB) ([]AgentExecResult, bool, error) {
	storeMu.Lock()
	s, ok := store[sid]
	if !ok {
		storeMu.Unlock()
		return nil, false, fmt.Errorf("session不存在: %s", sid)
	}
	if len(s.ReviewPending) > 0 {
		storeMu.Unlock()
		return nil, false, fmt.Errorf("有待审批的人工审核任务，请先审批")
	}

	task := s.Task
	all := s.All
	completedNames := make(map[string]bool)
	for k, v := range s.CompletedNames {
		completedNames[k] = v
	}
	dones := make([]AgentExecResult, len(s.Done))
	copy(dones, s.Done)
	ragCtx := s.RagContext
	storeMu.Unlock()

	batch := readyBatch(all, completedNames)
	if len(batch) == 0 {
		return nil, false, fmt.Errorf("没有更多可执行任务")
	}

	results, err := runBatch(config, task, batch, dones, ragCtx)
	if err != nil {
		return nil, false, err
	}

	var newReviewPending []AgentTask
	for _, t := range batch {
		if t.TaskType == "human_review" {
			newReviewPending = append(newReviewPending, t)
		}
	}

	storeMu.Lock()
	if state, ok := store[sid]; ok {
		for _, r := range results {
			isReview := false
			for _, p := range newReviewPending {
				if p.Name == r.TaskName {
					isReview = true
					break
				}
			}
			if !isReview {
				state.Done = append(state.Done, r)
				state.CompletedNames[r.TaskName] = true
			}
		}

		for _, r := range results {
			if decided := parseDecision(r.Output); decided != nil {
				for _, t := range all {
					if t.Name == r.TaskName {
						for routeKey, nextTask := range t.Routes {
							if routeKey != *decided {
								skipDownstream(all, nextTask, state.CompletedNames, &state.Done)
							}
						}
					}
				}
			}
		}

		for _, t := range newReviewPending {
			alreadyExists := false
			for _, p := range state.ReviewPending {
				if p.Name == t.Name {
					alreadyExists = true
					break
				}
			}
			if !alreadyExists {
				state.ReviewPending = append(state.ReviewPending, t)
			}
		}

		hasMore := len(state.ReviewPending) == 0 && len(state.CompletedNames) < len(state.All)

		if db != nil {
			for _, r := range results {
				isReview := false
				for _, p := range newReviewPending {
					if p.Name == r.TaskName {
						isReview = true
						break
					}
				}
				if !isReview {
					saveAgentResult(db, sid, &r)
				}
			}
			if !hasMore && len(newReviewPending) == 0 {
				updateAgentSessionStatus(db, sid, "completed")
			}
		}

		storeMu.Unlock()
		return results, hasMore, nil
	}
	storeMu.Unlock()
	return results, false, nil
}

// ── Execute All Batches ──

func (e *AgentEngine) ExecuteAllBatches(config ServiceConfig, task string, agentTasks []AgentTask, ragContext string) (*AgentExecResponse, error) {
	totalStart := time.Now()

	done := make(map[string]bool)
	var allResults []AgentExecResult

	for len(done) < len(agentTasks) {
		batch := readyBatch(agentTasks, done)
		if len(batch) == 0 {
			break
		}

		results, err := runBatch(config, task, batch, allResults, ragContext)
		if err != nil {
			return nil, err
		}

		for _, r := range results {
			done[r.TaskName] = true
			allResults = append(allResults, r)

			if decided := parseDecision(r.Output); decided != nil {
				for _, t := range agentTasks {
					if t.Name == r.TaskName {
						for routeKey, nextTask := range t.Routes {
							if routeKey != *decided {
								skipDownstream(agentTasks, nextTask, done, &allResults)
							}
						}
					}
				}
			}
		}
	}

	totalDuration := uint64(time.Since(totalStart).Milliseconds())
	totalTokens := 0
	for _, r := range allResults {
		totalTokens += r.Tokens
	}

	finalAnswer := ""
	if len(allResults) > 0 {
		finalAnswer = allResults[len(allResults)-1].Output
	}

	return &AgentExecResponse{
		Results:         allResults,
		FinalAnswer:     finalAnswer,
		TotalDurationMs: totalDuration,
		TotalTokens:     totalTokens,
	}, nil
}

// ── Batch Finalize ──

func (e *AgentEngine) BatchFinalize(sid string, db *sql.DB) (*AgentExecResponse, error) {
	storeMu.Lock()
	state, ok := store[sid]
	if ok {
		delete(store, sid)
	}
	storeMu.Unlock()

	if !ok {
		return &AgentExecResponse{
			Results:     []AgentExecResult{},
			FinalAnswer: "完成",
		}, nil
	}

	if db != nil {
		updateAgentSessionStatus(db, sid, "completed")
	}

	finalAnswer := ""
	if len(state.Done) > 0 {
		finalAnswer = state.Done[len(state.Done)-1].Output
	}
	totalTokens := 0
	for _, r := range state.Done {
		totalTokens += r.Tokens
	}

	return &AgentExecResponse{
		Results:         state.Done,
		FinalAnswer:     finalAnswer,
		TotalDurationMs: uint64(time.Since(state.Start).Milliseconds()),
		TotalTokens:     totalTokens,
	}, nil
}

// ── Approve Review ──

func (e *AgentEngine) ApproveReview(sid, taskName string, approved bool, feedback string) error {
	storeMu.Lock()
	defer storeMu.Unlock()

	state, ok := store[sid]
	if !ok {
		return fmt.Errorf("session不存在: %s", sid)
	}

	idx := -1
	for i, t := range state.ReviewPending {
		if t.Name == taskName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("找不到待审批任务: %s", taskName)
	}

	task := state.ReviewPending[idx]
	state.ReviewPending = append(state.ReviewPending[:idx], state.ReviewPending[idx+1:]...)

	status := "✅ 人工审批通过"
	if !approved {
		status = "❌ 人工审批拒绝"
	}

	state.CompletedNames[taskName] = true
	state.Done = append(state.Done, AgentExecResult{
		TaskName:     taskName,
		Tool:         "human_review",
		InputSummary: task.Description,
		Output:       fmt.Sprintf("%s。反馈：%s", status, feedback),
	})

	return nil
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return s
}
