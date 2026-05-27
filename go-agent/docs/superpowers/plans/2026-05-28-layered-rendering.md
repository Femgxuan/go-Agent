# Layered Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 元信息面板默认折叠为一行摘要，Ctrl+O 全局切换展开/折叠。

**Architecture:** 修改现有 Panel 结构体增加 Collapsible 字段，MessageView 增加全局折叠状态，renderPanel() 根据折叠状态渲染摘要或完整内容。

**Tech Stack:** Bubbletea, lipgloss (已有依赖)

---

### Task 1: Panel 结构体增加 Collapsible 字段

**Files:**
- Modify: `tui/viewport.go:22-27`

- [ ] **Step 1: 修改 Panel 结构体**

```go
// Panel represents a single message panel in the viewport.
type Panel struct {
	Type       PanelType
	Title      string
	Content    string
	Collapsed  bool
	Collapsible bool // 是否可折叠
}
```

- [ ] **Step 2: 修改 ToggleCollapse 方法**

将 `tui/viewport.go:70-79` 的 `ToggleCollapse` 方法改为检查 `Collapsible` 而非硬编码类型：

```go
func (m *MessageView) ToggleCollapse(index int) {
	if index < 0 || index >= len(m.panels) {
		return
	}
	p := &m.panels[index]
	if !p.Collapsible {
		return
	}
	p.Collapsed = !p.Collapsed
}
```

- [ ] **Step 3: 验证编译**

Run: `go build ./tui/...`
Expected: 编译通过

- [ ] **Step 4: Commit**

```bash
git add tui/viewport.go
git commit -m "feat(tui): add Collapsible field to Panel struct"
```

---

### Task 2: MessageView 增加全局折叠状态

**Files:**
- Modify: `tui/viewport.go:29-37`

- [ ] **Step 1: 修改 MessageView 结构体**

```go
// MessageView manages a list of panels.
type MessageView struct {
	panels          []Panel
	globalCollapsed bool // 全局折叠状态，默认 true
}
```

- [ ] **Step 2: 修改 NewMessageView**

```go
func NewMessageView() *MessageView {
	return &MessageView{globalCollapsed: true}
}
```

- [ ] **Step 3: 添加 ToggleGlobalCollapse 方法**

在 `tui/viewport.go` 的 `Clear()` 方法之前添加：

```go
// ToggleGlobalCollapse toggles the global collapsed state.
func (m *MessageView) ToggleGlobalCollapse() {
	m.globalCollapsed = !m.globalCollapsed
}
```

- [ ] **Step 4: 验证编译**

Run: `go build ./tui/...`
Expected: 编译通过

- [ ] **Step 5: Commit**

```bash
git add tui/viewport.go
git commit -m "feat(tui): add global collapsed state to MessageView"
```

---

### Task 3: renderPanel() 支持折叠摘要

**Files:**
- Modify: `tui/viewport.go:91-101` (Render 方法)
- Modify: `tui/viewport.go:103-156` (renderPanel 函数)

- [ ] **Step 1: 修改 Render 方法传递 globalCollapsed**

```go
func (m *MessageView) Render(width int) string {
	if len(m.panels) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, p := range m.panels {
		sb.WriteString(renderPanel(p, width, m.globalCollapsed))
	}
	return sb.String()
}
```

- [ ] **Step 2: 修改 renderPanel 函数签名和逻辑**

```go
func renderPanel(p Panel, width int, globalCollapsed bool) string {
	// 判断是否处于折叠状态：全局折叠且面板可折叠
	collapsed := globalCollapsed && p.Collapsible

	switch p.Type {
	case PanelUser:
		return "\n  " + userStyle.Render("You:") + " " + p.Content + "\n"

	case PanelThought:
		if collapsed {
			header := thoughtStyle.Render("▸ Thought ") + dimStyle.Render(strings.Repeat("─", max(0, width-12)))
			return "\n" + header + "\n"
		}
		arrow := "▾"
		header := thoughtStyle.Render(arrow+" Thought ") + dimStyle.Render(strings.Repeat("─", max(0, width-12)))
		return "\n" + header + "\n" + indent(p.Content, 4) + "\n"

	case PanelAction:
		title := p.Title
		if title == "" {
			title = "Action"
		}
		if collapsed {
			// 折叠时显示工具名和参数预览
			preview := truncatePreview(p.Content, 50)
			headerText := "▸ Action: " + title
			if preview != "" {
				headerText += " → " + preview
			}
			headerText += " "
			header := actionStyle.Render(headerText) + dimStyle.Render(strings.Repeat("─", max(0, width-lipgloss.Width(headerText)-2)))
			return "\n" + header + "\n"
		}
		arrow := "▾"
		headerText := arrow + " Action: " + title + " "
		header := actionStyle.Render(headerText) + dimStyle.Render(strings.Repeat("─", max(0, width-lipgloss.Width(headerText)-2)))
		return "\n" + header + "\n" + indent(p.Content, 4) + "\n"

	case PanelObservation:
		if collapsed {
			// 折叠时显示内容预览和长度
			preview := truncatePreview(p.Content, 50)
			headerText := fmt.Sprintf("▸ Observation → %s (%d chars) ", preview, len(p.Content))
			header := observationStyle.Render(headerText) + dimStyle.Render(strings.Repeat("─", max(0, width-lipgloss.Width(headerText)-2)))
			return "\n" + header + "\n"
		}
		arrow := "▾"
		header := observationStyle.Render(arrow+" Observation ") + dimStyle.Render(strings.Repeat("─", max(0, width-16)))
		return "\n" + header + "\n" + indent(p.Content, 4) + "\n"

	case PanelAnswer:
		return "\n  " + answerStyle.Render("Assistant:") + "\n    " + wrapText(p.Content, width-4, "    ") + "\n"

	case PanelError:
		return "\n  " + errorStyle.Render("Error: "+p.Content) + "\n"

	default:
		return ""
	}
}
```

