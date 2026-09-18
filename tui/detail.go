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
	bodyTabLinks
	bodyTabCount // sentinel — keep last
)

// BackToListMsg is sent when the user presses Esc in the detail view.
type BackToListMsg struct{}

// Detail is the sub-model for the full-screen issue detail view.
type Detail struct {
	issue      model.IssueDetail
	loading    bool
	err        error
	bodyTab    detailBodyTab
	linkCursor int
	vp         viewport.Model
	spinner    spinner.Model
	sidebar    Sidebar
	width      int
	height     int
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
	// Keep the link cursor visible after every resize/render.
	if d.bodyTab == bodyTabLinks {
		d.scrollLinkIntoView()
	}
	return d
}

func (d Detail) SetIssue(issue model.IssueDetail) Detail {
	d.issue = issue
	d.loading = false
	d.err = nil
	d.bodyTab = bodyTabDescription
	d.linkCursor = 0
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
	n := (int(d.bodyTab) + dir + int(bodyTabCount)) % int(bodyTabCount)
	d.bodyTab = detailBodyTab(n)
	d.vp.SetContent(d.bodyContent(d.vp.Width))
	d.vp.GotoTop()
	return d
}

func (d Detail) Update(msg tea.Msg) (Detail, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "j", "down":
			if d.bodyTab == bodyTabLinks {
				if d.linkCursor < len(d.issue.Links)-1 {
					d.linkCursor++
				}
			} else {
				d.vp.LineDown(1)
			}
			return d, nil
		case "k", "up":
			if d.bodyTab == bodyTabLinks {
				if d.linkCursor > 0 {
					d.linkCursor--
				}
			} else {
				d.vp.LineUp(1)
			}
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

// scrollLinkIntoView adjusts the viewport YOffset so the cursor row is visible.
// Header is 2 lines (header row + separator), so cursor row is at line linkCursor+2.
// Must only be called when vp.Height > 0 (i.e. from SetSize, not from Update).
func (d *Detail) scrollLinkIntoView() {
	if d.vp.Height <= 0 {
		return
	}
	line := d.linkCursor + 2
	if line < d.vp.YOffset {
		d.vp.YOffset = line
	} else if line >= d.vp.YOffset+d.vp.Height {
		d.vp.YOffset = line - d.vp.Height + 1
	}
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

// AppendLinks adds links to the issue (used for async child-issue fetch).
func (d Detail) AppendLinks(links []model.IssueLink) Detail {
	d.issue.Links = append(d.issue.Links, links...)
	d.vp.SetContent(d.bodyContent(d.vp.Width))
	return d
}

// SelectedLink returns the highlighted link when the Links tab is active.
func (d Detail) SelectedLink() (model.IssueLink, bool) {
	if d.bodyTab != bodyTabLinks || d.linkCursor < 0 || d.linkCursor >= len(d.issue.Links) {
		return model.IssueLink{}, false
	}
	return d.issue.Links[d.linkCursor], true
}

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
	case bodyTabLinks:
		return d.renderLinks(width)
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

func (d Detail) renderLinks(width int) string {
	if len(d.issue.Links) == 0 {
		return statusIdleStyle.Padding(1, 1).Render("No linked issues.")
	}
	if width < 10 {
		width = 10
	}

	const typeW, keyW, statusW = 20, 12, 14
	summaryW := width - typeW - keyW - statusW - 8
	if summaryW < 8 {
		summaryW = 8
	}

	typeStyle := lipgloss.NewStyle().Width(typeW).Foreground(lipgloss.Color("#888888"))
	keyStyle := lipgloss.NewStyle().Width(keyW).Foreground(lipgloss.Color("#5555ff")).Bold(true)
	summaryStyle := lipgloss.NewStyle().Width(summaryW).Foreground(lipgloss.Color("#dddddd"))
	statusStyle := lipgloss.NewStyle().Width(statusW).Foreground(lipgloss.Color("#aaaaaa"))
	cursorStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#222255")).
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true)
	hdrStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Bold(true)

	var sb strings.Builder
	hdr := fmt.Sprintf("  %-*s  %-*s  %-*s  %s", typeW, "TYPE", keyW, "KEY", summaryW, "SUMMARY", "STATUS")
	sb.WriteString(hdrStyle.Render(hdr) + "\n")
	sepW := width - 2
	if sepW < 0 {
		sepW = 0
	}
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")).
		Render(strings.Repeat("─", sepW)) + "\n")

	for i, link := range d.issue.Links {
		typ := link.Type
		if len(typ) > typeW {
			typ = typ[:typeW-1] + "…"
		}
		key := link.Key
		if len(key) > keyW {
			key = key[:keyW-1] + "…"
		}
		sum := link.Summary
		if len(sum) > summaryW {
			sum = sum[:summaryW-1] + "…"
		}
		status := link.Status
		if len(status) > statusW {
			status = status[:statusW-1] + "…"
		}

		if i == d.linkCursor {
			line := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s", typeW, typ, keyW, key, summaryW, sum, statusW, status)
			sb.WriteString(cursorStyle.Width(width - 2).Render(line) + "\n")
		} else {
			row := "  " + typeStyle.Render(typ) + "  " + keyStyle.Render(key) + "  " +
				summaryStyle.Render(sum) + "  " + statusStyle.Render(status)
			sb.WriteString(row + "\n")
		}
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
	linkCount := fmt.Sprintf("Links (%d)", len(d.issue.Links))
	linksTab := detailTabInactiveStyle.Render(linkCount)
	switch d.bodyTab {
	case bodyTabDescription:
		descTab = detailTabActiveStyle.Render("Description")
	case bodyTabComments:
		commTab = detailTabActiveStyle.Render("Comments")
	case bodyTabLinks:
		linksTab = detailTabActiveStyle.Render(linkCount)
	}
	bodyTabBar := descTab + commTab + linksTab

	// Main pane (viewport) + sidebar side by side.
	mainPane := lipgloss.NewStyle().Width(mainW).Render(d.vp.View())
	sidePane := d.sidebar.View(d.issue)

	content := lipgloss.JoinHorizontal(lipgloss.Top, mainPane, sidePane)

	return lipgloss.JoinVertical(lipgloss.Left, header, bodyTabBar, content)
}
