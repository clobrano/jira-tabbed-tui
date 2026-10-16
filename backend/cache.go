package backend

import (
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/clobrano/jira-tabbed-tui/model"
)

// defaultTTL is how long an issue's details stay cached; lists, whose age
// is shown in the title bar along with how to refresh them, last longer.
const (
	defaultTTL     = 5 * time.Minute
	defaultListTTL = 30 * time.Minute
)

// ListFetchedMsg is sent when a page of a tab's list arrives. Issues holds
// everything loaded so far; while Next is set, more pages remain.
type ListFetchedMsg struct {
	TabIdx    int
	Gen       int // load generation; replies to a superseded load are ignored
	Issues    []model.Issue
	Total     int    // matching issues, if known (0 = unknown)
	Next      string // cursor of the next page ("" = nothing more to load)
	Truncated bool   // jira-cli fallback returned a full page; more may exist
	Err       error
	Stale     bool      // true when returning stale cached data because the fetch failed
	At        time.Time // when the returned issues were fetched from Jira
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
	Action   string
	IssueKey string
	TabName  string
	Err      error
}

type tabCacheEntry struct {
	issues    []model.Issue
	total     int
	next      string // cursor of the next page to load ("" = complete)
	truncated bool
	fetchedAt time.Time
}

func (e *tabCacheEntry) msg(tabIdx, gen int) ListFetchedMsg {
	return ListFetchedMsg{
		TabIdx: tabIdx, Gen: gen, Issues: e.issues, Total: e.total,
		Next: e.next, Truncated: e.truncated, At: e.fetchedAt,
	}
}

type issueCacheEntry struct {
	detail    model.IssueDetail
	fetchedAt time.Time
}

// Cache holds per-tab and per-issue cached data with a TTL.
type Cache struct {
	mu      sync.Mutex
	ttl     time.Duration // issue details
	listTTL time.Duration
	tabs    map[string]*tabCacheEntry
	issues  map[string]*issueCacheEntry
	now     func() time.Time
}

// NewCache creates a cache that keeps lists for 30 minutes and issue
// details for 5.
func NewCache() *Cache {
	c := NewCacheWithTTL(defaultTTL)
	c.listTTL = defaultListTTL
	return c
}

// NewCacheWithTTL creates a cache with one custom TTL for lists and issue
// details. Useful in tests.
func NewCacheWithTTL(ttl time.Duration) *Cache {
	return &Cache{
		ttl:     ttl,
		listTTL: ttl,
		tabs:    make(map[string]*tabCacheEntry),
		issues:  make(map[string]*issueCacheEntry),
		now:     time.Now,
	}
}

// SetClock replaces the cache's clock. Useful in tests.
func (c *Cache) SetClock(now func() time.Time) { c.now = now }

// ListTTL is how long a cached list counts as fresh.
func (c *Cache) ListTTL() time.Duration { return c.listTTL }

// FetchListCmd returns the tab's cached list if fresh (including a load
// still in progress, which the caller resumes from Next), otherwise fetches
// the first page.
func (c *Cache) FetchListCmd(fetch PageFunc, tabIdx, gen int, tabName, jql string) tea.Cmd {
	return func() tea.Msg {
		c.mu.Lock()
		entry, cached := c.tabs[tabName]
		c.mu.Unlock()

		if cached && c.now().Sub(entry.fetchedAt) < c.listTTL {
			return entry.msg(tabIdx, gen)
		}

		return c.doFetch(fetch, tabIdx, gen, tabName, jql)
	}
}

// ForceFetchListCmd bypasses the TTL and always fetches the first page.
func (c *Cache) ForceFetchListCmd(fetch PageFunc, tabIdx, gen int, tabName, jql string) tea.Cmd {
	return func() tea.Msg {
		return c.doFetch(fetch, tabIdx, gen, tabName, jql)
	}
}

func (c *Cache) doFetch(fetch PageFunc, tabIdx, gen int, tabName, jql string) tea.Msg {
	page, err := fetch(jql, "")
	if err != nil {
		c.mu.Lock()
		entry, cached := c.tabs[tabName]
		c.mu.Unlock()
		if cached {
			msg := entry.msg(tabIdx, gen)
			msg.Next, msg.Err, msg.Stale = "", err, true
			return msg
		}
		return ListFetchedMsg{TabIdx: tabIdx, Gen: gen, Err: err}
	}

	entry := &tabCacheEntry{
		issues: page.Issues, total: page.Total, next: page.Next,
		truncated: page.Truncated, fetchedAt: c.now(),
	}
	if entry.next == "" {
		entry.total = len(entry.issues)
	}
	c.mu.Lock()
	c.tabs[tabName] = entry
	c.mu.Unlock()
	return entry.msg(tabIdx, gen)
}

