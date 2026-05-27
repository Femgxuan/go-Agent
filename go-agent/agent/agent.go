package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/memory"
	"github.com/fengxuan/go-agent/tools"
)

// AgentConfig holds configuration for the Agent.
type AgentConfig struct {
	MaxIterations int
	SystemPrompt  string
	Model         string
	MaxTokens     int  // working memory window size
	MemoryEnabled bool // whether to enable the memory system
}

// Agent is a ReAct-style agent that uses an LLM and tools.
type Agent struct {
	llm        client.LLMClient
	registry   *tools.Registry
	history    []client.Message
	mu         sync.Mutex
	config     AgentConfig
	callbacks  AgentCallbacks
	memory     memory.Manager
	classifier *memory.Classifier
}

// New creates a new Agent.
func New(llm client.LLMClient, registry *tools.Registry, config AgentConfig, mem memory.Manager, classifier *memory.Classifier) *Agent {
	if config.MaxIterations <= 0 {
		config.MaxIterations = 10
	}
	if config.MaxTokens <= 0 {
		config.MaxTokens = 8192
	}
	return &Agent{
		llm:        llm,
		registry:   registry,
		config:     config,
		memory:     mem,
		classifier: classifier,
	}
}

// SetCallbacks sets the optional callbacks for progress observation.
func (a *Agent) SetCallbacks(cb AgentCallbacks) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.callbacks = cb
}

// Reset replaces the agent's LLM client and config, clearing history.
func (a *Agent) Reset(llm client.LLMClient, config AgentConfig) {
	if config.MaxIterations <= 0 {
		config.MaxIterations = 10
	}
	if config.MaxTokens <= 0 {
		config.MaxTokens = 8192
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.llm = llm
	a.config = config
	a.history = nil
}

// History returns a copy of the conversation history.
func (a *Agent) History() []client.Message {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := make([]client.Message, len(a.history))
	copy(cp, a.history)
	return cp
}

// ClearHistory resets the conversation history.
func (a *Agent) ClearHistory() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.history = nil
}

// SetSystemPrompt updates the system prompt.
func (a *Agent) SetSystemPrompt(prompt string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config.SystemPrompt = prompt
}


// Run appends the user message to history and starts the ReAct loop.
// It returns a read-only channel of AgentEvents.
func (a *Agent) Run(ctx context.Context, input string) <-chan AgentEvent {
	ch := make(chan AgentEvent, 32)

	a.mu.Lock()
	a.history = append(a.history, client.Message{
		Role:    client.RoleUser,
		Content: input,
	})
	a.mu.Unlock()

	// Add user message to working memory synchronously so buildMessages() can see it.
	if a.config.MemoryEnabled && a.memory != nil {
		if err := a.memory.AddToWorkingMemory(memory.Message{
			Role:      "user",
			Content:   input,
			Timestamp: time.Now(),
		}); err != nil {
			slog.Warn("failed to add user message to working memory", "error", err)
		}
	}

	go func() {
		defer close(ch)
		a.runLoop(ctx, input, ch)
	}()

	return ch
}

// runLoop is the main ReAct iteration loop.
func (a *Agent) runLoop(ctx context.Context, input string, ch chan<- AgentEvent) {
	for iter := 0; iter < a.config.MaxIterations; iter++ {
		if a.runIteration(ctx, ch) {
			a.storeInteraction(ctx, input)
			return
		}
	}

	// Reached max iterations.
	select {
	case ch <- AgentEvent{
		Type:    EventError,
		Content: fmt.Sprintf("maximum iterations (%d) reached without final answer", a.config.MaxIterations),
	}:
	case <-ctx.Done():
	}
	a.storeInteraction(ctx, input)
}

// storeInteraction stores the user+assistant exchange in working memory and short-term memory.
func (a *Agent) storeInteraction(ctx context.Context, input string) {
	if !a.config.MemoryEnabled || a.memory == nil {
		return
	}
	a.mu.Lock()
	var lastAssistant string
	for i := len(a.history) - 1; i >= 0; i-- {
		if a.history[i].Role == client.RoleAssistant {
			lastAssistant = a.history[i].Content
			break
		}
	}
	a.mu.Unlock()
	if lastAssistant != "" {
		if err := a.memory.AddToWorkingMemory(memory.Message{
			Role:      "assistant",
			Content:   lastAssistant,
			Timestamp: time.Now(),
		}); err != nil {
			slog.Warn("failed to add assistant to working memory", "error", err)
		}
		go func() {
			if err := a.memory.Store(ctx, memory.Interaction{
				UserMsg:  input,
				AgentMsg: lastAssistant,
				Metadata: map[string]any{"timestamp": time.Now()},
			}); err != nil {
				slog.Warn("failed to store interaction", "error", err)
			}
		}()
	}

	// Rule pre-filter + LLM classification for long-term memory.
	if a.classifier != nil {
		if _, ok := extractMemoryFact(input); ok {
			go func() {
				classifyCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				if fact, ok := a.classifier.Classify(classifyCtx, input, lastAssistant); ok {
					if err := a.memory.Memorize(ctx, fact); err != nil {
						slog.Debug("failed to memorize classified fact", "error", err)
					}
				}
			}()
		}
	}
}

