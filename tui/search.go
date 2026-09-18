package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var searchInputStyle = lipgloss.NewStyle().
	Border(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("#5555ff")).
	Padding(0, 1).
	Margin(1, 2)

// SearchRunMsg is sent when the user submits a JQL query in the Search tab.
type SearchRunMsg struct {
	Query string
}

// SearchInput manages the JQL input in the Search tab.
type SearchInput struct {
	input textinput.Model
	query string // last submitted query
	width int
}

func NewSearchInput() SearchInput {
	ti := textinput.New()
	ti.Placeholder = "assignee = currentUser() AND status != Done"
	ti.CharLimit = 512
	ti.Width = 60
	return SearchInput{input: ti}
}

func (s SearchInput) Focus() SearchInput {
	s.input.Focus()
	return s
}

func (s SearchInput) Blur() SearchInput {
	s.input.Blur()
	return s
}

func (s SearchInput) IsFocused() bool { return s.input.Focused() }

func (s SearchInput) Query() string { return s.query }

func (s SearchInput) SetWidth(w int) SearchInput {
	s.width = w
	inputW := w - 8
	if inputW < 20 {
		inputW = 20
	}
	s.input.Width = inputW
	return s
}

func (s SearchInput) Update(msg tea.Msg) (SearchInput, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		q := s.input.Value()
		if q != "" {
			s.query = q
			s.input.Blur() // shift focus to list
			return s, func() tea.Msg { return SearchRunMsg{Query: q} }
		}
	}
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return s, cmd
}

func (s SearchInput) View() string {
	return searchInputStyle.Render(s.input.View())
}
