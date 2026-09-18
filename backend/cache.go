package backend

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/clobrano/jira-tabbed-tui/model"
)

const defaultTTL = 5 * time.Minute

// ListFetchedMsg is sent when a tab list fetch completes.
type ListFetchedMsg struct {
	TabIdx int
	Issues []model.Issue
	Total  int
	Err    error
	Stale  bool // true when returning stale cached data because the fetch failed
}

// DetailFetchedMsg is sent when an issue detail fetch completes.
type DetailFetchedMsg struct {
	Issue model.IssueDetail
	Err   error
}

// TransitionsFetchedMsg is sent when a transitions fetch completes.
type TransitionsFetchedMsg struct {
	Transitions []model.Transition
	Err         error
}

// ChildrenFetchedMsg is sent when the parent = KEY child-issue fetch completes.
type ChildrenFetchedMsg struct {
	ParentKey string
	Children  []model.Issue
	Err       error
}

// WriteActionDoneMsg is sent after any write action (transition, labels, comment).
type WriteActionDoneMsg struct {
	Action  string
	IssueKey string
	TabName  string
	Err     error
}

type tabCacheEntry struct {
	issues    []model.Issue
	total     int
	fetchedAt time.Time
}

type issueCacheEntry struct {
	detail    model.IssueDetail
	fetchedAt time.Time
}

// Cache holds per-tab and per-issue cached data with a TTL.
type Cache struct {
	mu     sync.Mutex
	ttl    time.Duration
	tabs   map[string]*tabCacheEntry
	issues map[string]*issueCacheEntry
}

// NewCache creates a cache with the default 5-minute TTL.
func NewCache() *Cache {
	return NewCacheWithTTL(defaultTTL)
}

// NewCacheWithTTL creates a cache with a custom TTL. Useful in tests.
func NewCacheWithTTL(ttl time.Duration) *Cache {
	return &Cache{
		ttl:    ttl,
		tabs:   make(map[string]*tabCacheEntry),
		issues: make(map[string]*issueCacheEntry),
	}
}

// FetchListCmd returns a Cmd that returns cached data if fresh, otherwise fetches.
func (c *Cache) FetchListCmd(r Runner, tabIdx int, tabName, jql string) tea.Cmd {
	return func() tea.Msg {
		c.mu.Lock()
		entry, cached := c.tabs[tabName]
		c.mu.Unlock()

		if cached && time.Since(entry.fetchedAt) < c.ttl {
			return ListFetchedMsg{TabIdx: tabIdx, Issues: entry.issues, Total: entry.total}
		}

		return c.doFetch(r, tabIdx, tabName, jql)
	}
}

// ForceFetchListCmd bypasses the TTL and always fetches fresh data.
func (c *Cache) ForceFetchListCmd(r Runner, tabIdx int, tabName, jql string) tea.Cmd {
	return func() tea.Msg {
		return c.doFetch(r, tabIdx, tabName, jql)
	}
}

func (c *Cache) doFetch(r Runner, tabIdx int, tabName, jql string) tea.Msg {
	issues, total, err := FetchIssueList(r, jql, 50)
	if err != nil {
		c.mu.Lock()
		entry, cached := c.tabs[tabName]
		c.mu.Unlock()
		if cached {
			return ListFetchedMsg{
				TabIdx: tabIdx, Issues: entry.issues,
				Total: entry.total, Err: err, Stale: true,
			}
		}
		return ListFetchedMsg{TabIdx: tabIdx, Err: err}
	}

	c.mu.Lock()
	c.tabs[tabName] = &tabCacheEntry{issues: issues, total: total, fetchedAt: time.Now()}
	c.mu.Unlock()

	return ListFetchedMsg{TabIdx: tabIdx, Issues: issues, Total: total}
}

