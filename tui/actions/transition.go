package actions

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clobrano/jira-tabbed-tui/model"
)

// TransitionSelectedMsg is sent when the user picks a transition.
type TransitionSelectedMsg struct {
	IssueKey       string
	TransitionName string
}

// TransitionCancelledMsg is sent when the user presses Esc.
type TransitionCancelledMsg struct{}

var (
	transitionOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#5555ff")).
				Padding(1, 2).
				Background(lipgloss.Color("#111111"))

	transitionTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#ffffff")).
				Bold(true)

	transitionCurrentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00cc44"))
)

type transitionItem struct {
	t model.Transition
}

func (ti transitionItem) Title() string {
	if ti.t.IsCurrentStatus {
		return transitionCurrentStyle.Render("● " + ti.t.Name + " (current)")
	}
	return ti.t.Name
}
func (ti transitionItem) Description() string { return "" }
func (ti transitionItem) FilterValue() string  { return ti.t.Name }

// TransitionModel is the overlay for picking a status transition.
type TransitionModel struct {
	issueKey string
	list     list.Model
	loading  bool
	err      error
	width    int
	height   int
}

func NewTransitionModel(issueKey string) TransitionModel {
	l := list.New(nil, list.NewDefaultDelegate(), 40, 12)
	l.Title = "Change Status"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = transitionTitleStyle
	return TransitionModel{issueKey: issueKey, list: l, loading: true}
}

func (m TransitionModel) SetTransitions(ts []model.Transition, err error) TransitionModel {
	m.err = err
	m.loading = false
	if err != nil {
		return m
	}
	items := make([]list.Item, len(ts))
	for i, t := range ts {
		items[i] = transitionItem{t}
	}
	m.list.SetItems(items)
	// Move cursor to the current status.
	for i, t := range ts {
		if t.IsCurrentStatus {
			m.list.Select(i)
			break
		}
	}
	return m
}

func (m TransitionModel) SetSize(w, h int) TransitionModel {
	m.width = w
	m.height = h
	overlayW := 44
	overlayH := 16
	if overlayW > w-4 {
		overlayW = w - 4
	}
	m.list.SetSize(overlayW-4, overlayH-4)
	return m
}

func (m TransitionModel) Update(msg tea.Msg) (TransitionModel, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	if m.err != nil {
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.String() == "esc" || key.String() == "q" {
				return m, func() tea.Msg { return TransitionCancelledMsg{} }
			}
		}
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	switch key.String() {
	case "enter":
		if item, ok := m.list.SelectedItem().(transitionItem); ok {
			return m, func() tea.Msg {
				return TransitionSelectedMsg{
					IssueKey:       m.issueKey,
					TransitionName: item.t.Name,
				}
			}
		}
	case "esc", "q":
		return m, func() tea.Msg { return TransitionCancelledMsg{} }
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m TransitionModel) View() string {
	overlayW := 44
	if m.width > 0 && overlayW > m.width-4 {
		overlayW = m.width - 4
	}

	var content string
	switch {
	case m.loading:
		content = "  Loading transitions…"
	case m.err != nil:
		content = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4444")).
			Render("Error: "+m.err.Error()) + "\n\nPress Esc to cancel."
	default:
		content = m.list.View()
	}

	overlay := transitionOverlayStyle.Width(overlayW).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}