- [ ] **Step 3: 添加 truncatePreview 辅助函数**

在 `tui/viewport.go` 文件末尾（`max` 函数之后）添加：

```go
// truncatePreview returns a truncated preview of text.
func truncatePreview(text string, maxLen int) string {
	// 移除换行和多余空格，取单行预览
	preview := strings.ReplaceAll(text, "\n", " ")
	preview = strings.TrimSpace(preview)
	if len(preview) > maxLen {
		return preview[:maxLen] + "..."
	}
	return preview
}
```

- [ ] **Step 4: 验证编译**

Run: `go build ./tui/...`
Expected: 编译通过

- [ ] **Step 5: Commit**

```bash
git add tui/viewport.go
git commit -m "feat(tui): render collapsed summary for Thought/Action/Observation panels"
```

---

### Task 4: handleAgentEvent() 设置 Collapsible 标志

**Files:**
- Modify: `tui/app.go:322-385`

- [ ] **Step 1: 修改面板创建，设置 Collapsible**

在 `handleAgentEvent()` 中，修改各 case 的 `AddPanel` 调用：

```go
case agent.EventThought:
	m.state = StateThinking
	m.messageView.AddPanel(Panel{
		Type:        PanelThought,
		Title:       "Thought",
		Content:     ev.Content,
		Collapsible: true,
	})

case agent.EventToolCall:
	m.state = StateExecuting
	params := formatParams(ev.Params)
	m.messageView.AddPanel(Panel{
		Type:        PanelAction,
		Title:       ev.ToolName,
		Content:     params,
		Collapsible: true,
	})

case agent.EventToolResult:
	m.state = StateThinking
	content := ev.Content
	if len(content) > 500 {
		content = content[:500] + "... [truncated]"
	}
	m.messageView.AddPanel(Panel{
		Type:        PanelObservation,
		Content:     content,
		Collapsible: true,
	})
```

PanelUser、PanelAnswer、PanelError 不设置 `Collapsible`（默认 `false`），保持始终展开。

- [ ] **Step 2: 验证编译**

Run: `go build ./tui/...`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add tui/app.go
git commit -m "feat(tui): set Collapsible flag on Thought/Action/Observation panels"
```

---

### Task 5: 绑定 Ctrl+O 快捷键

**Files:**
- Modify: `tui/app.go:123-130` (tea.KeyMsg 处理)

- [ ] **Step 1: 在 tea.KeyMsg switch 中添加 Ctrl+O 处理**

在 `case tea.KeyMsg:` 的 `switch msg.Type` 中，在 `case tea.KeyCtrlC:` 之前添加：

```go
case tea.KeyCtrlO:
	m.messageView.ToggleGlobalCollapse()
	m.syncViewport()
	return m, nil
```

- [ ] **Step 2: 验证编译**

Run: `go build ./tui/...`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add tui/app.go
git commit -m "feat(tui): bind Ctrl+O to toggle global collapse"
```

---

### Task 6: 端到端测试

- [ ] **Step 1: 运行完整测试套件**

Run: `go test ./... -short -count=1`
Expected: 所有测试通过

- [ ] **Step 2: 手动验证**

启动 TUI，执行以下操作：
1. 发送一个需要工具调用的消息（如"搜索Go语言特点"）
2. 验证 Thought 面板显示为 `▸ Thought ───`
3. 验证 Action 面板显示为 `▸ Action: tavily_search → {"query":...}`
4. 验证 Observation 面板显示为 `▸ Observation → <preview> (<N> chars)`
5. 验证 Answer 面板始终展开
6. 按 Ctrl+O，所有面板展开显示完整内容
7. 再按 Ctrl+O，所有面板恢复折叠

- [ ] **Step 3: Final commit**

```bash
git add -A
git commit -m "feat(tui): layered rendering with default collapsed panels"
```
