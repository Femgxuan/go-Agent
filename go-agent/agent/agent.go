package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/tools"
)

// AgentConfig holds configuration for the Agent.
type AgentConfig struct {
	MaxIterations int
	SystemPrompt  string
	Model         string
}

// Agent is a ReAct-style agent that uses an LLM and tools.
type Agent struct {
	llm      client.LLMClient
	registry *tools.Registry
	history  []client.Message
	mu        sync.Mutex
	config    AgentConfig
	callbacks AgentCallbacks
}

// New creates a new Agent.
func New(llm client.LLMClient, registry *tools.Registry, config AgentConfig) *Agent {
	if config.MaxIterations <= 0 {
		config.MaxIterations = 10
	}
	return &Agent{
		llm:      llm,
		registry: registry,
		config:   config,
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

	go func() {
		defer close(ch)
		a.runLoop(ctx, ch)
	}()

	return ch
}

// runLoop is the main ReAct iteration loop.
func (a *Agent) runLoop(ctx context.Context, ch chan<- AgentEvent) {
	for iter := 0; iter < a.config.MaxIterations; iter++ {
		if a.runIteration(ctx, ch) {
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
	msgs := a.buildMessages()

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

// buildMessages prepends the system prompt to a copy of the history.
func (a *Agent) buildMessages() []client.Message {
	a.mu.Lock()
	defer a.mu.Unlock()

	var msgs []client.Message
	if a.config.SystemPrompt != "" {
		msgs = append(msgs, client.Message{
			Role:    client.RoleSystem,
			Content: a.config.SystemPrompt,
		})
	}
	msgs = append(msgs, a.history...)
	return msgs
}
