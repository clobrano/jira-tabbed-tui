package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clobrano/jira-tabbed-tui/config"
	"github.com/clobrano/jira-tabbed-tui/model"
)

var (
	detailTabActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#ffffff")).
				Background(lipgloss.Color("#5555ff")).
				Padding(0, 2).
				Bold(true)

	detailTabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#aaaaaa")).
				Padding(0, 2)

	detailTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#ffffff")).
				Bold(true).
				Padding(0, 1)

	detailKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5555ff")).
			Bold(true)

	detailErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#ff4444")).
				Padding(1, 2)
)

type detailBodyTab int

const (
	bodyTabDescription detailBodyTab = iota
	bodyTabComments
)

// BackToListMsg is sent when the user presses Esc in the detail view.
type BackToListMsg struct{}

// Detail is the sub-model for the full-screen issue detail view.
type Detail struct {
	issue    model.IssueDetail
	loading  bool
	err      error
	bodyTab  detailBodyTab
	vp       viewport.Model
	spinner  spinner.Model
	sidebar  Sidebar
	width    int
	height   int
}

func NewDetail(cfg config.Config) Detail {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#5555ff"))
	return Detail{
		spinner: s,
		loading: true,
		sidebar: NewSidebar(cfg.Detail.SidebarFields),
	}
}

func (d Detail) SetSize(w, h int, sidebarPct int) Detail {
	d.width = w
	d.height = h
	sidebarW := w * sidebarPct / 100
	if sidebarW < 20 {
		sidebarW = 20
	}
	if sidebarW > w-30 {
		sidebarW = w - 30
	}
	d.sidebar = d.sidebar.SetWidth(sidebarW)
	mainW := w - sidebarW - 1
	vpH := h - 4 // header + body-tab bar
	if vpH < 3 {
		vpH = 3
	}
	// Resize in place — recreating would reset YOffset and lose scroll position.
	d.vp.Width = mainW
	d.vp.Height = vpH
	d.vp.SetContent(d.bodyContent(mainW))
	return d
}

func (d Detail) SetIssue(issue model.IssueDetail) Detail {
	d.issue = issue
	d.loading = false
	d.err = nil
	d.bodyTab = bodyTabDescription
	d.vp.SetContent(d.bodyContent(d.vp.Width))
	d.vp.GotoTop()
	return d
}

func (d Detail) SetError(err error) Detail {
	d.err = err
	d.loading = false
	return d
}

func (d Detail) SetLoading(v bool) Detail {
	d.loading = v
	return d
}

func (d Detail) SwitchBodyTab(dir int) Detail {
	if dir > 0 {
		d.bodyTab = bodyTabComments
	} else {
		d.bodyTab = bodyTabDescription
	}
	d.vp.SetContent(d.bodyContent(d.vp.Width))
	d.vp.GotoTop()
	return d
}

func (d Detail) Update(msg tea.Msg) (Detail, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "j", "down":
			d.vp.LineDown(1)
			return d, nil
		case "k", "up":
			d.vp.LineUp(1)
			return d, nil
		case "ctrl+d":
			d.vp.HalfPageDown()
			return d, nil
		case "ctrl+u":
			d.vp.HalfPageUp()
			return d, nil
		case "g":
			d.vp.GotoTop()
			return d, nil
		case "G":
			d.vp.GotoBottom()
			return d, nil
		}
	}
	// Forward mouse wheel and any other events to the viewport.
	var cmd tea.Cmd
	d.vp, cmd = d.vp.Update(msg)
	return d, cmd
}

func (d Detail) SpinnerTick() (Detail, tea.Cmd) {
	var cmd tea.Cmd
	d.spinner, cmd = d.spinner.Update(d.spinner.Tick())
	return d, cmd
}

func (d Detail) Init() tea.Cmd {
	return d.spinner.Tick
}

func (d Detail) IssueKey() string { return d.issue.Key }

func (d Detail) bodyContent(width int) string {
	if d.loading || d.err != nil {
		return ""
	}
	switch d.bodyTab {
	case bodyTabDescription:
		if d.issue.Description == "" {
			return statusIdleStyle.Padding(1, 1).Render("No description.")
		}
		return lipgloss.NewStyle().Width(width).Padding(0, 1).Render(d.issue.Description)
	case bodyTabComments:
		return d.renderComments(width)
	}
	return ""
}

func (d Detail) renderComments(width int) string {
	if len(d.issue.Comments) == 0 {
		return statusIdleStyle.Padding(1, 1).Render("No comments.")
	}
	var sb strings.Builder
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Width(width - 2).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color("#333333"))
	bodyStyle := lipgloss.NewStyle().Width(width - 2).Padding(0, 1)

	for _, c := range d.issue.Comments {
		header := fmt.Sprintf("%s  %s", c.Author, c.Created)
		sb.WriteString(headerStyle.Render(header))
		sb.WriteString("\n")
		sb.WriteString(bodyStyle.Render(c.Body))
		sb.WriteString("\n\n")
	}
	return sb.String()
}

func (d Detail) View() string {
	if d.loading {
		return fmt.Sprintf("\n  %s Loading issue…", d.spinner.View())
	}
	if d.err != nil {
		return detailErrorStyle.Render(
			"Error loading issue: "+MapCLIError(d.err.Error())+"\n\nPress Esc to go back.")
	}

	sidebarW := d.sidebar.width
	mainW := d.width - sidebarW - 1

	// Header: key + summary.
	header := detailKeyStyle.Render(d.issue.Key) + "  " +
		detailTitleStyle.Width(mainW-len(d.issue.Key)-3).Render(d.issue.Summary)

	// Body tab bar.
	descTab := detailTabInactiveStyle.Render("Description")
	commTab := detailTabInactiveStyle.Render("Comments")
	if d.bodyTab == bodyTabDescription {
		descTab = detailTabActiveStyle.Render("Description")
	} else {
		commTab = detailTabActiveStyle.Render("Comments")
	}
	bodyTabBar := descTab + commTab

	// Main pane (viewport) + sidebar side by side.
	mainPane := lipgloss.NewStyle().Width(mainW).Render(d.vp.View())
	sidePane := d.sidebar.View(d.issue)

	content := lipgloss.JoinHorizontal(lipgloss.Top, mainPane, sidePane)

	return lipgloss.JoinVertical(lipgloss.Left, header, bodyTabBar, content)
}
