package tui

import (
	"strings"
	"testing"

	"github.com/clobrano/jira-tabbed-tui/model"
)

func jira(key string) model.IssueLink { return model.IssueLink{Type: "relates to", Key: key} }

func web(title, url string) model.IssueLink {
	return model.IssueLink{Type: "mentioned in", Key: title, URL: url}
}

func TestSortLinksGroupsAndOrders(t *testing.T) {
	links := []model.IssueLink{
		web("Zeta doc", "https://docs.example.com/z"),
		jira("P-151"),
		web("PR 10", "https://github.com/acme/web/pull/10"),
		jira("P-98"),
		web("alpha doc", "https://docs.example.com/a"),
		web("PR 9", "https://github.com/acme/web/pull/9"),
		jira("P-152"),
	}
	sortLinks(links)
	var got []string
	for _, l := range links {
		got = append(got, l.Key)
	}
	want := "P-98 P-151 P-152 PR 9 PR 10 alpha doc Zeta doc"
	if strings.Join(got, " ") != want {
		t.Errorf("order = %q\nwant    %q", strings.Join(got, " "), want)
	}
}

func TestLinkRowLineAccountsForGroupHeaders(t *testing.T) {
	links := []model.IssueLink{jira("P-1"), jira("P-2"), web("doc", "https://x/doc")}
	// 2 header lines, "Jira issues" title, P-1, P-2, blank, "Web links" title, doc.
	for i, want := range []int{3, 4, 7} {
		if got := linkRowLine(links, i); got != want {
			t.Errorf("linkRowLine(%d) = %d, want %d", i, got, want)
		}
	}
	// The rendered table agrees, and no row (highlighted or not) is wider
	// than the space it is drawn in.
	for _, cursor := range []int{-1, 0} {
		for _, line := range strings.Split(renderLinkTable(links, cursor, 100), "\n") {
			if w := len([]rune(stripANSI(line))); w > 98 {
				t.Errorf("cursor=%d: row is %d wide, max 98: %q", cursor, w, stripANSI(line))
			}
		}
	}
	lines := strings.Split(renderLinkTable(links, -1, 100), "\n")
	for i, l := range links {
		if !strings.Contains(lines[linkRowLine(links, i)], l.Key) {
			t.Errorf("row for %s not at line %d: %q", l.Key, linkRowLine(links, i), lines[linkRowLine(links, i)])
		}
	}
}

func TestCursorFollowsSelectedLinkWhenLinksArrive(t *testing.T) {
	a := newTestApp(t)
	a.view = viewDetail
	a = send(t, a, fetched("P-1", "P-5", "P-7"), keyMsg("right"), keyMsg("right"), keyMsg("j"))
	if l, _ := a.detail.SelectedLink(); l.Key != "P-7" {
		t.Fatalf("selected %q, want P-7", l.Key)
	}
	// A child that sorts before P-7 arrives: the selection must stay on P-7.
	a.detail = a.detail.AppendLinks([]model.IssueLink{jira("P-6")})
	if l, _ := a.detail.SelectedLink(); l.Key != "P-7" {
		t.Errorf("after append selected %q, want P-7", l.Key)
	}
}

func TestRenderBreadcrumb(t *testing.T) {
	if got := renderBreadcrumb([]string{"P-1"}, 0, 80); got != "" {
		t.Errorf("single entry should not render a breadcrumb, got %q", got)
	}
	got := stripANSI(renderBreadcrumb([]string{"P-1", "P-2", "P-3"}, 1, 80))
	if got != "depth 2/3  P-1 ›  P-2  › P-3" {
		t.Errorf("breadcrumb = %q", got)
	}
	// Too narrow: leading entries are elided but the current one stays.
	got = stripANSI(renderBreadcrumb([]string{"PROJ-1", "PROJ-2", "PROJ-3", "PROJ-4"}, 3, 30))
	if !strings.HasPrefix(got, "depth 4/4  … › ") || !strings.Contains(got, "PROJ-4") {
		t.Errorf("narrow breadcrumb = %q", got)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
