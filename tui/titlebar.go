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

	var rights []string
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

		rights = a.freshness(tab)
	}

	return fitTitle(segs, rights, a.width)
}

// freshness describes how current the tab's list is, longest first; the
// title bar shows the longest that fits, e.g.
//
//	updated 2m ago · auto-refresh in 3m · r refresh
func (a App) freshness(tab tabState) []string {
	key := a.cfg.Keybindings.ForceRefresh
	switch {
	case tab.list.loading:
		return []string{titleDimStyle.Render("loading… ")}
	case tab.refreshing && tab.refreshTotal > 0:
		return []string{
			titleDimStyle.Render(fmt.Sprintf("refreshing %d of %d… ", tab.refreshLoaded, tab.refreshTotal)),
			titleDimStyle.Render("refreshing… "),
		}
	case tab.refreshing:
		return []string{titleDimStyle.Render("refreshing… ")}
	case tab.refreshFailed && !tab.fetchedAt.IsZero():
		data := "data from " + ago(a.now().Sub(tab.fetchedAt)) + " ago"
		return []string{
			titleWarnStyle.Render("refresh failed · " + data + " · " + key + " retry "),
			titleWarnStyle.Render("refresh failed · " + data + " "),
			titleWarnStyle.Render("refresh failed "),
		}
	case tab.refreshFailed:
		return []string{titleWarnStyle.Render("refresh failed · " + key + " retry "), titleWarnStyle.Render("refresh failed ")}
	case !tab.fetchedAt.IsZero():
		updated := "updated " + ago(a.now().Sub(tab.fetchedAt)) + " ago"
		auto := "auto-refresh soon"
		if left := a.cache.TTL() - a.now().Sub(tab.fetchedAt); left >= time.Minute {
			auto = "auto-refresh in " + ago(left.Round(time.Minute))
		} else if left > 0 {
			auto = "auto-refresh in <1m"
		}
		return []string{
			titleDimStyle.Render(updated + " · " + auto + " · " + key + " refresh "),
			titleDimStyle.Render(updated + " · " + key + " refresh "),
			titleDimStyle.Render(updated + " "),
		}
	}
	return nil
}

// fitTitle drops segments until the left side fits beside the right one,
// then pads so it sits at the end of the line. rights are alternatives,
// longest first: a shorter one is used rather than drop the issue count.
func fitTitle(segs []titleSeg, rights []string, width int) string {
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
	fit := func(right string) (string, bool) {
		segs := segs
		rw := lipgloss.Width(right)
		left := render(segs)
		for lipgloss.Width(left)+rw+1 > width {
			worst := -1
			for i, s := range segs {
				if s.drop > 0 && (worst < 0 || s.drop > segs[worst].drop) {
					worst = i
				}
			}
			if worst < 0 || segs[worst].drop <= 1 {
				return "", false
			}
			segs = append(segs[:worst:worst], segs[worst+1:]...)
			left = render(segs)
		}
		return left + strings.Repeat(" ", max(width-lipgloss.Width(left)-rw, 0)) + right, true
	}
	for _, right := range rights {
		if line, ok := fit(right); ok {
			return line
		}
	}
	if line, ok := fit(""); ok {
		return line
	}
	// Too narrow even for the issue count: only what never drops, cut to width.
	var keep []titleSeg
	for _, s := range segs {
		if s.drop == 0 {
			keep = append(keep, s)
		}
	}
	left := render(keep)
	if lipgloss.Width(left) > width {
		left = lipgloss.NewStyle().MaxWidth(width).Render(left)
	}
	return left + strings.Repeat(" ", max(width-lipgloss.Width(left), 0))
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
