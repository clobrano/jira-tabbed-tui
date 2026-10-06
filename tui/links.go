package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/clobrano/jira-tabbed-tui/backend"
	"github.com/clobrano/jira-tabbed-tui/model"
)

// linkGroup orders the Links tab: Jira issues first, then pull requests,
// then any other web link.
type linkGroup int

const (
	linkGroupJira linkGroup = iota
	linkGroupPR
	linkGroupWeb
)

var linkGroupTitles = map[linkGroup]string{
	linkGroupJira: "Jira issues",
	linkGroupPR:   "Pull requests",
	linkGroupWeb:  "Web links",
}

func groupOf(l model.IssueLink) linkGroup {
	switch {
	case l.URL == "":
		return linkGroupJira
	case backend.IsPullRequestURL(l.URL):
		return linkGroupPR
	default:
		return linkGroupWeb
	}
}

// linkID identifies a link independently of its position, so the cursor can
// follow it while links are re-sorted or appended asynchronously.
func linkID(l model.IssueLink) string {
	return l.Type + "\x00" + l.Key + "\x00" + l.URL
}

// sortLinks groups links (Jira, PRs, web) and sorts within each group: Jira
// issues by key (PROJ-9 before PROJ-10), PRs by repository and number, web
// links by title.
func sortLinks(links []model.IssueLink) {
	sort.SliceStable(links, func(i, j int) bool {
		a, b := links[i], links[j]
		ga, gb := groupOf(a), groupOf(b)
		if ga != gb {
			return ga < gb
		}
		switch ga {
		case linkGroupJira:
			if a.Key != b.Key {
				return issueKeyLess(a.Key, b.Key)
			}
			return a.Type < b.Type
		case linkGroupPR:
			return trailingNumberLess(a.URL, b.URL, "/")
		default:
			return strings.ToLower(a.Key) < strings.ToLower(b.Key)
		}
	})
}

// trailingNumberLess compares strings like ".../pull/9" and ".../pull/10" by
// their prefix, then numerically by what follows the last sep.
func trailingNumberLess(a, b, sep string) bool {
	a = strings.TrimRight(a, sep)
	b = strings.TrimRight(b, sep)
	ai, bi := strings.LastIndex(a, sep), strings.LastIndex(b, sep)
	if ai < 0 || bi < 0 || a[:ai] != b[:bi] {
		return a < b
	}
	an, aerr := strconv.Atoi(a[ai+len(sep):])
	bn, berr := strconv.Atoi(b[bi+len(sep):])
	if aerr != nil || berr != nil {
		return a < b
	}
	return an < bn
}

// linkRowLine returns the line of links[i] in the rendered Links tab: the
// column header and separator (2 lines), then for each group a title line,
// its rows, and a blank line between groups. links must be sorted.
func linkRowLine(links []model.IssueLink, i int) int {
	line := 2
	prev := linkGroup(-1)
	for j := 0; j <= i && j < len(links); j++ {
		if g := groupOf(links[j]); g != prev {
			if prev != -1 {
				line++ // blank line between groups
			}
			line++ // group title
			prev = g
		}
		if j < i {
			line++
		}
	}
	return line
}

var (
	linkColHdrStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Bold(true)
	linkGroupStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8888ff")).Bold(true)
	linkRelStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	linkKeyStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#5555ff")).Bold(true)
	linkWebTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5588ff")).Underline(true)
	linkTypeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#bbbbbb"))
	linkSummaryStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#dddddd"))
	linkURLStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#5588ff")).Faint(true)
	linkStatusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#aaaaaa"))
	linkCursorStyle   = lipgloss.NewStyle().
				Background(lipgloss.Color("#222255")).
				Foreground(lipgloss.Color("#ffffff")).
				Bold(true)

	prStateColors = map[string]lipgloss.Color{
		backend.PROpen:   lipgloss.Color("#3fb950"),
		backend.PRDraft:  lipgloss.Color("#8b949e"),
		backend.PRMerged: lipgloss.Color("#a371f7"),
		backend.PRClosed: lipgloss.Color("#f85149"),
	}
)

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > w {
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}

