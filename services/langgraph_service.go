package services

import (
	"fmt"
	"time"
	"encoding/json"
)

type LangGraphDemoService struct{}

func NewLangGraphDemoService() *LangGraphDemoService {
	return &LangGraphDemoService{}
}

type ParallelDemoResult struct {
	Input                   string              `json:"input"`
	ParallelTasks           []ParallelTaskResult `json:"parallel_tasks"`
	MergedResult            string              `json:"merged_result"`
	TotalTimeMs             uint64              `json:"total_time_ms"`
	SequentialTimeEstimateMs uint64             `json:"sequential_time_estimate_ms"`
	TimeSavedPercent        float32             `json:"time_saved_percent"`
}

type ParallelTaskResult struct {
	TaskName   string `json:"task_name"`
	Result     string `json:"result"`
	DurationMs uint64 `json:"duration_ms"`
}

type ConditionalDemoResult struct {
	Input         string   `json:"input"`
	RouteDecision string   `json:"route_decision"`
	PathTaken     string   `json:"path_taken"`
	Output        string   `json:"output"`
	Steps         []string `json:"steps"`
}

type StreamDemoEvent struct {
	NodeName      string         `json:"node_name"`
	EventType     string         `json:"event_type"`
	TimestampMs   uint64         `json:"timestamp_ms"`
	StateSnapshot *StateSnapshot `json:"state_snapshot,omitempty"`
}

type StateSnapshot struct {
	Input    string   `json:"input"`
	Output   *string  `json:"output,omitempty"`
	Messages []string `json:"messages"`
}

type SubgraphDemoResult struct {
	Input            string `json:"input"`
	GeneratedContent string `json:"generated_content"`
	ReviewResult     string `json:"review_result"`
	TotalDurationMs  uint64 `json:"total_duration_ms"`
}

type LLMConditionalResult struct {
	Input          string   `json:"input"`
	RouteTaken     string   `json:"route_taken"`
	Output         string   `json:"output"`
	Steps          []string `json:"steps"`
	TotalDurationMs uint64  `json:"total_duration_ms"`
}

type SubTaskDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	DependsOn   []string `json:"depends_on"`
}

type SubTaskExecResult struct {
	Name       string `json:"name"`
	Output     string `json:"output"`
	DurationMs uint64 `json:"duration_ms"`
	Tokens     int    `json:"tokens"`
}

type TaskDecomposeResult struct {
	OriginalTask  string        `json:"original_task"`
	SubTasks      []SubTaskDef  `json:"sub_tasks"`
	GraphStructure interface{}  `json:"graph_structure"`
}

type TaskExecuteResult struct {
	ExecutionResults []SubTaskExecResult `json:"execution_results"`
}

type LangGraphStructureResponse struct {
	Mode      string      `json:"mode"`
	Mermaid   string      `json:"mermaid"`
	Structure interface{} `json:"structure"`
}

func (s *LangGraphDemoService) RunParallelDemo(input string) (*ParallelDemoResult, error) {
	start := time.Now()
	time.Sleep(200 * time.Millisecond)

	tasks := []ParallelTaskResult{
		{TaskName: "TaskA", Result: "TaskA: 数据已获取", DurationMs: 100},
		{TaskName: "TaskB", Result: "TaskB: 文档已处理", DurationMs: 150},
		{TaskName: "TaskC", Result: "TaskC: 分析已完成", DurationMs: 200},
	}

	totalMs := uint64(time.Since(start).Milliseconds())
	seqEstimate := uint64(450)
	saved := float32(seqEstimate-totalMs) / float32(seqEstimate) * 100

	return &ParallelDemoResult{
		Input:                   input,
		ParallelTasks:           tasks,
		MergedResult:            fmt.Sprintf("并行执行完成，耗时 %dms", totalMs),
		TotalTimeMs:             totalMs,
		SequentialTimeEstimateMs: seqEstimate,
		TimeSavedPercent:        saved,
	}, nil
}

