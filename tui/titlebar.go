package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// appTitle and appTagline open the title bar.
const (
	appTitle   = "JIRA-TABBED-TUI"
	appTagline = "Jira, one tab per query"
)

var (
	titleNameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8888ff")).Bold(true)
	titleDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	titleMutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))
	titleWarnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#e3b341"))

	statusTodoColor     = lipgloss.Color("#8b949e")
	statusProgressColor = lipgloss.Color("#58a6ff")
	statusReviewColor   = lipgloss.Color("#a371f7")
	statusDoneColor     = lipgloss.Color("#3fb950")
)

// statusColor guesses a colour for a Jira status from its name: to do (grey),
// in progress (blue), review/QA (purple), done (green).
func statusColor(status string) lipgloss.Color {
	s := strings.ToLower(status)
	switch {
	case containsAny(s, "done", "closed", "resolved", "complete", "released", "merged"):
		return statusDoneColor
	case containsAny(s, "review", "qa", "test", "verif", "approval"):
		return statusReviewColor
	case containsAny(s, "to do", "todo", "open", "new", "backlog", "selected"):
		return statusTodoColor
	default:
		return statusProgressColor
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// titleSeg is one piece of the title bar. Pieces give way on narrow screens
// in drop order (higher first): the tagline, then the user, the server, the
// rarest statuses and finally the issue count; the name never drops.
type titleSeg struct {
	text  string
	style lipgloss.Style
	drop  int
	sep   bool // preceded by " · "
}

// titleBar is the top line: the app name and what you are looking at on the
// left, how fresh the active tab's data is on the right, e.g.
//
//	JIRA-TABBED-TUI  Jira, one tab per query · acme.atlassian.net · dana@acme.com · 6 issues 3 In Progress 2 To Do 1 In Review      updated 12s ago
func (a App) titleBar() string {
	segs := []titleSeg{
		{text: " " + appTitle, style: titleNameStyle},
		{text: "  " + appTagline, style: titleDimStyle, drop: 30},
	}
	if a.server != "" {
		segs = append(segs, titleSeg{text: a.server, style: titleMutedStyle, drop: 20, sep: true})
	}
	if a.user != "" {
		segs = append(segs, titleSeg{text: a.user, style: titleMutedStyle, drop: 25, sep: true})
	}

	var right string
	if a.activeTab < len(a.tabs) {
		tab := a.tabs[a.activeTab]
		issues := tab.list.allIssues
		if !tab.list.loading && (!tab.isSearch || tab.jql != "") {
			n := fmt.Sprintf("%d issues", len(issues))
			switch {
			case len(issues) == 1:
				n = "1 issue"
			case tab.list.total > len(issues):
				n = fmt.Sprintf("%d of %d issues", len(issues), tab.list.total)
			}
			segs = append(segs, titleSeg{text: n, style: titleMutedStyle, drop: 1, sep: true})

			// Status breakdown of the loaded issues, most common first.
			counts := map[string]int{}
			for _, iss := range issues {
				counts[iss.Status]++
			}
			statuses := make([]string, 0, len(counts))
			for s := range counts {
				statuses = append(statuses, s)
			}
			sort.Slice(statuses, func(i, j int) bool {
				if counts[statuses[i]] != counts[statuses[j]] {
					return counts[statuses[i]] > counts[statuses[j]]
				}
				return statuses[i] < statuses[j]
			})
			for i, s := range statuses {
				if s == "" {
					continue
				}
				segs = append(segs, titleSeg{
					text:  fmt.Sprintf(" %d %s", counts[s], s),
					style: lipgloss.NewStyle().Foreground(statusColor(s)),
					drop:  10 + min(i, 5), // rarer statuses give way first
				})
			}
		}

		switch {
		case tab.list.loading:
			right = titleDimStyle.Render("loading… ")
		case tab.refreshFailed && !tab.fetchedAt.IsZero():
			right = titleWarnStyle.Render("refresh failed · data from " + ago(a.now().Sub(tab.fetchedAt)) + " ago ")
		case tab.refreshFailed:
			right = titleWarnStyle.Render("refresh failed ")
		case !tab.fetchedAt.IsZero():
			right = titleDimStyle.Render("updated " + ago(a.now().Sub(tab.fetchedAt)) + " ago ")
		}
	}

	return fitTitle(segs, right, a.width)
}

// fitTitle drops segments until the left side fits beside right, then pads
// so right sits at the end of the line.
func fitTitle(segs []titleSeg, right string, width int) string {
	render := func(segs []titleSeg) string {
		var sb strings.Builder
		for _, s := range segs {
			if s.sep {
				sb.WriteString(titleDimStyle.Render(" · "))
			}
			sb.WriteString(s.style.Render(s.text))
		}
		return sb.String()
	}
	rw := lipgloss.Width(right)
	left := render(segs)
	for lipgloss.Width(left)+rw+1 > width {
		worst := -1
		for i, s := range segs {
			if s.drop > 0 && (worst < 0 || s.drop > segs[worst].drop) {
				worst = i
			}
		}
		if worst < 0 {
			break
		}
		segs = append(segs[:worst:worst], segs[worst+1:]...)
		left = render(segs)
	}
	if lipgloss.Width(left)+rw+1 > width {
		right, rw = "", 0
	}
	if lipgloss.Width(left) > width {
		left = lipgloss.NewStyle().MaxWidth(width).Render(left)
	}
	gap := max(width-lipgloss.Width(left)-rw, 0)
	return left + strings.Repeat(" ", gap) + right
}

// ago renders a duration as "12s", "4m", "3h" or "2d".
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
