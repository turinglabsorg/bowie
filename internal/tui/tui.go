package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/turinglabs/bobby/internal/config"
	"github.com/turinglabs/bobby/internal/docker"
	"github.com/turinglabs/bobby/internal/task"
)

type mode int

const (
	modeList mode = iota
	modeChat
)

type lineKind int

const (
	lineText       lineKind = iota // normal text, always visible
	lineToolCall                   // [tool] name — always visible (compact)
	lineToolResult                 // full result — only visible when expanded
)

type chatLine struct {
	kind    lineKind
	text    string // rendered string for display
	toolIdx int    // groups tool_call + tool_result together
}

type agentMsg map[string]interface{}
type errMsg struct{ err error }

// attachMsg is sent when the background goroutine finishes starting/attaching a container.
type attachMsg struct {
	container *docker.Container
	task      *task.Task
	err       error
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	toolStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
)

type Model struct {
	mode            mode
	tasks           []*task.Task
	cursor          int
	viewport        viewport.Model
	input           textinput.Model
	lines           []chatLine
	showToolResults bool
	toolCounter     int
	status          string
	width           int
	height          int
	container       *docker.Container
	msgChan         chan map[string]interface{}
	ctx             context.Context
	cancel          context.CancelFunc
	taskID          string
	taskDesc        string
	err             string
	quitting        bool
	lastEnterEmpty  time.Time
	loading         bool
}

func NewListModel() Model {
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.CharLimit = 2000

	tasks, _ := task.List()

	return Model{
		mode:   modeList,
		tasks:  tasks,
		input:  ti,
		status: "idle",
	}
}

func NewChatModel(c *docker.Container, taskID string, description string) Model {
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.CharLimit = 2000
	ti.Focus()

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan map[string]interface{}, 64)
	go c.ReadMessages(ctx, ch)

	lines := []chatLine{
		{kind: lineText, text: titleStyle.Render("Task: " + description)},
		{kind: lineText, text: ""},
	}

	return Model{
		mode:      modeChat,
		input:     ti,
		lines:     lines,
		container: c,
		msgChan:   ch,
		ctx:       ctx,
		cancel:    cancel,
		taskID:    taskID,
		taskDesc:  description,
		status:    "idle",
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if m.mode == modeChat {
		cmds = append(cmds, m.waitForMsg())
	}
	return tea.Batch(cmds...)
}

func (m *Model) waitForMsg() tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-m.msgChan
		if !ok {
			return errMsg{fmt.Errorf("agent disconnected")}
		}
		return agentMsg(msg)
	}
}

// attachToTask starts/attaches a container for the given task in a background goroutine.
func attachToTask(t *task.Task) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		cli, err := docker.NewClient()
		if err != nil {
			return attachMsg{err: err}
		}
		defer cli.Close()

		containerID, running, err := docker.FindByTaskID(ctx, cli, t.ID)
		if err != nil {
			return attachMsg{err: err}
		}

		if running {
			c, err := docker.Attach(ctx, cli, containerID)
			return attachMsg{container: c, task: t, err: err}
		}

		// Not running — start it
		if err := docker.EnsureImage(ctx, cli); err != nil {
			return attachMsg{err: err}
		}

		llmCfg, err := config.LoadLLMConfig(t.Config)
		if err != nil {
			return attachMsg{err: fmt.Errorf("config %q: %w", t.Config, err)}
		}
		var mcpCfg *config.MCPConfig
		if t.MCP != "" {
			mcpCfg, err = config.LoadMCPConfig(t.MCP)
			if err != nil {
				return attachMsg{err: fmt.Errorf("mcp %q: %w", t.MCP, err)}
			}
		}

		c, err := docker.Start(ctx, cli, t, llmCfg, mcpCfg)
		return attachMsg{container: c, task: t, err: err}
	}
}

