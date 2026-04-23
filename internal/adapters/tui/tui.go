package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"node_messager/pkg/logbuffer"
)

type tickMsg time.Time

type appState int

const (
	stateMenu appState = iota
	stateResult
)

var (
	borderColor   = lipgloss.Color("62")
	dimColor      = lipgloss.Color("241")
	selectedColor = lipgloss.Color("212")

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

	dimStyle = lipgloss.NewStyle().
			Foreground(dimColor)
)

type model struct {
	choices   []string
	cursor    int
	state     appState
	result    string
	width     int
	height    int
	logBuffer *logbuffer.Buffer
	logs      []string
}

func initialModel(buf *logbuffer.Buffer) model {
	return model{
		choices:   []string{"Send a message", "Broadcast a message", "List all nodes", "Log node messages", "Quit"},
		state:     stateMenu,
		logBuffer: buf,
	}
}

func tickCmd() tea.Cmd {
	return tea.Every(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
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

	case tea.KeyMsg:
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
				switch m.cursor {
				case 0:
					m.result = "Hello!"
				case 1:
					m.result = "Goodbye!"
				case 2:
					m.result = "1  2  3"
				case 4:
					return m, tea.Quit
				}
				m.state = stateResult
			}

		case stateResult:
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			default:
				m.state = stateMenu
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	// border(2) + padding(2) = 4 overhead per panel horizontally
	// border(2) + padding(0) = 2 overhead vertically
	const hOverhead = 4
	const vOverhead = 2

	leftW := m.width/2 - hOverhead
	rightW := m.width - m.width/2 - hOverhead
	innerH := m.height - vOverhead

	left := panelStyle.
		Width(leftW).
		Height(innerH).
		Render(m.renderMenu(innerH))

	right := panelStyle.
		Width(rightW).
		Height(innerH).
		Render(m.renderLogs(rightW, innerH))

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m model) renderMenu(_ int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Menu") + "\n")

	switch m.state {
	case stateResult:
		fmt.Fprintf(&sb, "Result: %s\n\n", m.result)
		sb.WriteString(dimStyle.Render("Press any key to go back..."))
	default:
		for i, choice := range m.choices {
			if m.cursor == i {
				sb.WriteString(selectedStyle.Render("▶ "+choice) + "\n")
			} else {
				sb.WriteString("  " + choice + "\n")
			}
		}
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("↑/↓ move  enter select  q quit"))
	}

	return sb.String()
}

func (m model) renderLogs(_ int, height int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Logs") + "\n")

	lines := m.logs
	maxLines := height - 3 // title + newline + bottom margin
	maxLines = max(maxLines, 1)
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

func NewTui(buf *logbuffer.Buffer) (tea.Model, error) {
	p := tea.NewProgram(initialModel(buf), tea.WithAltScreen())
	return p.Run()
}