// renderLinkTable draws the Links tab: one section per group, columns
// RELATION · KEY/TITLE · TYPE · SUMMARY/URL · STATUS. links must be sorted.
func renderLinkTable(links []model.IssueLink, cursor, width int) string {
	if len(links) == 0 {
		return statusIdleStyle.Padding(1, 1).Render("No linked issues.")
	}
	if width < 10 {
		width = 10
	}
	const relW, keyW, typeW, statusW = 16, 13, 10, 12
	// A row is a 2-space indent + 5 columns joined by 2 spaces, and must fit
	// in width-2 so the highlighted row (rendered at width-2) doesn't wrap.
	sumW := width - 2 - (2 + relW + keyW + typeW + statusW + 4*2)
	if sumW < 8 {
		sumW = 8
	}

	var sb strings.Builder
	sb.WriteString(linkColHdrStyle.Render("  "+fit("RELATION", relW)+"  "+fit("KEY / TITLE", keyW)+"  "+
		fit("TYPE", typeW)+"  "+fit("SUMMARY / URL", sumW)+"  STATUS") + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")).
		Render(strings.Repeat("─", max(width-2, 0))) + "\n")

	counts := map[linkGroup]int{}
	for _, l := range links {
		counts[groupOf(l)]++
	}

	prev := linkGroup(-1)
	for i, l := range links {
		g := groupOf(l)
		if g != prev {
			if prev != -1 {
				sb.WriteString("\n")
			}
			sb.WriteString(linkGroupStyle.Render(fmt.Sprintf(" %s (%d)", linkGroupTitles[g], counts[g])) + "\n")
			prev = g
		}

		typ, sum, status := l.IssueType, l.Summary, l.Status
		switch g {
		case linkGroupPR:
			typ = "PR"
		case linkGroupWeb:
			typ = "web"
		}
		if g != linkGroupJira {
			sum = l.URL
		}

		cells := []string{fit(l.Type, relW), fit(l.Key, keyW), fit(typ, typeW), fit(sum, sumW), fit(status, statusW)}
		if i == cursor {
			sb.WriteString(linkCursorStyle.Width(width-2).Render("  "+strings.Join(cells, "  ")) + "\n")
			continue
		}
		keyStyle, sumStyle, stStyle := linkKeyStyle, linkSummaryStyle, linkStatusStyle
		if g != linkGroupJira {
			keyStyle, sumStyle = linkWebTitleStyle, linkURLStyle
		}
		if c, ok := prStateColors[status]; ok && g == linkGroupPR {
			stStyle = lipgloss.NewStyle().Foreground(c).Bold(true)
		}
		sb.WriteString("  " + linkRelStyle.Render(cells[0]) + "  " + keyStyle.Render(cells[1]) + "  " +
			linkTypeStyle.Render(cells[2]) + "  " + sumStyle.Render(cells[3]) + "  " + stStyle.Render(cells[4]) + "\n")
	}
	return sb.String()
}

var (
	crumbStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	crumbCurrentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#333366")).Bold(true)
	crumbForwardStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
)

// renderBreadcrumb shows the detail history, e.g.
// "depth 2/3  PROJ-1 › PROJ-7 › PROJ-9", with the current entry highlighted and
// forward entries dimmed. Leading entries are elided to fit width.
func renderBreadcrumb(keys []string, idx, width int) string {
	if len(keys) < 2 || idx < 0 || idx >= len(keys) {
		return ""
	}
	depth := fmt.Sprintf("depth %d/%d  ", idx+1, len(keys))
	sep := " › "

	start := 0
	plainWidth := func(from int) int {
		w := len([]rune(depth))
		if from > 0 {
			w += len([]rune("…" + sep))
		}
		for i := from; i < len(keys); i++ {
			if i > from {
				w += len([]rune(sep))
			}
			w += len([]rune(keys[i])) + 2 // current entry is padded
		}
		return w
	}
	for start < idx && plainWidth(start) > width {
		start++
	}

	var parts []string
	if start > 0 {
		parts = append(parts, crumbStyle.Render("…"))
	}
	for i := start; i < len(keys); i++ {
		switch {
		case i == idx:
			parts = append(parts, crumbCurrentStyle.Render(" "+keys[i]+" "))
		case i > idx:
			parts = append(parts, crumbForwardStyle.Render(keys[i]))
		default:
			parts = append(parts, crumbStyle.Render(keys[i]))
		}
	}
	return crumbStyle.Render(depth) + strings.Join(parts, crumbStyle.Render(sep))
}
