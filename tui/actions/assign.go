package actions

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clobrano/jira-tabbed-tui/model"
)

type assignPhase int

const (
	assignPhaseInput   assignPhase = iota
	assignPhaseResults assignPhase = iota
)

// AssignSearchRequestMsg is sent when the user submits a search query.
type AssignSearchRequestMsg struct {
	IssueKey string
	Query    string
}

// AssignConfirmedMsg is sent when the user selects an assignee.
type AssignConfirmedMsg struct {
	IssueKey    string
	AccountID   string
	DisplayName string
}

// AssignCancelledMsg is sent when the user cancels.
type AssignCancelledMsg struct{}

var (
	assignOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#5555ff")).
				Padding(1, 2).
				Background(lipgloss.Color("#111111"))

	assignTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#ffffff")).
				Bold(true)

	assignCursorStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#222255")).
				Foreground(lipgloss.Color("#ffffff")).
				Bold(true)

	assignResultStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#dddddd"))

	assignHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))

	assignErrStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff4444"))
)

// AssignModel is the overlay for searching and picking an assignee.
type AssignModel struct {
	issueKey  string
	input     textinput.Model
	results   []model.User
	cursor    int
	phase     assignPhase
	searching bool
	err       error
	width     int
	height    int
}

func NewAssignModel(issueKey string) AssignModel {
	ti := textinput.New()
	ti.Placeholder = "type name or email…"
	ti.CharLimit = 128
	ti.Width = 40
	ti.Focus()
	return AssignModel{issueKey: issueKey, input: ti, phase: assignPhaseInput}
}

func (m AssignModel) SetSize(w, h int) AssignModel {
	m.width = w
	m.height = h
	inputW := 52
	if inputW > w-8 {
		inputW = w - 8
	}
	m.input.Width = inputW - 4
	return m
}

func (m AssignModel) SetResults(users []model.User, err error) AssignModel {
	m.searching = false
	m.err = err
	m.results = users
	m.cursor = 0
	if err == nil {
		m.phase = assignPhaseResults
	}
	return m
}

func (m AssignModel) Update(msg tea.Msg) (AssignModel, tea.Cmd) {
	key, isKey := msg.(tea.KeyMsg)
	if !isKey {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	// Esc: back to search input if in results, otherwise cancel.
	if key.String() == "esc" {
		if m.phase == assignPhaseResults {
			m.phase = assignPhaseInput
			m.results = nil
			m.err = nil
			m.input.Focus()
			return m, nil
		}
		return m, func() tea.Msg { return AssignCancelledMsg{} }
	}

	switch m.phase {
	case assignPhaseInput:
		if m.searching {
			return m, nil
		}
		if key.String() == "enter" {
			q := strings.TrimSpace(m.input.Value())
			if q == "" {
				return m, nil
			}
			m.searching = true
			return m, func() tea.Msg {
				return AssignSearchRequestMsg{IssueKey: m.issueKey, Query: q}
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case assignPhaseResults:
		switch key.String() {
		case "j", "down":
			if m.cursor < len(m.results)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			if m.cursor < len(m.results) {
				u := m.results[m.cursor]
				return m, func() tea.Msg {
					return AssignConfirmedMsg{IssueKey: m.issueKey, AccountID: u.AccountID, DisplayName: u.DisplayName}
				}
			}
		case "/":
			m.phase = assignPhaseInput
			m.results = nil
			m.err = nil
			m.input.Focus()
		}
	}
	return m, nil
}

func (m AssignModel) View() string {
	overlayW := 58
	if m.width > 0 && overlayW > m.width-4 {
		overlayW = m.width - 4
	}

	var sb strings.Builder
	sb.WriteString(assignTitleStyle.Render("Assign Issue") + "\n\n")

	switch m.phase {
	case assignPhaseInput:
		sb.WriteString(m.input.View() + "\n\n")
		if m.searching {
			sb.WriteString(assignHintStyle.Render("Searching…"))
		} else if m.err != nil {
			sb.WriteString(assignErrStyle.Render("Error: " + m.err.Error()))
		} else {
			sb.WriteString(assignHintStyle.Render("Enter to search  •  Esc to cancel"))
		}

	case assignPhaseResults:
		if len(m.results) == 0 {
			sb.WriteString(assignHintStyle.Render("No users found.") + "\n\n")
			sb.WriteString(assignHintStyle.Render("Esc to search again"))
		} else {
			inner := overlayW - 4 // subtract padding
			emailW := inner / 3
			nameW := inner - emailW - 4 // 4 for column gap
			if emailW < 10 {
				emailW = 10
			}
			hdr := fmt.Sprintf("%-*s  %s", nameW, "NAME", "EMAIL")
			sb.WriteString(assignHintStyle.Render(hdr) + "\n")
			sb.WriteString(assignHintStyle.Render(strings.Repeat("─", inner)) + "\n")
			for i, u := range m.results {
				name := u.DisplayName
				if len(name) > nameW {
					name = name[:nameW-1] + "…"
				}
				email := u.Email
				if len(email) > emailW {
					email = email[:emailW-1] + "…"
				}
				line := fmt.Sprintf("%-*s  %-*s", nameW, name, emailW, email)
				if i == m.cursor {
					sb.WriteString(assignCursorStyle.Width(inner).Render(line) + "\n")
				} else {
					sb.WriteString(assignResultStyle.Render(line) + "\n")
				}
			}
			sb.WriteString("\n")
			sb.WriteString(assignHintStyle.Render("j/k navigate  •  Enter assign  •  Esc back  •  / new search"))
		}
	}

	overlay := assignOverlayStyle.Width(overlayW).Render(sb.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}