// runIteration executes one ReAct iteration (think → act → observe).
// Returns true if the agent loop should stop.
func (a *Agent) runIteration(ctx context.Context, ch chan<- AgentEvent) bool {
	if err := ctx.Err(); err != nil {
		return true
	}

	a.mu.Lock()
	cb := a.callbacks
	a.mu.Unlock()

	if cb != nil {
		cb.OnThinkingStart()
	}

	// Heartbeat: fires every 10s during long-running iterations.
	if cb != nil {
		heartbeatStop := make(chan struct{})
		defer close(heartbeatStop)
		iterStart := time.Now()
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					cb.OnHeartbeat(time.Since(iterStart))
				case <-heartbeatStop:
					return
				}
			}
		}()
	}

	// Build messages including system prompt.
	msgs := a.buildMessages("")

	req := client.ChatRequest{
		Model:    a.config.Model,
		Messages: msgs,
		Tools:    a.registry.ToolSchemas(),
	}

	streamCh, err := a.llm.ChatCompletion(ctx, req)
	if err != nil {
		select {
		case ch <- AgentEvent{Type: EventError, Content: err.Error()}:
		case <-ctx.Done():
		}
		return true
	}

	// Consume stream: accumulate content and tool calls.
	var fullContent string
	var toolCalls []client.ToolCall

	for chunk := range streamCh {
		if ctx.Err() != nil {
			return true
		}
		if chunk.Err != nil {
			select {
			case ch <- AgentEvent{Type: EventError, Content: chunk.Err.Error()}:
			case <-ctx.Done():
			}
			return true
		}
		if chunk.Delta != "" {
			fullContent += chunk.Delta
			if cb != nil {
				cb.OnStreamDelta(chunk.Delta)
			}
			select {
			case ch <- AgentEvent{Type: EventDelta, Content: chunk.Delta}:
			case <-ctx.Done():
				return true
			}
		}
		if len(chunk.ToolCalls) > 0 {
			toolCalls = append(toolCalls, chunk.ToolCalls...)
		}
	}

	if cb != nil {
		cb.OnThinkingEnd()
	}

	if len(toolCalls) == 0 {
		// No tool calls: this is the final answer.
		a.mu.Lock()
		a.history = append(a.history, client.Message{
			Role:    client.RoleAssistant,
			Content: fullContent,
		})
		a.mu.Unlock()

		select {
		case ch <- AgentEvent{Type: EventAnswer, Content: fullContent}:
		case <-ctx.Done():
		}
		return true
	}

	// Append assistant message with tool calls to history.
	a.mu.Lock()
	a.history = append(a.history, client.Message{
		Role:      client.RoleAssistant,
		Content:   fullContent,
		ToolCalls: toolCalls,
	})
	a.mu.Unlock()

	// Emit EventToolCall for each tool call.
	for _, tc := range toolCalls {
		var params map[string]any
		_ = json.Unmarshal([]byte(tc.Arguments), &params)
		select {
		case ch <- AgentEvent{
			Type:     EventToolCall,
			ToolName: tc.Name,
			ToolID:   tc.ID,
			Params:   params,
		}:
		case <-ctx.Done():
			return true
		}
	}

	// Execute tools concurrently, collect results in order.
	if cb != nil {
		for _, tc := range toolCalls {
			var params map[string]any
			_ = json.Unmarshal([]byte(tc.Arguments), &params)
			cb.OnToolStart(&ToolProgress{
				Name:   tc.Name,
				Args:   params,
				Status: "running",
			})
		}
	}

	results, err := a.executeTools(ctx, ch, toolCalls)
	if err != nil {
		if ctx.Err() != nil {
			return true
		}
		select {
		case ch <- AgentEvent{Type: EventError, Content: err.Error()}:
		case <-ctx.Done():
		}
		return true
	}

	if cb != nil {
		for i, tc := range toolCalls {
			cb.OnToolEnd(&ToolProgress{
				Name:   tc.Name,
				Status: "done",
			}, results[i])
		}
	}

	// Append tool results to history.
	a.mu.Lock()
	for i, tc := range toolCalls {
		a.history = append(a.history, client.Message{
			Role:       client.RoleTool,
			Content:    results[i],
			ToolCallID: tc.ID,
		})
	}
	a.mu.Unlock()

	return false // continue loop
}

// toolResult holds the result of a single tool execution keyed by index.
type toolResult struct {
	index  int
	output string
}

