package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"node_messager/internal/entities"
	"node_messager/pkg/dto"
	"node_messager/pkg/logbuffer"
	"node_messager/pkg/msgstore"
	"node_messager/pkg/node"
	"node_messager/pkg/wsclient"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── connection pool ───────────────────────────────────────────────────────────

type connPool struct {
	mu    sync.Mutex
	conns map[int]*wsclient.Client
}

func newConnPool() *connPool {
	return &connPool{conns: make(map[int]*wsclient.Client)}
}

func (p *connPool) get(n node.Node) (*wsclient.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.conns[n.ID]; ok && !c.IsClosed() {
		return c, nil
	}
	c, err := wsclient.Connect(n.Host, n.Port)
	if err != nil {
		return nil, err
	}
	p.conns[n.ID] = c
	return c, nil
}

// ── messages ──────────────────────────────────────────────────────────────────

type tickMsg time.Time
type sendResultMsg struct{ err error }

// ── states ────────────────────────────────────────────────────────────────────

type appState int

const (
	stateMenu appState = iota
	stateSelectFrom
	stateSelectTo
	stateInputMsg
	stateSelectLogNode
	stateResult
)

type menuAction int

const (
	actionSend menuAction = iota
	actionBroadcast
	actionListNodes
	actionViewLogs
)

// ── styles ────────────────────────────────────────────────────────────────────

