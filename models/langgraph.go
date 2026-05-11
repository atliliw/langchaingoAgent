package models

// LangGraphRequest — LangGraph demo request
type LangGraphRequest struct {
	Input string `json:"input"`
}

// LangGraphStructureRequest — GET graph structure request
type LangGraphStructureRequest struct {
	Mode string `json:"mode"`
}

// LangGraphStructureResponse — Graph structure with Mermaid
type LangGraphStructureResponse struct {
	Mode      string      `json:"mode"`
	Mermaid   string      `json:"mermaid"`
	Structure interface{} `json:"structure"`
}

// TaskDecomposeRequest — POST /api/langgraph/decompose
type TaskDecomposeRequest struct {
	Task string `json:"task"`
}

// SubTaskDef — Sub-task definition from LLM decomposition
type SubTaskDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	DependsOn   []string `json:"depends_on"`
}

// SubTaskExecResult — Sub-task execution result
type SubTaskExecResult struct {
	Name       string `json:"name"`
	Output     string `json:"output"`
	DurationMs uint64 `json:"duration_ms"`
	Tokens     int    `json:"tokens"`
}

// TaskDecomposeResult — Decomposition result
type TaskDecomposeResult struct {
	OriginalTask  string      `json:"original_task"`
	SubTasks      []SubTaskDef `json:"sub_tasks"`
	GraphStructure interface{} `json:"graph_structure"`
}

// TaskExecuteRequest — POST /api/langgraph/execute
type TaskExecuteRequest struct {
	Task     string       `json:"task"`
	SubTasks []SubTaskDef `json:"sub_tasks"`
}

// TaskExecuteResult — Sub-task execution response
type TaskExecuteResult struct {
	ExecutionResults []SubTaskExecResult `json:"execution_results"`
}

// ParallelDemoResult — Parallel execution demo result
type ParallelDemoResult struct {
	Input                   string              `json:"input"`
	ParallelTasks           []ParallelTaskResult `json:"parallel_tasks"`
	MergedResult            string              `json:"merged_result"`
	TotalTimeMs             uint64              `json:"total_time_ms"`
	SequentialTimeEstimateMs uint64             `json:"sequential_time_estimate_ms"`
	TimeSavedPercent        float32             `json:"time_saved_percent"`
}

// ParallelTaskResult — One parallel task result
type ParallelTaskResult struct {
	TaskName   string `json:"task_name"`
	Result     string `json:"result"`
	DurationMs uint64 `json:"duration_ms"`
}

// ConditionalDemoResult — Conditional routing demo result
type ConditionalDemoResult struct {
	Input         string   `json:"input"`
	RouteDecision string   `json:"route_decision"`
	PathTaken     string   `json:"path_taken"`
	Output        string   `json:"output"`
	Steps         []string `json:"steps"`
}

// StreamDemoEvent — Stream execution event
type StreamDemoEvent struct {
	NodeName     string         `json:"node_name"`
	EventType    string         `json:"event_type"`
	TimestampMs  uint64         `json:"timestamp_ms"`
	StateSnapshot *StateSnapshot `json:"state_snapshot,omitempty"`
}

// StateSnapshot — Execution state snapshot
type StateSnapshot struct {
	Input    string   `json:"input"`
	Output   *string  `json:"output,omitempty"`
	Messages []string `json:"messages"`
}

// SubgraphDemoResult — Subgraph demo result
type SubgraphDemoResult struct {
	Input            string `json:"input"`
	GeneratedContent string `json:"generated_content"`
	ReviewResult     string `json:"review_result"`
	TotalDurationMs  uint64 `json:"total_duration_ms"`
}

// LLMConditionalResult — LLM conditional routing result
type LLMConditionalResult struct {
	Input          string   `json:"input"`
	RouteTaken     string   `json:"route_taken"`
	Output         string   `json:"output"`
	Steps          []string `json:"steps"`
	TotalDurationMs uint64  `json:"total_duration_ms"`
}

// ToolDef — Tool definition
type ToolDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  []ToolParam `json:"parameters"`
}

// ToolParam — Tool parameter
type ToolParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// AgentTask — One task in agent planning
type AgentTask struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Tool         string            `json:"tool"`
	DependsOn    []string          `json:"depends_on"`
	InputTemplate string           `json:"input_template"`
	TaskType     string            `json:"task_type"`
	Routes       map[string]string `json:"routes"`
}

// AgentPlan — Agent plan result
type AgentPlan struct {
	OriginalTask  string      `json:"original_task"`
	Tasks         []AgentTask `json:"tasks"`
	GraphStructure interface{} `json:"graph_structure"`
}

// AgentExecResult — Agent execution result
type AgentExecResult struct {
	TaskName     string `json:"task_name"`
	Tool         string `json:"tool"`
	InputSummary string `json:"input_summary"`
	Output       string `json:"output"`
	DurationMs   uint64 `json:"duration_ms"`
	Tokens       int    `json:"tokens"`
}

// AgentExecResponse — Agent execution response
type AgentExecResponse struct {
	Results         []AgentExecResult `json:"results"`
	FinalAnswer     string            `json:"final_answer"`
	TotalDurationMs uint64            `json:"total_duration_ms"`
	TotalTokens     int               `json:"total_tokens"`
}

// AgentStepResult — Step-by-step execution result
type AgentStepResult struct {
	SessionID string         `json:"session_id"`
	Result    AgentExecResult `json:"result"`
	HasNext   bool           `json:"has_next"`
	IsFinal   bool           `json:"is_final"`
}

// AgentStepRequest — Continue agent execution request
type AgentStepRequest struct {
	SessionID string `json:"session_id"`
}