// FetchListPageCmd appends the next page of results to the tab cache.
func (c *Cache) FetchListPageCmd(r Runner, tabIdx int, tabName, jql string, startAt int) tea.Cmd {
	return func() tea.Msg {
		issues, total, err := FetchIssueListPage(r, jql, 50, startAt)
		if err != nil {
			return ListFetchedMsg{TabIdx: tabIdx, Err: err}
		}

		c.mu.Lock()
		existing := c.tabs[tabName]
		if existing != nil {
			combined := append(existing.issues, issues...)
			c.tabs[tabName] = &tabCacheEntry{
				issues: combined, total: total, fetchedAt: existing.fetchedAt,
			}
			issues = combined
		} else {
			c.tabs[tabName] = &tabCacheEntry{
				issues: issues, total: total, fetchedAt: time.Now(),
			}
		}
		c.mu.Unlock()

		return ListFetchedMsg{TabIdx: tabIdx, Issues: issues, Total: total}
	}
}

// FetchDetailCmd returns a Cmd that fetches issue detail, using cache if fresh.
func (c *Cache) FetchDetailCmd(r Runner, key string) tea.Cmd {
	return func() tea.Msg {
		c.mu.Lock()
		entry, cached := c.issues[key]
		c.mu.Unlock()

		if cached && time.Since(entry.fetchedAt) < c.ttl {
			return DetailFetchedMsg{Issue: entry.detail}
		}

		detail, err := FetchIssueDetail(r, key)
		if err != nil {
			return DetailFetchedMsg{Err: err}
		}

		c.mu.Lock()
		c.issues[key] = &issueCacheEntry{detail: detail, fetchedAt: time.Now()}
		c.mu.Unlock()

		return DetailFetchedMsg{Issue: detail}
	}
}

// FetchChildrenCmd fetches issues whose parent field equals key.
// Jira Cloud's next-gen hierarchy doesn't expose children in the parent's fields;
// they must be queried separately via JQL.
func FetchChildrenCmd(r Runner, key string) tea.Cmd {
	return func() tea.Msg {
		issues, _, err := FetchIssueList(r, "parent = "+key, 200)
		return ChildrenFetchedMsg{ParentKey: key, Children: issues, Err: err}
	}
}

// FetchTransitionsCmd fetches available transitions for an issue via the Jira REST API.
// cfgURL is backend.url from the app config (may be empty; falls back to jira-cli's config).
func FetchTransitionsCmd(r Runner, cfgURL, key string) tea.Cmd {
	return func() tea.Msg {
		ts, err := FetchTransitionsREST(cfgURL, key)
		return TransitionsFetchedMsg{Transitions: ts, Err: err}
	}
}

// ApplyTransitionCmd applies a transition and emits WriteActionDoneMsg.
func ApplyTransitionCmd(r Runner, key, transitionName, tabName string) tea.Cmd {
	return func() tea.Msg {
		err := ApplyTransition(r, key, transitionName)
		return WriteActionDoneMsg{Action: "transition", IssueKey: key, TabName: tabName, Err: err}
	}
}

// AddLabelsCmd adds labels and emits WriteActionDoneMsg.
func AddLabelsCmd(r Runner, key string, labels []string, tabName string) tea.Cmd {
	return func() tea.Msg {
		err := AddLabels(r, key, labels)
		return WriteActionDoneMsg{Action: "add labels", IssueKey: key, TabName: tabName, Err: err}
	}
}

// AddCommentCmd posts a comment and emits WriteActionDoneMsg.
func AddCommentCmd(r Runner, key, body, tabName string) tea.Cmd {
	return func() tea.Msg {
		err := AddComment(r, key, body)
		return WriteActionDoneMsg{Action: "add comment", IssueKey: key, TabName: tabName, Err: err}
	}
}

// Invalidate removes cached data for the given tab name and/or issue key.
func (c *Cache) Invalidate(tabName, issueKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if tabName != "" {
		delete(c.tabs, tabName)
	}
	if issueKey != "" {
		delete(c.issues, issueKey)
	}
}