func (m *Model) switchToChat(c *docker.Container, t *task.Task) tea.Cmd {
	m.mode = modeChat
	m.loading = false
	m.taskID = t.ID
	m.taskDesc = t.Description
	m.status = "idle"
	m.lines = []chatLine{
		{kind: lineText, text: titleStyle.Render("Task: " + t.Description)},
		{kind: lineText, text: ""},
	}
	m.toolCounter = 0
	m.showToolResults = false
	m.container = c

	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel
	m.msgChan = make(chan map[string]interface{}, 64)
	go c.ReadMessages(ctx, m.msgChan)

	m.input.Focus()
	m.viewport = viewport.New(m.width, m.height-4)
	m.updateViewport()

	return m.waitForMsg()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "q":
			if m.mode == modeList {
				m.quitting = true
				return m, tea.Quit
			}
		case "ctrl+o":
			if m.mode == modeChat {
				m.showToolResults = !m.showToolResults
				m.updateViewport()
				return m, nil
			}
		case "enter":
			if m.mode == modeList && !m.loading {
				if len(m.tasks) > 0 && m.cursor < len(m.tasks) {
					m.loading = true
					t := m.tasks[m.cursor]
					return m, attachToTask(t)
				}
			}
			if m.mode == modeChat {
				text := m.input.Value()
				if text != "" {
					m.input.SetValue("")
					m.addText("")
					m.addText(fmt.Sprintf("You: %s", text))
					m.addText("")
					m.updateViewport()
					if m.container != nil {
						m.container.SendMessage(text)
					}
					m.lastEnterEmpty = time.Time{}
					return m, m.waitForMsg()
				}
				// Empty enter — double enter = interrupt
				now := time.Now()
				if !m.lastEnterEmpty.IsZero() && now.Sub(m.lastEnterEmpty) < 500*time.Millisecond {
					if m.container != nil {
						m.container.Send(map[string]interface{}{"type": "interrupt"})
					}
					m.lastEnterEmpty = time.Time{}
					return m, nil
				}
				m.lastEnterEmpty = now
			}
		case "esc":
			if m.mode == modeChat {
				m.mode = modeList
				m.tasks, _ = task.List()
				if m.cancel != nil {
					m.cancel()
				}
				m.container = nil
				m.msgChan = nil
				return m, nil
			}
		case "up", "k":
			if m.mode == modeList && m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.mode == modeList && m.cursor < len(m.tasks)-1 {
				m.cursor++
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport = viewport.New(msg.Width, msg.Height-4)
		m.updateViewport()

	case attachMsg:
		if msg.err != nil {
			m.loading = false
			m.err = msg.err.Error()
			return m, nil
		}
		cmd := m.switchToChat(msg.container, msg.task)
		return m, cmd

	case agentMsg:
		m.handleAgentMsg(msg)
		return m, m.waitForMsg()

	case errMsg:
		m.err = msg.err.Error()
		m.addText(errorStyle.Render("Error: " + m.err))
		m.updateViewport()
	}

	if m.mode == modeChat {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *Model) addText(s string) {
	m.lines = append(m.lines, chatLine{kind: lineText, text: s})
}

func (m *Model) handleAgentMsg(msg map[string]interface{}) {
	msgType, _ := msg["type"].(string)
	switch msgType {
	case "agent_response":
		content, _ := msg["content"].(string)
		m.addText("")
		m.addText(fmt.Sprintf("Bowie: %s", content))
		m.addText("")
	case "tool_call":
		tool, _ := msg["tool"].(string)
		m.toolCounter++
		m.lines = append(m.lines, chatLine{
			kind:    lineToolCall,
			text:    toolStyle.Render(fmt.Sprintf("  [tool] %s", tool)),
			toolIdx: m.toolCounter,
		})
	case "tool_result":
		tool, _ := msg["tool"].(string)
		content, _ := msg["content"].(string)
		m.lines = append(m.lines, chatLine{
			kind:    lineToolResult,
			text:    dimStyle.Render(fmt.Sprintf("         %s: %s", tool, content)),
			toolIdx: m.toolCounter,
		})
	case "status":
		state, _ := msg["state"].(string)
		m.status = state
	case "error":
		content, _ := msg["content"].(string)
		m.addText(errorStyle.Render("Error: " + content))
	}
	m.updateViewport()
}

func (m *Model) updateViewport() {
	w := m.width
	if w <= 0 {
		w = 80
	}
	var visible []string
	for _, l := range m.lines {
		if l.kind == lineToolResult && !m.showToolResults {
			continue
		}
		visible = append(visible, wrapLine(l.text, w)...)
	}
	m.viewport.SetContent(strings.Join(visible, "\n"))
	m.viewport.GotoBottom()
}

func wrapLine(s string, width int) []string {
	if width <= 0 || len(s) <= width {
		return []string{s}
	}
	var lines []string
	for len(s) > width {
		cut := width
		if i := strings.LastIndex(s[:cut], " "); i > 0 {
			cut = i + 1
		}
		lines = append(lines, s[:cut])
		s = s[cut:]
	}
	if len(s) > 0 {
		lines = append(lines, s)
	}
	return lines
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.mode == modeList {
		return m.viewList()
	}
	return m.viewChat()
}

func (m Model) viewList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Bowie — Tasks") + "\n\n")

	if len(m.tasks) == 0 {
		b.WriteString(dimStyle.Render("  No tasks. Use 'bowie new' to create one.") + "\n")
	}

	for i, t := range m.tasks {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.cursor {
			cursor = "> "
			style = selectedStyle
		}

		ts := taskTime(t.ID)
		status := task.ReadStatus(t.ID)

		id := shortID(t.ID)
		desc := truncate(t.Description, 50)

		line := fmt.Sprintf("%s%s  %s  %s  %s",
			cursor,
			style.Render(id),
			dimStyle.Render(ts),
			desc,
			dimStyle.Render(fmt.Sprintf("[%s]", status)),
		)
		b.WriteString(line + "\n")
	}

	b.WriteString("\n")
	if m.loading {
		b.WriteString(statusStyle.Render("  Starting container..."))
	} else if m.err != "" {
		b.WriteString(errorStyle.Render("  Error: " + m.err))
	} else {
		b.WriteString(dimStyle.Render("  enter: attach | q: quit"))
	}
	return b.String()
}

func (m Model) viewChat() string {
	statusLine := statusStyle.Render(fmt.Sprintf(" [%s] task:%s ", m.status, shortID(m.taskID)))
	hints := []string{}
	if m.showToolResults {
		hints = append(hints, "ctrl+o: hide details")
	} else {
		hints = append(hints, "ctrl+o: show details")
	}
	hints = append(hints, "enter enter: interrupt", "esc: back")
	toggleHint := dimStyle.Render(" " + strings.Join(hints, " | "))
	return fmt.Sprintf("%s %s\n%s\n%s\n%s",
		statusLine,
		toggleHint,
		m.viewport.View(),
		strings.Repeat("─", max(m.width, 40)),
		m.input.View(),
	)
}

// taskTime extracts the unix timestamp from a task ID (format: <timestamp>_<uuid>)
// and returns a human-readable relative time.
func taskTime(id string) string {
	parts := strings.SplitN(id, "_", 2)
	if len(parts) < 2 {
		return ""
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ""
	}
	t := time.Unix(ts, 0)
	return relativeTime(t)
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// shortID returns the uuid part of a task ID (after the timestamp).
func shortID(id string) string {
	parts := strings.SplitN(id, "_", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return id
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