var (
	borderColor   = lipgloss.Color("62")
	dimColor      = lipgloss.Color("241")
	selectedColor = lipgloss.Color("212")
	successColor  = lipgloss.Color("78")
	errorColor    = lipgloss.Color("196")

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(borderColor).
			MarginBottom(1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(selectedColor).
			Bold(true)

	dimStyle     = lipgloss.NewStyle().Foreground(dimColor)
	successStyle = lipgloss.NewStyle().Foreground(successColor)
	errorStyle   = lipgloss.NewStyle().Foreground(errorColor)
)

type model struct {
	choices []string
	cursor  int
	state   appState
	action  menuAction

	nodes    []node.Node
	fromNode node.Node
	toNode   node.Node
	hostNode *node.Node

	inputMsg string
	inputErr string

	result    string
	resultErr bool

	width  int
	height int

	logBuffer *logbuffer.Buffer
	logs      []string
	stores    map[int]*msgstore.Store
	pool      *connPool
}

func initialModel(buf *logbuffer.Buffer, nodes []node.Node, stores map[int]*msgstore.Store, hostNode *node.Node) model {
	return model{
		choices:   []string{"Send a message", "Broadcast a message", "View node logs", "List all nodes", "Quit"},
		state:     stateMenu,
		logBuffer: buf,
		nodes:     nodes,
		stores:    stores,
		hostNode:  hostNode,
		pool:      newConnPool(),
	}
}

// targets returns nodes excluding from — used for the TO selection list.
func (m model) targets() []node.Node {
	out := make([]node.Node, 0, len(m.nodes)-1)
	for _, n := range m.nodes {
		if n.ID != m.fromNode.ID {
			out = append(out, n)
		}
	}
	return out
}

func tickCmd() tea.Cmd {
	return tea.Every(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func sendMsgCmd(from, to node.Node, content string, stores map[int]*msgstore.Store, pool *connPool) tea.Cmd {
	return func() tea.Msg {
		c, err := pool.get(to)
		if err != nil {
			return sendResultMsg{err: err}
		}
		m := dto.Message{
			ID:       uuid.New().String(),
			Type:     string(entities.MSG),
			FromNode: from.Name,
			ToNode:   to.Name,
			Content:  content,
			SendAt:   time.Now().UTC().Format(time.RFC3339),
		}
		data, err := json.Marshal(m)
		if err != nil {
			return sendResultMsg{err: err}
		}
		if err := c.Send(data); err != nil {
			return sendResultMsg{err: err}
		}
		if s, ok := stores[from.ID]; ok {
			s.Save(m, msgstore.Sent) //nolint:errcheck
		}
		if s, ok := stores[to.ID]; ok {
			s.Save(m, msgstore.Received) //nolint:errcheck
		}
		return sendResultMsg{}
	}
}

func broadcastCmd(from node.Node, nodes []node.Node, content string, stores map[int]*msgstore.Store, pool *connPool) tea.Cmd {
	return func() tea.Msg {
		id := uuid.New().String()
		now := time.Now().UTC().Format(time.RFC3339)
		var errs []string
		for _, n := range nodes {
			c, err := pool.get(n)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", n.Name, err))
				continue
			}
			m := dto.Message{
				ID:       id,
				Type:     string(entities.BROADCAST),
				FromNode: from.Name,
				ToNode:   n.Name,
				Content:  content,
				SendAt:   now,
			}
			data, _ := json.Marshal(m)
			if err := c.Send(data); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", n.Name, err))
			} else {
				if s, ok := stores[from.ID]; ok {
					s.Save(m, msgstore.Sent) //nolint:errcheck
				}
				if s, ok := stores[n.ID]; ok {
					s.Save(m, msgstore.Received) //nolint:errcheck
				}
			}
		}
		if len(errs) > 0 {
			return sendResultMsg{err: fmt.Errorf("%s", strings.Join(errs, "; "))}
		}
		return sendResultMsg{}
	}
}

func (m model) Init() tea.Cmd {
	return tickCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		if m.logBuffer != nil {
			m.logs = m.logBuffer.Lines()
		}
		return m, tickCmd()

	case sendResultMsg:
		if msg.err != nil {
			m.result = "Error: " + msg.err.Error()
			m.resultErr = true
		} else {
			m.result = "Message sent"
			m.resultErr = false
		}
		m.state = stateResult
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {

	case stateMenu:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter", " ":
			choice := m.cursor
			m.cursor = 0
			switch choice {
			case 0:
				m.action = actionSend
				if m.hostNode != nil {
					m.fromNode = *m.hostNode
					m.state = stateSelectTo
				} else {
					m.state = stateSelectFrom
				}
			case 1:
				m.action = actionBroadcast
				if m.hostNode != nil {
					m.fromNode = *m.hostNode
					m.inputMsg = ""
					m.state = stateInputMsg
				} else {
					m.state = stateSelectFrom
				}
			case 2:
				m.action = actionViewLogs
				if m.hostNode != nil {
					entries, _ := m.stores[m.hostNode.ID].Latest(50)
					m.result = formatEntries(m.hostNode.Name, entries, false)
					m.resultErr = false
					m.state = stateResult
				} else {
					m.state = stateSelectLogNode
				}
			case 3:
				m.action = actionListNodes
				lines := make([]string, len(m.nodes))
				for i, n := range m.nodes {
					lines[i] = fmt.Sprintf("  %-8s  %s:%d  (ws://%s:%d/ws)", n.Name, n.Host, n.Port, n.Host, n.Port)
				}
				m.result = strings.Join(lines, "\n")
				m.resultErr = false
				m.state = stateResult
			case 4:
				return m, tea.Quit
			}
		}

	case stateSelectFrom:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cursor = 0
			m.state = stateMenu
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.nodes)-1 {
				m.cursor++
			}
		case "enter":
			m.fromNode = m.nodes[m.cursor]
			m.cursor = 0
			if m.action == actionSend {
				m.state = stateSelectTo
			} else {
				m.inputMsg = ""
				m.state = stateInputMsg
			}
		}

	case stateSelectTo:
		targets := m.targets()
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cursor = 0
			m.state = stateSelectFrom
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(targets)-1 {
				m.cursor++
			}
		case "enter":
			m.toNode = targets[m.cursor]
			m.inputMsg = ""
			m.inputErr = ""
			m.state = stateInputMsg
		}

	case stateInputMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cursor = 0
			m.state = stateMenu
		case "enter":
			if m.inputMsg == "" {
				m.inputErr = "message cannot be empty"
				return m, nil
			}
			m.inputErr = ""
			if m.action == actionSend {
				return m, sendMsgCmd(m.fromNode, m.toNode, m.inputMsg, m.stores, m.pool)
			}
			return m, broadcastCmd(m.fromNode, m.nodes, m.inputMsg, m.stores, m.pool)
		case "backspace":
			if len(m.inputMsg) > 0 {
				runes := []rune(m.inputMsg)
				m.inputMsg = string(runes[:len(runes)-1])
				m.inputErr = ""
			}
		default:
			if len(msg.Runes) > 0 {
				m.inputMsg += string(msg.Runes)
				m.inputErr = ""
			}
		}

	case stateSelectLogNode:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cursor = 0
			m.state = stateMenu
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.nodes)-1 {
				m.cursor++
			}
		case "enter":
			n := m.nodes[m.cursor]
			entries, _ := m.stores[n.ID].Latest(50)
			m.result = formatEntries(n.Name, entries, false)
			m.resultErr = false
			m.cursor = 0
			m.state = stateResult
		}

	case stateResult:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		default:
			m.cursor = 0
			m.state = stateMenu
		}
	}
	return m, nil
}

