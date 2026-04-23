package tui

import (
	"fmt"
	"strings"
	"time"

	"node_messager/pkg/logbuffer"
	"node_messager/pkg/node"
	"node_messager/pkg/wsclient"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
	stateResult
)

type menuAction int

const (
	actionSend menuAction = iota
	actionBroadcast
	actionListNodes
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

	inputMsg string

	result    string
	resultErr bool

	width  int
	height int

	logBuffer *logbuffer.Buffer
	logs      []string
}

func initialModel(buf *logbuffer.Buffer, nodes []node.Node) model {
	return model{
		choices:   []string{"Send a message", "Broadcast a message", "List all nodes", "Quit"},
		state:     stateMenu,
		logBuffer: buf,
		nodes:     nodes,
	}
}

func tickCmd() tea.Cmd {
	return tea.Every(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func sendMsgCmd(from, to node.Node, msg string) tea.Cmd {
	return func() tea.Msg {
		c, err := wsclient.Connect(to.Host, to.Port)
		if err != nil {
			return sendResultMsg{err: err}
		}
		defer c.Close()
		payload := fmt.Sprintf("[from:%s → to:%s] %s", from.Name, to.Name, msg)
		return sendResultMsg{err: c.Send([]byte(payload))}
	}
}

func broadcastCmd(from node.Node, nodes []node.Node, msg string) tea.Cmd {
	return func() tea.Msg {
		payload := fmt.Sprintf("[broadcast from:%s] %s", from.Name, msg)
		var errs []string
		for _, n := range nodes {
			c, err := wsclient.Connect(n.Host, n.Port)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", n.Name, err))
				continue
			}
			if err := c.Send([]byte(payload)); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", n.Name, err))
			}
			c.Close()
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
				m.state = stateSelectFrom
			case 1:
				m.action = actionBroadcast
				m.state = stateSelectFrom
			case 2:
				m.action = actionListNodes
				lines := make([]string, len(m.nodes))
				for i, n := range m.nodes {
					lines[i] = fmt.Sprintf("  %-8s", n.Name)
				}
				m.result = strings.Join(lines, "\n")
				m.resultErr = false
				m.state = stateResult
			case 3:
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
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cursor = 0
			m.state = stateSelectFrom
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.nodes)-1 {
				m.cursor++
			}
		case "enter":
			m.toNode = m.nodes[m.cursor]
			m.inputMsg = ""
			m.state = stateInputMsg
		}

	case stateInputMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cursor = 0
			m.state = stateMenu
		case "enter":
			if m.inputMsg == "" {
				return m, nil
			}
			if m.action == actionSend {
				return m, sendMsgCmd(m.fromNode, m.toNode, m.inputMsg)
			}
			return m, broadcastCmd(m.fromNode, m.nodes, m.inputMsg)
		case "backspace":
			if len(m.inputMsg) > 0 {
				runes := []rune(m.inputMsg)
				m.inputMsg = string(runes[:len(runes)-1])
			}
		default:
			if len(msg.Runes) > 0 {
				m.inputMsg += string(msg.Runes)
			}
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

	case stateSelectTo:
		sb.WriteString(titleStyle.Render("Select TO node") + "\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("from: %s", m.fromNode.Name)) + "\n\n")
		renderNodeList(&sb, m.nodes, m.cursor)

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
		sb.WriteString("> " + m.inputMsg + "█\n\n")
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

func NewTui(buf *logbuffer.Buffer, nodes []node.Node) (tea.Model, error) {
	p := tea.NewProgram(initialModel(buf, nodes), tea.WithAltScreen())
	return p.Run()
}
