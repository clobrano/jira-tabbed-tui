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

// DetailViewState is the part of the detail view restored when navigating
// back/forward through history: the active body tab and the selected link.
type DetailViewState struct {
	bodyTab detailBodyTab
	linkSel string // linkID of the selected link
}

// BackToListMsg is sent when the user presses Esc in the detail view.
type BackToListMsg struct{}

// Detail is the sub-model for the full-screen issue detail view.
type Detail struct {
	issue      model.IssueDetail
	loading    bool
	err        error
	bodyTab    detailBodyTab
	linkCursor int
	linkSel    string // linkID of the selected link; the cursor follows it as links are sorted/appended
	crumbs     []string
	crumbIdx   int
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
	if d.breadcrumb(mainW) != "" {
		vpH--
	}
	if vpH < 3 {
		vpH = 3
	}
	// Resize in place — recreating would reset YOffset and lose scroll position.
	d.vp.Width = mainW
	d.vp.Height = vpH
	d.sidebar = d.sidebar.SetHeight(vpH)
	d.vp.SetContent(d.bodyContent(mainW))
	// Keep the link cursor visible after every resize/render.
	if d.bodyTab == bodyTabLinks {
		d.scrollLinkIntoView()
	}
	return d
}

func (d Detail) SetIssue(issue model.IssueDetail) Detail {
	// Reloading the same issue (e.g. after acting on one of its links) keeps
	// the current body tab, link cursor and scroll position.
	refresh := issue.Key != "" && issue.Key == d.issue.Key
	// Copy before sorting: the slice is shared with the cache.
	issue.Links = append([]model.IssueLink(nil), issue.Links...)
	d.issue = issue
	d.loading = false
	d.err = nil
	if !refresh {
		d.bodyTab = bodyTabDescription
		d.linkSel = ""
		d.linkCursor = 0
	}
	d.relink()
	d.vp.SetContent(d.bodyContent(d.vp.Width))
	if !refresh {
		d.vp.GotoTop()
	}
	return d
}

// relink sorts the links and points the cursor back at the selected link.
func (d *Detail) relink() {
	sortLinks(d.issue.Links)
	for i, l := range d.issue.Links {
		if d.linkSel != "" && linkID(l) == d.linkSel {
			d.linkCursor = i
			return
		}
	}
	if d.linkCursor >= len(d.issue.Links) {
		d.linkCursor = len(d.issue.Links) - 1
	}
	if d.linkCursor < 0 {
		d.linkCursor = 0
	}
}

// moveLinkCursor moves the selection by delta and remembers the selected link.
func (d *Detail) moveLinkCursor(delta int) {
	n := d.linkCursor + delta
	if n < 0 || n >= len(d.issue.Links) {
		return
	}
	d.linkCursor = n
	d.linkSel = linkID(d.issue.Links[n])
	d.vp.SetContent(d.bodyContent(d.vp.Width))
	d.scrollLinkIntoView()
}

// SetBreadcrumb sets the detail history shown above the header when more
// than one issue has been visited.
func (d Detail) SetBreadcrumb(keys []string, idx int) Detail {
	d.crumbs = keys
	d.crumbIdx = idx
	return d
}

func (d Detail) breadcrumb(width int) string {
	return renderBreadcrumb(d.crumbs, d.crumbIdx, width)
}

// SetPullRequestStates fills in the status of PR web links, keyed by URL.
func (d Detail) SetPullRequestStates(states map[string]string) Detail {
	links := append([]model.IssueLink(nil), d.issue.Links...)
	for i, l := range links {
		if s, ok := states[l.URL]; ok && l.URL != "" {
			links[i].Status = s
		}
	}
	d.issue.Links = links
	d.vp.SetContent(d.bodyContent(d.vp.Width))
	return d
}

// ViewState returns the current body tab and link cursor.
func (d Detail) ViewState() DetailViewState {
	sel := d.linkSel
	if sel == "" && d.linkCursor < len(d.issue.Links) {
		sel = linkID(d.issue.Links[d.linkCursor])
	}
	return DetailViewState{bodyTab: d.bodyTab, linkSel: sel}
}

// RestoreViewState re-applies a state saved with ViewState. The selected link
// may not be loaded yet (children and remote links arrive asynchronously);
// the cursor moves to it once it does.
func (d Detail) RestoreViewState(s DetailViewState) Detail {
	d.bodyTab = s.bodyTab
	d.linkSel = s.linkSel
	d.relink()
	d.vp.SetContent(d.bodyContent(d.vp.Width))
	d.vp.GotoTop()
	if d.bodyTab == bodyTabLinks {
		d.scrollLinkIntoView()
	}
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
				d.moveLinkCursor(1)
			} else {
				d.vp.LineDown(1)
			}
			return d, nil
		case "k", "up":
			if d.bodyTab == bodyTabLinks {
				d.moveLinkCursor(-1)
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
func (d *Detail) scrollLinkIntoView() {
	if d.vp.Height <= 0 {
		return
	}
	line := linkRowLine(d.issue.Links, d.linkCursor)
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

// Issue returns the displayed issue.
func (d Detail) Issue() model.IssueDetail { return d.issue }

// AppendLinks adds links to the issue (used for async child-issue fetch).
func (d Detail) AppendLinks(links []model.IssueLink) Detail {
	d.issue.Links = append(append([]model.IssueLink(nil), d.issue.Links...), links...)
	d.relink()
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
		return renderLinkTable(d.issue.Links, d.linkCursor, width)
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
	commLabel := fmt.Sprintf("Comments (%d)", len(d.issue.Comments))
	linkLabel := fmt.Sprintf("Links (%d)", len(d.issue.Links))
	descTab := detailTabInactiveStyle.Render("Description")
	commTab := detailTabInactiveStyle.Render(commLabel)
	linksTab := detailTabInactiveStyle.Render(linkLabel)
	switch d.bodyTab {
	case bodyTabDescription:
		descTab = detailTabActiveStyle.Render("Description")
	case bodyTabComments:
		commTab = detailTabActiveStyle.Render(commLabel)
	case bodyTabLinks:
		linksTab = detailTabActiveStyle.Render(linkLabel)
	}
	bodyTabBar := descTab + commTab + linksTab

	// Main pane (viewport) + sidebar side by side.
	mainPane := lipgloss.NewStyle().Width(mainW).Render(d.vp.View())
	sidePane := d.sidebar.View(d.issue)

	content := lipgloss.JoinHorizontal(lipgloss.Top, mainPane, sidePane)

	if crumb := d.breadcrumb(mainW); crumb != "" {
		return lipgloss.JoinVertical(lipgloss.Left, crumb, header, bodyTabBar, content)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, bodyTabBar, content)
}