func formatEntries(nodeName string, entries []msgstore.Entry, sentOnly bool) string {
	var sb strings.Builder
	for _, e := range entries {
		if sentOnly && e.Type != msgstore.Sent {
			continue
		}
		fmt.Fprintf(&sb, "%s  %-10s  %-10s  from=%-8s  to=%-8s  %q\n",
			e.At.Format(time.RFC3339),
			e.Type,
			e.Msg.Type,
			e.Msg.FromNode,
			e.Msg.ToNode,
			e.Msg.Content,
		)
	}
	if sb.Len() == 0 {
		return fmt.Sprintf("No messages for %s yet.", nodeName)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ── view ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	const hOverhead = 4
	const vOverhead = 2

	leftW := m.width/4 - hOverhead
	rightW := m.width - m.width/4 - hOverhead
	innerH := m.height - vOverhead

	left := panelStyle.Width(leftW).Height(innerH).Render(m.renderLeft())
	right := panelStyle.Width(rightW).Height(innerH).Render(m.renderLogs(innerH))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m model) renderLeft() string {
	var sb strings.Builder
	switch m.state {

	case stateMenu:
		sb.WriteString(titleStyle.Render("Menu") + "\n")
		for i, choice := range m.choices {
			if m.cursor == i {
				sb.WriteString(selectedStyle.Render("▶ "+choice) + "\n")
			} else {
				sb.WriteString("  " + choice + "\n")
			}
		}
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("↑/↓ move  enter select  q quit"))

	case stateSelectFrom:
		sb.WriteString(titleStyle.Render("Select FROM node") + "\n")
		renderNodeList(&sb, m.nodes, m.cursor)

	case stateSelectLogNode:
		sb.WriteString(titleStyle.Render("View logs for node") + "\n")
		renderNodeList(&sb, m.nodes, m.cursor)

	case stateSelectTo:
		sb.WriteString(titleStyle.Render("Select TO node") + "\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("from: %s", m.fromNode.Name)) + "\n\n")
		renderNodeList(&sb, m.targets(), m.cursor)

	case stateInputMsg:
		title := "Send message"
		if m.action == actionBroadcast {
			title = "Broadcast message"
		}
		sb.WriteString(titleStyle.Render(title) + "\n")
		if m.action == actionSend {
			sb.WriteString(dimStyle.Render(fmt.Sprintf("%s → %s", m.fromNode.Name, m.toNode.Name)) + "\n\n")
		} else {
			sb.WriteString(dimStyle.Render(fmt.Sprintf("%s → all nodes", m.fromNode.Name)) + "\n\n")
		}
		sb.WriteString("Message:\n")
		sb.WriteString("> " + m.inputMsg + "█\n")
		if m.inputErr != "" {
			sb.WriteString(errorStyle.Render(m.inputErr) + "\n")
		}
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("enter send  esc cancel"))

	case stateResult:
		sb.WriteString(titleStyle.Render("Result") + "\n")
		if m.resultErr {
			sb.WriteString(errorStyle.Render(m.result) + "\n")
		} else {
			sb.WriteString(successStyle.Render(m.result) + "\n")
		}
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("Press any key to go back..."))
	}
	return sb.String()
}

func renderNodeList(sb *strings.Builder, nodes []node.Node, cursor int) {
	for i, n := range nodes {
		label := fmt.Sprintf("%-8s  %s:%d", n.Name, n.Host, n.Port)
		if cursor == i {
			sb.WriteString(selectedStyle.Render("▶ "+label) + "\n")
		} else {
			sb.WriteString("  " + label + "\n")
		}
	}
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("↑/↓ move  enter select  esc back"))
}

func (m model) renderLogs(height int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Logs") + "\n")

	lines := m.logs
	maxLines := max(height-3, 1)
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}

	if len(lines) == 0 {
		sb.WriteString(dimStyle.Render("No logs yet..."))
	} else {
		sb.WriteString(strings.Join(lines, "\n"))
	}
	return sb.String()
}

// ── constructor ───────────────────────────────────────────────────────────────

func NewTui(buf *logbuffer.Buffer, nodes []node.Node, stores map[int]*msgstore.Store, hostNode *node.Node) (tea.Model, error) {
	p := tea.NewProgram(initialModel(buf, nodes, stores, hostNode), tea.WithAltScreen())
	return p.Run()
}
