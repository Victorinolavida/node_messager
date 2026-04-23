package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// States
type appState int

const (
	stateMenu appState = iota
	stateResult
)

type model struct {
	choices []string
	cursor  int
	state   appState
	result  string
}

func initialModel() model {
	return model{
		choices: []string{"Send a message", "Broadcast a message", "List all nodes", "Log node messages", "Quit"},
		state:   stateMenu,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.state {

		// ── Menu state ──────────────────────────────────────────
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
				// Run the selected action
				switch m.cursor {
				case 0:
					m.result = "👋 Hello!"
				case 1:
					m.result = "👋 Goodbye!"
				case 2:
					m.result = "1️⃣  2️⃣  3️⃣"
				case 3:
					return m, tea.Quit
				}
				// ⬇️  KEY PART: switch to result state instead of quitting
				m.state = stateResult
			}

		// ── Result state: show output, then go back to menu ─────
		case stateResult:
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			default:
				// Any key press returns to the menu
				m.state = stateMenu
			}
		}
	}

	return m, nil
}

func (m model) View() string {
	switch m.state {

	case stateResult:
		return fmt.Sprintf(
			"\n  Result: %s\n\n  Press any key to go back to the menu...\n",
			m.result,
		)

	default: // stateMenu
		s := "\n  What do you want to do?\n\n"
		for i, choice := range m.choices {
			cursor := "  "
			if m.cursor == i {
				cursor = "▶ "
			}
			s += fmt.Sprintf("  %s%s\n", cursor, choice)
		}
		s += "\n  (↑/↓ to navigate, enter to select, q to quit)\n"
		return s
	}
}

func NewTui() (tea.Model, error) {
	p := tea.NewProgram(initialModel())
	return p.Run()
}