func (s *LangGraphDemoService) RunConditionalDemo(input string) (*ConditionalDemoResult, error) {
	routeDecision := "long"
	pathTaken := "detailed_process"
	if len(input) < 10 {
		routeDecision = "short"
		pathTaken = "quick_process"
	}

	output := fmt.Sprintf("详细结果: %s (长度: %d)", input, len(input))
	if pathTaken == "quick_process" {
		output = fmt.Sprintf("快速结果: %s", input)
	}

	return &ConditionalDemoResult{
		Input:         input,
		RouteDecision: routeDecision,
		PathTaken:     pathTaken,
		Output:        output,
		Steps:         []string{"分析输入: " + input, output},
	}, nil
}

func (s *LangGraphDemoService) RunStreamDemo(input string) ([]StreamDemoEvent, error) {
	start := time.Now()
	steps := []string{"step1", "step2", "step3"}
	var events []StreamDemoEvent

	events = append(events, StreamDemoEvent{
		NodeName: "START", EventType: "graph_start", TimestampMs: 0,
	})

	for _, step := range steps {
		events = append(events, StreamDemoEvent{
			NodeName: step, EventType: "enter", TimestampMs: uint64(time.Since(start).Milliseconds()),
		})
		time.Sleep(50 * time.Millisecond)
		output := step + "完成"
		msg := output
		events = append(events, StreamDemoEvent{
			NodeName: step, EventType: "complete", TimestampMs: uint64(time.Since(start).Milliseconds()),
			StateSnapshot: &StateSnapshot{
				Input:    input,
				Output:   &output,
				Messages: []string{msg},
			},
		})
	}

	events = append(events, StreamDemoEvent{
		NodeName: "END", EventType: "graph_end", TimestampMs: uint64(time.Since(start).Milliseconds()),
	})
	return events, nil
}

func (s *LangGraphDemoService) GetGraphInfo() interface{} {
	return map[string]interface{}{
		"parallel_demo": map[string]interface{}{
			"name":        "并行执行演示",
			"description": "FanOut → 3个并行任务 → FanIn",
			"nodes":       []string{"dispatcher", "task_a", "task_b", "task_c"},
			"edges":       []string{"START → dispatcher", "dispatcher → [task_a, task_b, task_c] (FanOut)", "task_a → END", "task_b → END", "task_c → END"},
			"features":    []string{"add_fan_out", "add_async_node"},
		},
		"conditional_demo": map[string]interface{}{
			"name":        "条件路由演示",
			"description": "根据输入长度选择路径",
			"nodes":       []string{"analyze", "quick_process", "detailed_process"},
			"edges":       []string{"START → analyze", "analyze → quick_process (条件: 长度<10)", "analyze → detailed_process (条件: 长度>=10)"},
			"features":    []string{"add_conditional_edges", "FunctionRouter"},
		},
		"stream_demo": map[string]interface{}{
			"name":        "流式执行演示",
			"description": "展示执行事件流",
			"nodes":       []string{"step1", "step2", "step3"},
			"edges":       []string{"START → step1 → step2 → step3 → END"},
			"features":    []string{"stream()", "StreamEvent"},
		},
	}
}

