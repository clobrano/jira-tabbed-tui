package backend

import (
	"strings"
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

// RemoteLinksFetchedMsg is sent when the remote/web links fetch completes.
type RemoteLinksFetchedMsg struct {
	IssueKey string
	Links    []model.IssueLink
	Err      error
}

// AssignSearchDoneMsg is sent when an assignable-user search completes.
type AssignSearchDoneMsg struct {
	Users []model.User
	Err   error
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
		jql := "parent = " + key
		if project, _, ok := strings.Cut(key, "-"); ok {
			jql = "project = " + project + " AND " + jql
		}
		issues, _, err := FetchIssueList(r, jql, 100)
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

// FieldOptionsFetchedMsg is sent when field option values have been resolved.
// Options is nil when the field has no enumerable values (use text input fallback).
type FieldOptionsFetchedMsg struct {
	FieldID      string
	FieldName    string
	IssueKey     string
	Options      []string // nil = no options (free text); non-nil = show picker
	CurrentValue string
	Err          error
}

// FetchFieldOptionsCmd resolves the editable options for a field, then emits FieldOptionsFetchedMsg.
// It handles the special cases (status, assignee, labels, priority) and falls back to
// custom-field option lookup or free text for everything else.
func FetchFieldOptionsCmd(cfgURL, issueKey, fieldID, fieldName, currentValue string) tea.Cmd {
	return func() tea.Msg {
		base := FieldOptionsFetchedMsg{
			FieldID: fieldID, FieldName: fieldName,
			IssueKey: issueKey, CurrentValue: currentValue,
		}
		switch fieldID {
		case "priority":
			opts, err := FetchPrioritiesREST(cfgURL)
			base.Options, base.Err = opts, err
		case "status", "assignee", "labels":
			// These are handled by dedicated overlays; signal with a nil options slice
			// and a sentinel Err so the caller knows to route differently.
			base.Options = nil
		default:
			if len(fieldID) > 12 && fieldID[:12] == "customfield_" {
				opts, err := FetchCustomFieldOptionsREST(cfgURL, fieldID)
				if err != nil {
					base.Err = err
				} else {
					base.Options = opts // may be nil → text input
				}
			}
			// built-in text fields (summary, duedate, …): leave Options nil
		}
		return base
	}
}

// EditFieldCmd applies a field value change via jira issue edit.
func EditFieldCmd(r Runner, issueKey, fieldID, value, tabName string) tea.Cmd {
	return func() tea.Msg {
		err := EditField(r, issueKey, fieldID, value)
		return WriteActionDoneMsg{Action: "edit " + fieldID, IssueKey: issueKey, TabName: tabName, Err: err}
	}
}

// FetchRemoteLinksCmd fetches web/remote links for an issue via the Jira REST API.
func FetchRemoteLinksCmd(cfgURL, key string) tea.Cmd {
	return func() tea.Msg {
		links, err := FetchRemoteLinksREST(cfgURL, key)
		return RemoteLinksFetchedMsg{IssueKey: key, Links: links, Err: err}
	}
}

// AssignSearchCmd searches for assignable users and emits AssignSearchDoneMsg.
func AssignSearchCmd(cfgURL, issueKey, query string) tea.Cmd {
	return func() tea.Msg {
		users, err := SearchAssignableUsersREST(cfgURL, issueKey, query)
		return AssignSearchDoneMsg{Users: users, Err: err}
	}
}

// DoAssignCmd assigns the issue and emits WriteActionDoneMsg.
func DoAssignCmd(r Runner, key, login, displayName, tabName string) tea.Cmd {
	return func() tea.Msg {
		err := AssignIssue(r, key, login)
		return WriteActionDoneMsg{Action: "assign to " + displayName, IssueKey: key, TabName: tabName, Err: err}
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