// FetchNextPageCmd loads the page at cursor and appends it to the tab's
// cached list. If the cache has moved on (another load already fetched that
// page, or a refresh replaced the list), it returns the cache as it is, so
// the caller carries on from its Next. A failed page keeps what was loaded
// and stops there.
func (c *Cache) FetchNextPageCmd(fetch PageFunc, tabIdx, gen int, tabName, jql, cursor string) tea.Cmd {
	return func() tea.Msg {
		page, err := fetch(jql, cursor)

		c.mu.Lock()
		defer c.mu.Unlock()
		entry, ok := c.tabs[tabName]
		if !ok {
			return nil
		}
		if entry.next != cursor {
			return entry.msg(tabIdx, gen)
		}
		if err != nil {
			msg := entry.msg(tabIdx, gen)
			msg.Next, msg.Err = "", err
			return msg
		}
		entry.issues = append(append([]model.Issue(nil), entry.issues...), page.Issues...)
		entry.next = page.Next
		if page.Total > 0 {
			entry.total = page.Total
		}
		// Once complete the count is exact; Cloud's total is an estimate.
		if entry.next == "" || entry.total < len(entry.issues) {
			entry.total = len(entry.issues)
		}
		return entry.msg(tabIdx, gen)
	}
}

// FetchDetailCmd returns a Cmd that fetches issue detail, using cache if fresh.
func (c *Cache) FetchDetailCmd(r Runner, key string) tea.Cmd {
	return func() tea.Msg {
		c.mu.Lock()
		entry, cached := c.issues[key]
		c.mu.Unlock()

		if cached && c.now().Sub(entry.fetchedAt) < c.ttl {
			return DetailFetchedMsg{Issue: entry.detail}
		}

		detail, err := FetchIssueDetail(r, key)
		if err != nil {
			return DetailFetchedMsg{Err: err}
		}

		c.mu.Lock()
		c.issues[key] = &issueCacheEntry{detail: detail, fetchedAt: c.now()}
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
// Options nil + FallbackMsg non-empty = no predefined values, show text input with the message.
// Options nil + FallbackMsg empty = dedicated overlay handles this field (status/assignee/labels).
type FieldOptionsFetchedMsg struct {
	FieldID      string
	FieldName    string
	IssueKey     string
	Options      []string
	CurrentValue string
	FallbackMsg  string // neutral info shown when falling back to free text
	Err          error
}

// FetchFieldOptionsCmd resolves editable options for any field, then emits FieldOptionsFetchedMsg.
// Strategy per field type:
//   - status/assignee/labels → dedicated overlays (nil Options, empty FallbackMsg)
//   - priority               → GET /rest/api/3/priority
//   - fixVersions/versions   → GET /rest/api/3/project/{key}/versions
//   - affectsVersions        → same as versions
//   - components             → GET /rest/api/3/project/{key}/components
//   - issuetype              → GET /rest/api/3/issuetype
//   - customfield_*          → context/option chain; nil if not a select field
//   - anything else          → skip fetch, free text with FallbackMsg
func FetchFieldOptionsCmd(cfgURL, issueKey, fieldID, fieldName, currentValue string) tea.Cmd {
	return func() tea.Msg {
		base := FieldOptionsFetchedMsg{
			FieldID: fieldID, FieldName: fieldName,
			IssueKey: issueKey, CurrentValue: currentValue,
		}

		var (
			opts []string
			err  error
		)

		switch fieldID {
		case "status", "assignee", "labels":
			// Handled by dedicated overlays — return as-is (nil Options, no FallbackMsg).
			return base

		case "priority":
			opts, err = FetchPrioritiesREST(cfgURL)

		case "fixVersions", "versions", "affectsVersions":
			opts, err = FetchProjectVersionsREST(cfgURL, issueKey)

		case "components":
			opts, err = FetchProjectComponentsREST(cfgURL, issueKey)

		case "issuetype":
			opts, err = FetchIssueTypesREST(cfgURL)

		default:
			if len(fieldID) > 12 && fieldID[:12] == "customfield_" {
				opts, err = FetchCustomFieldOptionsREST(cfgURL, fieldID)
			}
			// For unknown built-in fields, opts stays nil → free text.
		}

		if err != nil {
			base.Err = err
			return base
		}
		if len(opts) == 0 {
			base.FallbackMsg = "No predefined values found — enter manually"
		}
		base.Options = opts
		return base
	}
}

// EditFieldCmd applies a field value change via jira issue edit.
// replace=true overwrites multi-value fields; false appends.
func EditFieldCmd(r Runner, cfgURL, issueKey, fieldID, value, tabName string, replace bool) tea.Cmd {
	return func() tea.Msg {
		err := EditField(r, cfgURL, issueKey, fieldID, value, replace)
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

// DoAssignCmd assigns the issue via the Jira REST API using the account ID.
func DoAssignCmd(cfgURL, key, accountID, displayName, tabName string) tea.Cmd {
	return func() tea.Msg {
		err := AssignIssueREST(cfgURL, key, accountID)
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
