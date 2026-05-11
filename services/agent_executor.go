package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/atliliw/lanchaingo-agent/stores"
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

// ── LLM Helper ──

func newLLMClient(cfg ServiceConfig) *stores.LLMClient {
	return &stores.LLMClient{
		APIKey:  cfg.OpenAIAPIKey,
		BaseURL: cfg.OpenAIBaseURL,
		Model:   cfg.ChatModel,
	}
}

func callLLM(client *stores.LLMClient, prompt string) (string, error) {
	result, err := client.Invoke([]stores.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// ── Plan ──

func (e *AgentEngine) Plan(config ServiceConfig, task string, ragContext string, useRag bool, useRouting bool) (*AgentPlan, error) {
	llm := newLLMClient(config)

	toolsJSON, _ := json.Marshal(AvailableTools)
	tj := string(toolsJSON)

	routingSection := ""
	if useRouting {
		routingSection = `注意：如果任务需要根据中间结果决定下一步，必须创建 type=decision 的决策节点。
决策节点不需要 tool，通过 routes 定义走向（key=结果, value=下一个任务）。
routes 中的所有 value 必须在同一个任务列表中创建对应的子任务，每个子任务用 depends_on 指向决策节点。
如果任务需要人工审批才能继续，创建 type=human_review 的审核节点。
示例：调研Go和Python后，需要判断信息是否充分：
[
  {"name":"调研Go","tool":"rag_search","depends_on":[],"task_type":"normal","input_template":""},
  {"name":"调研Python","tool":"rag_search","depends_on":[],"task_type":"normal","input_template":""},
  {"name":"判断信息是否充分","task_type":"decision","depends_on":["调研Go","调研Python"],"routes":{"充分":"写对比","不充分":"补充搜索"},"input_template":""},
  {"name":"补充搜索","tool":"web_search","depends_on":["判断信息是否充分"],"task_type":"normal","input_template":""},
  {"name":"写对比","tool":"llm_query","depends_on":["判断信息是否充分"],"task_type":"normal","input_template":""}
]`
	}

	prompt := ""
	if useRag && ragContext != "" {
		prompt = fmt.Sprintf(
			"第一个子任务必须是「知识库检索」，使用 rag_search 工具。后续子任务基于知识库检索的结果执行。\n\n知识库检索到以下相关信息：\n%s\n%s\n将任务拆解为2-5个子任务并分配工具。\n要求：对比类任务必须将A和B拆成独立的搜索任务（depends_on为空），最终汇总任务depends_on所有搜索任务。\n可用工具：%s\n返回JSON：[{ \"name\": \"子任务名（中文）\", \"description\": \"做什么\", \"tool\": \"工具名\", \"task_type\": \"normal\", \"depends_on\": [\"前置\"], \"input_template\": \"需要什么\" }]\n任务：%s\n只返回JSON。",
			ragContext, routingSection, tj, task,
		)
	} else {
		prompt = fmt.Sprintf(
			"将任务拆解为2-5个子任务并分配工具。\n%s\n要求：对比类任务必须将A和B拆成独立的搜索任务（depends_on为空），最终汇总任务depends_on所有搜索任务。\n可用工具：%s\n返回JSON：[{ \"name\": \"子任务名（中文）\", \"description\": \"做什么\", \"tool\": \"工具名\", \"task_type\": \"normal\", \"depends_on\": [\"前置\"], \"input_template\": \"需要什么\" }]\n任务：%s\n只返回JSON。",
			routingSection, tj, task,
		)
	}

	content, err := callLLM(llm, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM规划失败: %w", err)
	}
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	tasks, parseErr := parseTasks(content)
	if parseErr != nil {
		fixed := fixJSON(content)
		tasks, parseErr = parseTasks(fixed)
		if parseErr != nil {
			fixPrompt := fmt.Sprintf("JSON 格式错误：%v\n\n原始内容：\n```\n%s\n```\n\n请修正为合法 JSON，只返回修正后的 JSON。", parseErr, content)
			fixResult, fixErr := callLLM(llm, fixPrompt)
			if fixErr == nil {
				fixed2 := strings.TrimSpace(fixResult)
				fixed2 = strings.TrimPrefix(fixed2, "```json")
				fixed2 = strings.TrimPrefix(fixed2, "```")
				fixed2 = strings.TrimSuffix(fixed2, "```")
				fixed2 = strings.TrimSpace(fixed2)
				tasks, parseErr = parseTasks(fixJSON(fixed2))
			}
		}
	}

	if parseErr != nil || len(tasks) == 0 {
		tasks = []AgentTask{
			{Name: "知识库检索", Description: "检索相关知识", Tool: "rag_search", DependsOn: []string{}, TaskType: "normal"},
			{Name: "信息分析", Description: "分析检索结果", Tool: "llm_query", DependsOn: []string{"知识库检索"}, TaskType: "normal"},
			{Name: "总结报告", Description: "生成总结报告", Tool: "llm_query", DependsOn: []string{"信息分析"}, TaskType: "normal"},
		}
	}

	gs := buildGraph(tasks)
	return &AgentPlan{
		OriginalTask:  task,
		Tasks:         tasks,
		GraphStructure: gs,
	}, nil
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
	llm := newLLMClient(config)
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
			result, err := callLLM(llm, prompt)
			if err != nil {
				output = fmt.Sprintf("决策失败: %v", err)
			} else {
				output = result
			}
			tokens = estimateTokenUsage(prompt, output)

		default:
			switch at.Tool {
			case "llm_query", "":
				prompt := fmt.Sprintf("任务：%s\n当前子任务：%s\n\n前置结果：\n%s\n\n请执行当前子任务并输出结果。",
					task, at.Description, ctxStr)
				result, err := callLLM(llm, prompt)
				if err != nil {
					output = fmt.Sprintf("执行失败: %v", err)
				} else {
					output = result
				}
				tokens = estimateTokenUsage(prompt, output)

			case "web_search":
				prompt := fmt.Sprintf("任务：%s\n当前子任务：%s\n\n请基于你的知识回答。",
					task, at.Description)
				result, err := callLLM(llm, prompt)
				if err != nil {
					output = fmt.Sprintf("搜索失败: %v", err)
				} else {
					output = result
				}
				tokens = estimateTokenUsage(prompt, output)

			case "weather":
				cityPrompt := fmt.Sprintf("任务：%s\n当前子任务：%s\n\n请输出要查询天气的城市名，不要多余内容。",
					task, at.Description)
				city, err := callLLM(llm, cityPrompt)
				if err != nil || strings.TrimSpace(city) == "" {
					city = at.Description
				}
				city = strings.TrimSpace(city)
				tool := NewWeatherTool()
				wResult, wErr := tool.Invoke(WeatherInput{City: city})
				if wErr != nil {
					output = fmt.Sprintf("天气查询失败: %v", wErr)
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
				result, err := callLLM(llm, prompt)
				if err != nil {
					output = fmt.Sprintf("执行失败: %v", err)
				} else {
					output = result
				}
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

// ── Execute All Batches ──

func (e *AgentEngine) ExecuteAllBatches(config ServiceConfig, task string, agentTasks []AgentTask, ragContext string) (*AgentExecResponse, error) {
	totalStart := time.Now()
	doneMap := make(map[string]bool)
	var allResults []AgentExecResult

	for len(doneMap) < len(agentTasks) {
		batch := readyBatch(agentTasks, doneMap)
		if len(batch) == 0 {
			break
		}
		results, err := runBatch(config, task, batch, allResults, ragContext)
		if err != nil {
			return nil, err
		}
		for _, r := range results {
			doneMap[r.TaskName] = true
			allResults = append(allResults, r)
			if decided := parseDecision(r.Output); decided != nil {
				for _, t := range agentTasks {
					if t.Name == r.TaskName {
						for routeKey, nextTask := range t.Routes {
							if routeKey != *decided {
								skipDownstream(agentTasks, nextTask, doneMap, &allResults)
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

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return s
}