// executeTools runs all tool calls concurrently and returns results in order.
// It sends EventToolResult events for each result.
func (a *Agent) executeTools(ctx context.Context, ch chan<- AgentEvent, toolCalls []client.ToolCall) ([]string, error) {
	results := make([]string, len(toolCalls))
	resultCh := make(chan toolResult, len(toolCalls))

	eg, egCtx := errgroup.WithContext(ctx)

	for i, tc := range toolCalls {
		i, tc := i, tc // capture loop vars
		eg.Go(func() error {
			tool, ok := a.registry.Get(tc.Name)
			if !ok {
				resultCh <- toolResult{index: i, output: fmt.Sprintf("tool %q not found", tc.Name)}
				return nil
			}

			var params map[string]any
			if tc.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Arguments), &params); err != nil {
					resultCh <- toolResult{index: i, output: fmt.Sprintf("failed to parse arguments: %v", err)}
					return nil
				}
			}

			output, err := tool.Execute(egCtx, params)
			if err != nil {
				resultCh <- toolResult{index: i, output: fmt.Sprintf("tool error: %v", err)}
				return nil
			}
			if output == "" {
				output = "(no results)"
			}
			resultCh <- toolResult{index: i, output: output}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	close(resultCh)

	for r := range resultCh {
		results[r.index] = r.output
	}

	// Emit EventToolResult for each result in original order.
	for i, tc := range toolCalls {
		select {
		case ch <- AgentEvent{
			Type:     EventToolResult,
			ToolName: tc.Name,
			ToolID:   tc.ID,
			Content:  results[i],
		}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return results, nil
}

// buildMessages prepends the system prompt and memory context to a copy of the history.
func (a *Agent) buildMessages(userQuery string) []client.Message {
	a.mu.Lock()
	defer a.mu.Unlock()

	// If no query provided, extract from history.
	if userQuery == "" {
		for i := len(a.history) - 1; i >= 0; i-- {
			if a.history[i].Role == client.RoleUser {
				userQuery = a.history[i].Content
				break
			}
		}
	}

	var msgs []client.Message
	if a.config.SystemPrompt != "" {
		msgs = append(msgs, client.Message{
			Role:    client.RoleSystem,
			Content: a.config.SystemPrompt,
		})
	}

	if a.config.MemoryEnabled && a.memory != nil {
		ctx := context.Background()
		memCtx, err := a.memory.Retrieve(ctx, userQuery, memory.RetrieveOptions{
			MaxTokens: a.config.MaxTokens,
		})
		if err != nil {
			slog.Warn("failed to retrieve memory context", "error", err)
		} else {
			categoryLabels := map[memory.FactCategory]string{
				memory.FactPreference:  "用户偏好",
				memory.FactEnvironment: "环境信息",
				memory.FactCorrection:  "注意事项",
				memory.FactNorm:        "项目规范",
				memory.FactMilestone:   "历史记录",
				memory.FactExplicit:    "记忆",
			}
			for _, fact := range memCtx.RelevantFacts {
				label := categoryLabels[fact.Category]
				if label == "" {
					label = "相关记忆"
				}
				msgs = append(msgs, client.Message{
					Role:    client.RoleSystem,
					Content: fmt.Sprintf("[%s] %s: %s", label, fact.Key, fact.Content),
				})
			}
			for _, ref := range memCtx.SelfReflection {
				msgs = append(msgs, client.Message{
					Role:    client.RoleSystem,
					Content: fmt.Sprintf("[反思] %s", ref.Content),
				})
			}
		}
	}

	msgs = append(msgs, a.history...)
	return msgs
}

// extractMemoryFact performs rule-based pre-filtering on user input.
// Returns a candidate Fact with category hint if patterns match, or zero Fact and false otherwise.
// The candidate is NOT stored directly — it's passed to the LLM classifier for precise extraction.
func extractMemoryFact(input string) (memory.Fact, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return memory.Fact{}, false
	}

	type pattern struct {
		category memory.FactCategory
		keywords []string
	}

	patterns := []pattern{
		// Explicit requests (prefix match)
		{memory.FactExplicit, []string{"记住", "remember"}},
		// Preferences
		{memory.FactPreference, []string{
			"我喜欢", "我不喜欢", "我想要", "我偏好", "我的爱好", "我最爱",
			"I like", "I love", "I prefer", "my favorite", "my hobby",
		}},
		// Environment facts
		{memory.FactEnvironment, []string{
			"服务器是", "运行在", "部署在", "系统是", "数据库是",
			"running on", "deployed on", "server is", "database is",
		}},
		// Corrections
		{memory.FactCorrection, []string{
			"不要用", "别用", "不用", "请不要", "请别",
			"don't use", "stop using", "never use", "please don't",
		}},
		// Norms
		{memory.FactNorm, []string{
			"代码风格", "规范是", "约定是", "格式是", "编码规范",
			"coding style", "convention", "code format",
		}},
		// Milestones
		{memory.FactMilestone, []string{
			"完成了", "搞定了", "迁移了", "部署了", "上线了",
			"finished", "completed", "migrated", "deployed", "shipped",
		}},
	}

	lower := strings.ToLower(input)
	for _, p := range patterns {
		for _, kw := range p.keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return memory.Fact{
					ID:         fmt.Sprintf("fact_%d", time.Now().UnixNano()),
					Category:   p.category,
					Key:        string(p.category),
					Content:    input,
					Source:     "user",
					Confidence: 0.5, // rule-based, low confidence
					CreatedAt:  time.Now(),
					DecayScore: 1.0,
				}, true
			}
		}
	}

	return memory.Fact{}, false
}