func (s *LangGraphDemoService) GetGraphStructure(mode string) (*LangGraphStructureResponse, error) {
	mermaid := ""
	structure := map[string]interface{}{"nodes": []string{}, "edges": []string{}}

	switch mode {
	case "parallel":
		mermaid = "graph TD\n  START(\"START\")\n  END[\"END\"]\n  dispatcher[\"dispatcher\"]\n  task_a[\"task_a\"]\n  task_b[\"task_b\"]\n  task_c[\"task_c\"]\n  START --> dispatcher\n  dispatcher --> task_a\n  dispatcher --> task_b\n  dispatcher --> task_c\n  task_a --> END\n  task_b --> END\n  task_c --> END"
		structure = map[string]interface{}{
			"nodes": []string{"dispatcher", "task_a", "task_b", "task_c"},
			"edges": []map[string]interface{}{
				{"source": "__start__", "target": "dispatcher", "type": "fixed"},
				{"source": "dispatcher", "targets": []string{"task_a", "task_b", "task_c"}, "type": "fanout"},
			},
		}
	case "conditional":
		mermaid = "graph TD\n  START(\"START\")\n  END[\"END\"]\n  analyze[\"analyze\"]\n  quick_process[\"quick_process\"]\n  detailed_process[\"detailed_process\"]\n  START --> analyze\n  analyze -- \"short\" --> quick_process\n  analyze -- \"long\" --> detailed_process\n  quick_process --> END\n  detailed_process --> END"
		structure = map[string]interface{}{
			"nodes": []string{"analyze", "quick_process", "detailed_process"},
			"edges": []map[string]interface{}{
				{"source": "__start__", "target": "analyze", "type": "fixed"},
				{"source": "analyze", "type": "conditional", "targets": map[string]string{"short": "quick_process", "long": "detailed_process"}},
			},
		}
	case "stream":
		mermaid = "graph TD\n  START(\"START\")\n  END[\"END\"]\n  step1[\"step1\"]\n  step2[\"step2\"]\n  step3[\"step3\"]\n  START --> step1\n  step1 --> step2\n  step2 --> step3\n  step3 --> END"
		structure = map[string]interface{}{
			"nodes": []string{"step1", "step2", "step3"},
			"edges": []map[string]interface{}{
				{"source": "__start__", "target": "step1", "type": "fixed"},
				{"source": "step1", "target": "step2", "type": "fixed"},
				{"source": "step2", "target": "step3", "type": "fixed"},
			},
		}
	default:
		return nil, fmt.Errorf("unknown mode: %s", mode)
	}

	return &LangGraphStructureResponse{
		Mode:      mode,
		Mermaid:   mermaid,
		Structure: structure,
	}, nil
}

func (s *LangGraphDemoService) DecomposeTask(task string) (*TaskDecomposeResult, error) {
	subTasks := []SubTaskDef{
		{Name: "需求分析", Description: "分析用户任务需求", DependsOn: []string{}},
		{Name: "信息收集", Description: "收集相关信息", DependsOn: []string{"需求分析"}},
		{Name: "方案设计", Description: "设计方案", DependsOn: []string{"信息收集"}},
		{Name: "执行实施", Description: "实施方案", DependsOn: []string{"方案设计"}},
		{Name: "结果验证", Description: "验证执行结果", DependsOn: []string{"执行实施"}},
	}

	graphStructure := map[string]interface{}{
		"nodes": []string{"需求分析", "信息收集", "方案设计", "执行实施", "结果验证"},
		"edges": []map[string]string{
			{"source": "__start__", "target": "需求分析"},
			{"source": "需求分析", "target": "信息收集"},
			{"source": "信息收集", "target": "方案设计"},
			{"source": "方案设计", "target": "执行实施"},
			{"source": "执行实施", "target": "结果验证"},
		},
	}

	return &TaskDecomposeResult{
		OriginalTask:  task,
		SubTasks:      subTasks,
		GraphStructure: graphStructure,
	}, nil
}

func (s *LangGraphDemoService) ExecuteSubTasks(task string, subTasks []SubTaskDef) (*TaskExecuteResult, error) {
	var results []SubTaskExecResult
	for _, st := range subTasks {
		results = append(results, SubTaskExecResult{
			Name:   st.Name,
			Output: fmt.Sprintf("【%s】执行完成：基于任务「%s」的%s。结果：✓", st.Name, task, st.Description),
			DurationMs: 100,
			Tokens: 50,
		})
	}
	return &TaskExecuteResult{ExecutionResults: results}, nil
}

func (s *LangGraphDemoService) RunSubgraphDemo(input string) (*SubgraphDemoResult, error) {
	content := fmt.Sprintf("这是一篇关于「%s」的文章...", input)
	return &SubgraphDemoResult{
		Input:            input,
		GeneratedContent: content,
		ReviewResult:     fmt.Sprintf("审核结果：审核通过：内容长度 %d 字，符合要求 | 决定：通过", len([]rune(content))),
		TotalDurationMs:  300,
	}, nil
}

