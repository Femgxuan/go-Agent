# Layered Rendering + Default Collapsed Design

## Goal

保持 TUI 面板干净：元信息面板（Thought、Action、Observation）默认折叠为一行摘要，用户可通过 Ctrl+O 全局切换展开/折叠。

## Architecture

基于现有 Bubbletea TUI 架构，修改 Panel 渲染逻辑，新增全局折叠状态。

## Design

### 1. Data Structure Changes

**`tui/viewport.go` — Panel struct:**
- Add `Collapsed bool` field
- Add `Collapsible bool` field (标识该面板是否可折叠)

**`tui/viewport.go` — MessageView struct:**
- Add `GlobalCollapsed bool` field (default: `true`)
- Add `ToggleGlobalCollapse()` method

### 2. Collapsed Summary Format

| Panel Type | Collapsed Summary |
|---|---|
| PanelThought | `▶ Thought (thinking...)` |
| PanelAction | `▶ Action: <toolName> → <params preview>` |
| PanelObservation | `▶ Observation → <first 50 chars>... (<N> chars)` |
| PanelUser | Always expanded |
| PanelAnswer | Always expanded |
| PanelError | Always expanded |

### 3. Rendering Logic

`renderPanel()` modification:
- If `GlobalCollapsed && panel.Collapsible` → render summary line
- Else → render full content (existing logic)

### 4. Keybinding

`Update()` addition:
- `Ctrl+O` → toggle `m.messageView.GlobalCollapsed`, call `syncViewport()` to refresh

### 5. New Panel Default State

When creating new panels in `handleAgentEvent()`:
- PanelThought, PanelAction, PanelObservation → `Collapsible: true`
- PanelUser, PanelAnswer, PanelError → `Collapsible: false`

## Files to Modify

- `tui/viewport.go` — Panel struct, MessageView struct, renderPanel()
- `tui/app.go` — handleAgentEvent(), Update() keybinding

## Testing

- 启动 TUI，发送消息触发工具调用
- 验证 Thought/Action/Observation 面板默认折叠为一行摘要
- 按 Ctrl+O 展开所有面板
- 再按 Ctrl+O 折叠所有面板
- User/Answer/Error 面板始终展开