func (s *LangGraphDemoService) RunLLMConditionalDemo(input string) (*LLMConditionalResult, error) {
	route := "general"
	if len(input) > 20 {
		route = "tech"
	}

	output := fmt.Sprintf("【%s回答】%s", route, input)
	return &LLMConditionalResult{
		Input:          input,
		RouteTaken:     route,
		Output:         output,
		Steps:          []string{fmt.Sprintf("分析输入：%s", input), output},
		TotalDurationMs: 150,
	}, nil
}

func (s *LangGraphDemoService) AgentPlan(api *ApiService, task string, useRag bool, useRouting bool) (*AgentPlan, error) {
	ragCtx := ""
	if useRag {
		ragCtx = api.SearchKnowledgeBase(task, 5)
	}
	_ = ragCtx

	tasks := []AgentTask{
		{
			Name: "知识库检索", Description: "检索相关知识", Tool: "rag_search",
			DependsOn: []string{}, TaskType: "normal",
		},
		{
			Name: "信息分析", Description: "分析检索结果", Tool: "llm_query",
			DependsOn: []string{"知识库检索"}, TaskType: "normal",
		},
		{
			Name: "总结报告", Description: "生成总结报告", Tool: "llm_query",
			DependsOn: []string{"信息分析"}, TaskType: "normal",
		},
	}

	graphStructure := map[string]interface{}{
		"nodes": []string{"知识库检索", "信息分析", "总结报告"},
		"edges": []map[string]string{
			{"source": "__start__", "target": "知识库检索"},
			{"source": "知识库检索", "target": "信息分析"},
			{"source": "信息分析", "target": "总结报告"},
		},
	}

	return &AgentPlan{
		OriginalTask:  task,
		Tasks:         tasks,
		GraphStructure: graphStructure,
	}, nil
}

func (s *LangGraphDemoService) AgentExecuteAll(api *ApiService, task string, tasks []AgentTask, useRag bool) (*AgentExecResponse, error) {
	var results []AgentExecResult
	for _, t := range tasks {
		ragCtx := ""
		if useRag && t.Tool == "rag_search" {
			ragCtx = api.SearchKnowledgeBase(task, 3)
		}
		output := fmt.Sprintf("【%s】执行完成。%s", t.Name, ragCtx)
		if t.Tool == "llm_query" || t.Tool == "" {
			output = fmt.Sprintf("基于任务「%s」的分析结果：%s 已完成。", task, t.Description)
		}

		results = append(results, AgentExecResult{
			TaskName: t.Name, Tool: t.Tool, InputSummary: t.Description,
			Output: output, DurationMs: 100, Tokens: 50,
		})
	}

	fa := ""
	if len(results) > 0 {
		fa = results[len(results)-1].Output
	}

	return &AgentExecResponse{
		Results:         results,
		FinalAnswer:     fa,
		TotalDurationMs: uint64(len(results)) * 100,
		TotalTokens:     len(results) * 50,
	}, nil
}

func (s *LangGraphDemoService) DecomposeTaskStr(task string) (*TaskDecomposeResult, error) {
	subTasks := []SubTaskDef{
		{Name: "需求分析", Description: "分析用户任务需求", DependsOn: []string{}},
		{Name: "信息收集", Description: "收集相关信息", DependsOn: []string{"需求分析"}},
		{Name: "方案设计", Description: "设计方案", DependsOn: []string{"信息收集"}},
		{Name: "执行实施", Description: "实施方案", DependsOn: []string{"方案设计"}},
		{Name: "结果验证", Description: "验证执行结果", DependsOn: []string{"执行实施"}},
	}
	return &TaskDecomposeResult{OriginalTask: task, SubTasks: subTasks, GraphStructure: json.RawMessage(`{"nodes":["需求分析","信息收集","方案设计","执行实施","结果验证"],"edges":[{"source":"__start__","target":"需求分析"},{"source":"需求分析","target":"信息收集"},{"source":"信息收集","target":"方案设计"},{"source":"方案设计","target":"执行实施"},{"source":"执行实施","target":"结果验证"}]}`)}, nil
}
