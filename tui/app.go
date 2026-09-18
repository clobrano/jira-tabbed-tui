package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clobrano/jira-tabbed-tui/backend"
	"github.com/clobrano/jira-tabbed-tui/config"
	"github.com/clobrano/jira-tabbed-tui/model"
	"github.com/clobrano/jira-tabbed-tui/tui/actions"
)

type viewMode int

const (
	viewList   viewMode = iota
	viewDetail viewMode = iota
)

type overlayMode int

const (
	overlayNone          overlayMode = iota
	overlayTransition    overlayMode = iota
	overlayLabels        overlayMode = iota
	overlayComment       overlayMode = iota
	overlayHelp          overlayMode = iota
	overlayFields        overlayMode = iota
	overlayFieldsConfirm overlayMode = iota
	overlayAddTab        overlayMode = iota
	overlayConfirmDelete overlayMode = iota
	overlayEditJQL       overlayMode = iota
	overlaySort          overlayMode = iota
	overlayAssign        overlayMode = iota
)

// tabState holds per-tab runtime state.
type tabState struct {
	name   string
	jql    string
	list   IssueList
	// Search tab only
	search   SearchInput
	isSearch bool
}

// FieldsOverlayMsg is sent to open the field-discovery overlay.
type FieldsOverlayMsg struct {
	Fields []model.Field
}

// browserOpenDoneMsg is sent after the "open in browser" action.
// Kept separate from WriteActionDoneMsg so it does not trigger a cache refetch.
type browserOpenDoneMsg struct {
	err error
}

// App is the root Bubbletea model.
type App struct {
	cfg          config.Config
	configPath   string
	runner       backend.Runner
	cache        *backend.Cache
	tabs         []tabState
	activeTab    int
	view         viewMode
	overlay      overlayMode
	detail       Detail
	transition   actions.TransitionModel
	labels       actions.LabelsModel
	comment      actions.CommentModel
	assign       actions.AssignModel
	addTabM      actions.AddTabModel
	editJQLM     actions.EditJQLModel
	deleteTabIdx int // tab index pending confirmation
	help         HelpOverlay
	statusLine   StatusLine
	tabBar       TabBar
	sortCursor    int             // cursor in the sort picker overlay
	detailHistory []string        // issue keys navigated in the current detail session
	detailHistIdx int             // current position in detailHistory (-1 = empty)
	fields        []model.Field
	fieldSelected map[string]bool // working checkbox state (field ID → in sidebar)
	fieldOriginal map[string]bool // snapshot when overlay opened
	fieldCursor   int             // cursor row within filtered list
	fieldScroll   int             // first visible row
	fieldInput    textinput.Model
	width        int
	height       int
	globalSpinner spinner.Model
}

// New creates the root App model.
func New(cfg config.Config, configPath string, runner backend.Runner) App {
	cache := backend.NewCache()

	// Build tab states: index 0 = Search, then config tabs.
	tabs := make([]tabState, 0, 1+len(cfg.Tabs))
	tabs = append(tabs, tabState{
		name:     "Search",
		isSearch: true,
		list:     NewIssueList(cfg.List.Columns).SetLoading(false),
		search:   NewSearchInput(),
	})
	for _, t := range cfg.Tabs {
		tabs = append(tabs, tabState{
			name: t.Name,
			jql:  t.JQL,
			list: NewIssueList(cfg.List.Columns),
		})
	}

	tabNames := make([]string, len(tabs))
	for i, t := range tabs {
		tabNames[i] = t.name
	}

	fi := textinput.New()
	fi.Placeholder = "type to filter…"
	fi.CharLimit = 64

	gs := spinner.New()
	gs.Spinner = spinner.Dot
	gs.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#5555ff"))

	// Start on the first non-search tab if one exists.
	initialTab := 0
	if len(tabs) > 1 {
		initialTab = 1
		tabs[1].list = tabs[1].list.SetLoading(true)
	} else {
		// Only the Search tab exists; focus the JQL input immediately.
		tabs[0].search = tabs[0].search.Focus()
	}

	return App{
		cfg:           cfg,
		configPath:    configPath,
		runner:        runner,
		cache:         cache,
		tabs:          tabs,
		activeTab:     initialTab,
		view:          viewList,
		overlay:       overlayNone,
		detail:        NewDetail(cfg),
		help:          NewHelpOverlay(cfg.Keybindings),
		statusLine:    StatusLine{},
		tabBar:        NewTabBar(tabNames),
		fieldInput:    fi,
		globalSpinner: gs,
		detailHistIdx: -1,
	}
}

// Init starts the spinner and triggers the initial fetch for the first non-search tab.
func (a App) Init() tea.Cmd {
	cmds := []tea.Cmd{a.globalSpinner.Tick}
	if a.activeTab > 0 && a.activeTab < len(a.tabs) {
		t := a.tabs[a.activeTab]
		cmds = append(cmds, a.cache.FetchListCmd(a.runner, a.activeTab, t.name, t.jql))
	}
	return tea.Batch(cmds...)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ── window size ──────────────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a = a.resizeAll()
		return a, nil

	// ── spinner tick ─────────────────────────────────────────────────────────
	case spinner.TickMsg:
		var cmd tea.Cmd
		a.globalSpinner, cmd = a.globalSpinner.Update(msg)
		for i := range a.tabs {
			a.tabs[i].list, _ = a.tabs[i].list.SpinnerTick()
		}
		a.detail, _ = a.detail.SpinnerTick()
		return a, cmd

	// ── list fetched ─────────────────────────────────────────────────────────
	case backend.ListFetchedMsg:
		if msg.TabIdx >= 0 && msg.TabIdx < len(a.tabs) {
			tab := &a.tabs[msg.TabIdx]
			tab.list = tab.list.SetIssues(msg.Issues, msg.Total, msg.Stale, msg.Err)
		}
		if msg.Err != nil {
			a.statusLine = a.statusLine.SetMessage(MapCLIError(msg.Err.Error()), true)
		} else {
			a.statusLine = a.statusLine.SetMessage("", false)
		}
		return a, nil

	// ── detail fetched ───────────────────────────────────────────────────────
	case backend.DetailFetchedMsg:
		if msg.Err != nil {
			a.detail = a.detail.SetError(msg.Err)
			a.statusLine = a.statusLine.SetMessage(MapCLIError(msg.Err.Error()), true)
			return a, nil
		}
		a.detail = a.detail.SetIssue(msg.Issue)
		a.statusLine = a.statusLine.SetMessage("", false)
		// Fetch children and web/remote links in parallel.
		return a, tea.Batch(
			backend.FetchChildrenCmd(a.runner, msg.Issue.Key),
			backend.FetchRemoteLinksCmd(a.cfg.Backend.URL, msg.Issue.Key),
		)

	// ── children fetched ─────────────────────────────────────────────────────
	case backend.ChildrenFetchedMsg:
		if msg.Err == nil && len(msg.Children) > 0 && a.detail.IssueKey() == msg.ParentKey {
			links := make([]model.IssueLink, len(msg.Children))
			for i, ch := range msg.Children {
				links[i] = model.IssueLink{
					Type:    "child issue",
					Key:     ch.Key,
					Summary: ch.Summary,
					Status:  ch.Status,
				}
			}
			a.detail = a.detail.AppendLinks(links)
		}
		return a, nil

	// ── remote/web links fetched ─────────────────────────────────────────────
	case backend.RemoteLinksFetchedMsg:
		if msg.Err == nil && len(msg.Links) > 0 && a.detail.IssueKey() == msg.IssueKey {
			a.detail = a.detail.AppendLinks(msg.Links)
		}
		return a, nil

	// ── transitions fetched ──────────────────────────────────────────────────
	case backend.TransitionsFetchedMsg:
		a.transition = a.transition.SetTransitions(msg.Transitions, msg.Err)
		if msg.Err != nil {
			a.statusLine = a.statusLine.SetMessage(MapCLIError(msg.Err.Error()), true)
		}
		return a, nil

	// ── browser open done (no cache refetch) ────────────────────────────────
	case browserOpenDoneMsg:
		if msg.err != nil {
			a.statusLine = a.statusLine.SetMessage("open browser failed: "+msg.err.Error(), true)
		} else {
			a.statusLine = a.statusLine.SetMessage("opened in browser", false)
		}
		return a, nil

	// ── write action done ────────────────────────────────────────────────────
	case backend.WriteActionDoneMsg:
		a.overlay = overlayNone
		if msg.Err != nil {
			a.statusLine = a.statusLine.SetMessage(msg.Action+" failed: "+msg.Err.Error(), true)
			return a, nil
		}
		a.statusLine = a.statusLine.SetMessage(msg.Action+" succeeded", false)
		a.cache.Invalidate(msg.TabName, msg.IssueKey)
		var cmds []tea.Cmd
		// Re-fetch detail and the originating tab list.
		cmds = append(cmds, a.cache.FetchDetailCmd(a.runner, msg.IssueKey))
		for i, t := range a.tabs {
			if t.name == msg.TabName {
				a.tabs[i].list = a.tabs[i].list.SetLoading(true)
				cmds = append(cmds, a.cache.ForceFetchListCmd(a.runner, i, t.name, t.jql))
				break
			}
		}
		return a, tea.Batch(cmds...)

	// ── search run ───────────────────────────────────────────────────────────
	case SearchRunMsg:
		tab := &a.tabs[0]
		tab.jql = msg.Query
		tab.list = tab.list.SetLoading(true)
		return a, a.cache.ForceFetchListCmd(a.runner, 0, "Search:"+msg.Query, msg.Query)

	// ── back to list ─────────────────────────────────────────────────────────
	case BackToListMsg:
		a.view = viewList
		a.overlay = overlayNone
		return a, nil

	// ── field discovery ──────────────────────────────────────────────────────
	case FieldsOverlayMsg:
		a.fields = msg.Fields
		a.fieldCursor = 0
		a.fieldScroll = 0
		// Snapshot current sidebar field IDs as both original and working state.
		sel := make(map[string]bool, len(a.cfg.Detail.SidebarFields))
		for _, sf := range a.cfg.Detail.SidebarFields {
			sel[sf.Field] = true
		}
		a.fieldOriginal = sel
		a.fieldSelected = make(map[string]bool, len(sel))
		for k, v := range sel {
			a.fieldSelected[k] = v
		}
		overlayW := 76
		if overlayW > a.width-4 {
			overlayW = a.width - 4
		}
		a.fieldInput.Width = overlayW - 10
		a.fieldInput.SetValue("")
		a.fieldInput.Focus()
		a.overlay = overlayFields
		return a, nil

	// ── action overlay results ───────────────────────────────────────────────
	case actions.TransitionSelectedMsg:
		a.overlay = overlayNone
		tabName := a.currentTabName()
		return a, backend.ApplyTransitionCmd(a.runner, msg.IssueKey, msg.TransitionName, tabName)

	case actions.TransitionCancelledMsg:
		a.overlay = overlayNone
		return a, nil

	// ── assign overlay messages ──────────────────────────────────────────────
	case backend.AssignSearchDoneMsg:
		a.assign = a.assign.SetResults(msg.Users, msg.Err)
		if msg.Err != nil {
			a.statusLine = a.statusLine.SetMessage("user search failed: "+msg.Err.Error(), true)
		}
		return a, nil

	case actions.AssignSearchRequestMsg:
		return a, backend.AssignSearchCmd(a.cfg.Backend.URL, msg.IssueKey, msg.Query)

	case actions.AssignConfirmedMsg:
		a.overlay = overlayNone
		tabName := a.currentTabName()
		return a, backend.DoAssignCmd(a.runner, msg.IssueKey, msg.Login, msg.DisplayName, tabName)

	case actions.AssignCancelledMsg:
		a.overlay = overlayNone
		return a, nil

	case actions.LabelsSubmittedMsg:
		a.overlay = overlayNone
		tabName := a.currentTabName()
		return a, backend.AddLabelsCmd(a.runner, msg.IssueKey, msg.Labels, tabName)

	case actions.LabelsCancelledMsg:
		a.overlay = overlayNone
		return a, nil

	case actions.CommentSubmittedMsg:
		a.overlay = overlayNone
		tabName := a.currentTabName()
		return a, backend.AddCommentCmd(a.runner, msg.IssueKey, msg.Body, tabName)

	case actions.CommentCancelledMsg:
		a.overlay = overlayNone
		return a, nil

	case actions.AddTabSubmittedMsg:
		return a.applyAddTab(msg)

	case actions.AddTabCancelledMsg:
		a.overlay = overlayNone
		return a, nil

	case actions.EditJQLSubmittedMsg:
		a.overlay = overlayNone
		tab := &a.tabs[a.activeTab]
		tab.jql = msg.JQL
		tab.list = tab.list.SetLoading(true)
		return a, a.cache.ForceFetchListCmd(a.runner, a.activeTab, tab.name, msg.JQL)

	case actions.EditJQLCancelledMsg:
		a.overlay = overlayNone
		return a, nil

	// ── keyboard ─────────────────────────────────────────────────────────────
	case tea.KeyMsg:
		return a.handleKey(msg)
	}

	// Forward to sub-models when overlays are active.
	return a.forwardToOverlay(msg)
}

// isFilterActive reports whether the fuzzy filter input bar is open on the active list tab.
func (a App) isFilterActive() bool {
	if a.view != viewList || a.activeTab >= len(a.tabs) {
		return false
	}
	tab := a.tabs[a.activeTab]
	return !tab.isSearch && tab.list.IsFilterActive()
}

func (a App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quit — not when filter input is open.
	switch msg.String() {
	case "ctrl+c", "q":
		if a.overlay == overlayNone && a.view == viewList && !a.isFilterActive() {
			return a, tea.Quit
		}
		if a.overlay == overlayHelp {
			a.overlay = overlayNone
			return a, nil
		}
	}

	// Escape handling.
	if msg.Type == tea.KeyEsc {
		switch {
		case a.overlay != overlayNone:
			if a.overlay == overlayTransition || a.overlay == overlayLabels ||
				a.overlay == overlayComment || a.overlay == overlayFields ||
				a.overlay == overlayFieldsConfirm {
				// Let the sub-model handle Esc.
				return a.forwardToOverlay(msg)
			}
			a.overlay = overlayNone
			return a, nil
		case a.view == viewDetail:
			a.view = viewList
			a.detailHistory = nil
			a.detailHistIdx = -1
			return a, nil
		case a.view == viewList && a.activeTab < len(a.tabs):
			tab := &a.tabs[a.activeTab]
			if !tab.isSearch && (tab.list.IsFilterActive() || tab.list.IsFilterApplied()) {
				tab.list = tab.list.ClearFilter()
				return a, nil
			}
		}
		return a, nil
	}

	// Delegate to active overlay.
	if a.overlay != overlayNone {
		return a.forwardToOverlay(msg)
	}

	// Help toggle — skip when filter input is open.
	if !a.isFilterActive() && msg.String() == a.cfg.Keybindings.Help {
		if a.overlay == overlayHelp {
			a.overlay = overlayNone
		} else {
			a.overlay = overlayHelp
			a.help = a.help.SetContext(a.view == viewDetail)
		}
		return a, nil
	}

	if a.view == viewList {
		return a.handleListKey(msg)
	}
	return a.handleDetailKey(msg)
}

func (a App) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	tab := &a.tabs[a.activeTab]

	// Search tab handling.
	if tab.isSearch {
		switch msg.String() {
		case "ctrl+c":
			return a, tea.Quit
		case "Q":
			// Refocus the JQL input from the list.
			if !tab.search.IsFocused() {
				tab.search = tab.search.Focus()
				return a, nil
			}
		case "tab", "shift+tab":
			// Allow tab switching even when JQL input is focused; fall through.
		default:
			if tab.search.IsFocused() {
				var cmd tea.Cmd
				tab.search, cmd = tab.search.Update(msg)
				return a, cmd
			}
		}
		// If list has focus, fall through to normal list key handling below.
	}

	// When fuzzy filter input is active, forward all keys to it.
	if tab.list.IsFilterActive() {
		switch msg.String() {
		case "enter":
			tab.list = tab.list.DeactivateFilter()
		case "esc":
			// Handled upstream in handleKey; shouldn't reach here, but guard just in case.
			tab.list = tab.list.ClearFilter()
		default:
			var cmd tea.Cmd
			tab.list, cmd = tab.list.UpdateFilter(msg)
			return a, cmd
		}
		return a, nil
	}

	switch msg.String() {
	case "/":
		tab.list = tab.list.ActivateFilter()
		return a, nil

	case "j", "down":
		if tab.list.AtBottom() && tab.list.HasMore() && !tab.list.loadingMore {
			tab.list = tab.list.SetLoadingMore(true)
			return a, a.cache.FetchListPageCmd(
				a.runner, a.activeTab, tab.name, tab.jql, tab.list.LoadedCount())
		}
		tab.list = tab.list.MoveDown()

	case "k", "up":
		tab.list = tab.list.MoveUp()

	case "enter":
		if iss, ok := tab.list.SelectedIssue(); ok {
			a.view = viewDetail
			a.detail = a.detail.SetLoading(true)
			a = a.pushHistory(iss.Key)
			return a, a.cache.FetchDetailCmd(a.runner, iss.Key)
		}

	case "tab", "right":
		return a.switchTab(1)

	case "shift+tab", "left":
		return a.switchTab(-1)

	case a.cfg.Keybindings.ForceRefresh:
		tab.list = tab.list.SetLoading(true)
		return a, a.cache.ForceFetchListCmd(a.runner, a.activeTab, tab.name, tab.jql)

	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		n := int(msg.String()[0] - '0')
		if n < len(a.tabs) {
			return a.switchToTab(n)
		}

	case "Q":
		tab := a.tabs[a.activeTab]
		a.editJQLM = actions.NewEditJQLModel(tab.name, tab.jql).SetSize(a.width, a.height)
		a.overlay = overlayEditJQL
		return a, nil

	case "+":
		a.addTabM = actions.NewAddTabModel().SetSize(a.width, a.height)
		a.overlay = overlayAddTab
		return a, nil

	case "-":
		// Cannot delete the Search tab.
		if a.activeTab > 0 {
			a.deleteTabIdx = a.activeTab
			a.overlay = overlayConfirmDelete
			return a, nil
		}

	case a.cfg.Keybindings.OpenBrowser:
		if iss, ok := tab.list.SelectedIssue(); ok {
			return a, a.openBrowserCmd(iss.Key)
		}

	case a.cfg.Keybindings.Transition:
		if iss, ok := tab.list.SelectedIssue(); ok {
			a.transition = actions.NewTransitionModel(iss.Key).SetSize(a.width, a.height)
			a.overlay = overlayTransition
			return a, backend.FetchTransitionsCmd(a.runner, a.cfg.Backend.URL, iss.Key)
		}

	case a.cfg.Keybindings.Assign:
		if iss, ok := tab.list.SelectedIssue(); ok {
			a.assign = actions.NewAssignModel(iss.Key).SetSize(a.width, a.height)
			a.overlay = overlayAssign
			return a, nil
		}

	case a.cfg.Keybindings.Sort:
		if !tab.isSearch {
			sf := a.sortableFields()
			// Pre-position cursor on the currently active sort field.
			a.sortCursor = 0
			for i, f := range sf {
				if f.ID == tab.list.SortField() {
					a.sortCursor = i
					break
				}
			}
			a.overlay = overlaySort
		}
	}
	return a, nil
}

func (a App) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if link, ok := a.detail.SelectedLink(); ok {
			if link.URL != "" {
				return a, a.openDirectURLCmd(link.URL)
			}
			a.detail = a.detail.SetLoading(true)
			a = a.pushHistory(link.Key)
			return a, a.cache.FetchDetailCmd(a.runner, link.Key)
		}

	case "backspace", "ctrl+o":
		if a.detailHistIdx > 0 {
			a.detailHistIdx--
			key := a.detailHistory[a.detailHistIdx]
			a.detail = a.detail.SetLoading(true)
			return a, a.cache.FetchDetailCmd(a.runner, key)
		}

	case "ctrl+i", "tab":
		if a.detailHistIdx < len(a.detailHistory)-1 {
			a.detailHistIdx++
			key := a.detailHistory[a.detailHistIdx]
			a.detail = a.detail.SetLoading(true)
			return a, a.cache.FetchDetailCmd(a.runner, key)
		}

	case "left":
		a.detail = a.detail.SwitchBodyTab(-1)

	case "right":
		a.detail = a.detail.SwitchBodyTab(1)

	case a.cfg.Keybindings.Transition:
		key := a.detail.IssueKey()
		a.transition = actions.NewTransitionModel(key).SetSize(a.width, a.height)
		a.overlay = overlayTransition
		return a, backend.FetchTransitionsCmd(a.runner, a.cfg.Backend.URL, key)

	case a.cfg.Keybindings.Assign:
		key := a.detail.IssueKey()
		a.assign = actions.NewAssignModel(key).SetSize(a.width, a.height)
		a.overlay = overlayAssign
		return a, nil

	case a.cfg.Keybindings.AddLabels:
		key := a.detail.IssueKey()
		a.labels = actions.NewLabelsModel(key).SetSize(a.width, a.height)
		a.overlay = overlayLabels
		return a, nil

	case a.cfg.Keybindings.AddComment:
		key := a.detail.IssueKey()
		m := actions.NewCommentModel(key).SetSize(a.width, a.height)
		a.comment = m
		a.overlay = overlayComment
		// If using $EDITOR, launch it immediately.
		if cmd := m.LaunchEditorCmd(); cmd != nil {
			return a, cmd
		}
		return a, nil

	case a.cfg.Keybindings.OpenBrowser:
		return a, a.openBrowserCmd(a.detail.IssueKey())

	case a.cfg.Keybindings.FieldDiscover:
		key := a.detail.IssueKey()
		return a, func() tea.Msg {
			fields, err := backend.FetchAllFields(a.runner, key)
			if err != nil {
				return backend.WriteActionDoneMsg{Err: err, Action: "field discovery"}
			}
			return FieldsOverlayMsg{Fields: fields}
		}

	default:
		// Forward everything else (j/k/arrows/pgup/pgdn/ctrl+d/ctrl+u/g/G/mouse) to scroll.
		var cmd tea.Cmd
		a.detail, cmd = a.detail.Update(msg)
		return a, cmd
	}
	return a, nil
}

func (a App) forwardToOverlay(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch a.overlay {
	case overlayTransition:
		var cmd tea.Cmd
		a.transition, cmd = a.transition.Update(msg)
		return a, cmd

	case overlayAssign:
		var cmd tea.Cmd
		a.assign, cmd = a.assign.Update(msg)
		return a, cmd

	case overlayLabels:
		var cmd tea.Cmd
		a.labels, cmd = a.labels.Update(msg)
		return a, cmd

	case overlayComment:
		var cmd tea.Cmd
		a.comment, cmd = a.comment.Update(msg)
		return a, cmd

	case overlayFields:
		if key, ok := msg.(tea.KeyMsg); ok {
			filtered := a.filteredFields()
			switch key.String() {
			case "down", "j":
				if a.fieldCursor < len(filtered)-1 {
					a.fieldCursor++
				}
				a = a.clampFieldScroll()
			case "up", "k":
				if a.fieldCursor > 0 {
					a.fieldCursor--
				}
				a = a.clampFieldScroll()
			case "enter":
				if len(filtered) > 0 && a.fieldCursor < len(filtered) {
					id := filtered[a.fieldCursor].ID
					a.fieldSelected[id] = !a.fieldSelected[id]
				}
			case "esc":
				if a.fieldInput.Value() != "" {
					a.fieldInput.SetValue("")
					a.fieldCursor = 0
					a.fieldScroll = 0
					return a, nil
				}
				// Check if any changes were made.
				if a.fieldChangesExist() {
					a.overlay = overlayFieldsConfirm
				} else {
					a.fieldInput.Blur()
					a.overlay = overlayNone
				}
			default:
				prev := a.fieldInput.Value()
				var cmd tea.Cmd
				a.fieldInput, cmd = a.fieldInput.Update(msg)
				if a.fieldInput.Value() != prev {
					a.fieldCursor = 0
					a.fieldScroll = 0
				}
				return a, cmd
			}
		}
		return a, nil

	case overlayFieldsConfirm:
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "y", "Y":
				a = a.applyFieldChanges()
				a.fieldInput.Blur()
				a.overlay = overlayNone
			default:
				// Go back to the field list.
				a.overlay = overlayFields
			}
		}
		return a, nil

	case overlaySort:
		if key, ok := msg.(tea.KeyMsg); ok {
			sf := a.sortableFields()
			switch key.String() {
			case "down", "j":
				if a.sortCursor < len(sf)-1 {
					a.sortCursor++
				}
			case "up", "k":
				if a.sortCursor > 0 {
					a.sortCursor--
				}
			case "enter":
				if a.sortCursor < len(sf) && a.activeTab < len(a.tabs) {
					chosen := sf[a.sortCursor].ID
					tab := &a.tabs[a.activeTab]
					asc := true
					if tab.list.SortField() == chosen {
						asc = !tab.list.SortAsc() // toggle direction
					}
					tab.list = tab.list.SetSort(chosen, asc)
				}
				a.overlay = overlayNone
			}
		}
		return a, nil

	case overlayAddTab:
		var cmd tea.Cmd
		a.addTabM, cmd = a.addTabM.Update(msg)
		return a, cmd

	case overlayEditJQL:
		var cmd tea.Cmd
		a.editJQLM, cmd = a.editJQLM.Update(msg)
		return a, cmd

	case overlayConfirmDelete:
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "y", "Y":
				return a.applyDeleteTab(a.deleteTabIdx)
			default:
				a.overlay = overlayNone
			}
		}
		return a, nil
	}
	return a, nil
}

func (a App) View() string {
	if a.width == 0 {
		return ""
	}

	// Compose base view.
	var base string
	switch a.view {
	case viewList:
		base = a.listView()
	case viewDetail:
		base = a.detailView()
	}

	// Overlay on top.
	switch a.overlay {
	case overlayHelp:
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center,
			a.help.View())
	case overlayTransition:
		return a.transition.View()
	case overlayAssign:
		return a.assign.View()
	case overlayLabels:
		return a.labels.View()
	case overlayComment:
		return a.comment.View()
	case overlayFields:
		return a.fieldsOverlayView()
	case overlayFieldsConfirm:
		return a.fieldsConfirmView()
	case overlaySort:
		return a.sortOverlayView()
	case overlayAddTab:
		return a.addTabM.View()
	case overlayEditJQL:
		return a.editJQLM.View()
	case overlayConfirmDelete:
		return a.confirmDeleteView()
	}

	return base
}

func (a App) listView() string {
	tab := a.tabs[a.activeTab]
	tabBarView := a.tabBar.SetActive(a.activeTab).SetWidth(a.width).View()
	statusView := a.statusLine.SetHint(a.listHint()).SetWidth(a.width).View()

	contentH := a.height - lipgloss.Height(tabBarView) - lipgloss.Height(statusView) - 1
	if contentH < 1 {
		contentH = 1
	}

	var content string
	if tab.isSearch {
		searchView := tab.search.SetWidth(a.width).View()
		if tab.search.Query() == "" {
			// No query submitted yet — show only the input bar.
			content = searchView
		} else {
			listH := contentH - lipgloss.Height(searchView)
			if listH < 1 {
				listH = 1
			}
			tab.list = tab.list.SetSize(a.width, listH)
			content = searchView + "\n" + tab.list.View()
		}
	} else {
		tab.list = tab.list.SetSize(a.width, contentH)
		content = tab.list.View()
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		tabBarView,
		content,
		statusView,
	)
}

func (a App) listHint() string {
	if a.activeTab >= len(a.tabs) {
		return ""
	}
	tab := a.tabs[a.activeTab]
	if tab.isSearch {
		if tab.search.IsFocused() {
			return "Enter run JQL · Tab/←→ switch tabs · ? help · q quit"
		}
		return "j/k navigate · Enter open · Q edit JQL · Tab/←→ tabs · ? help · q quit"
	}
	if tab.list.IsFilterActive() {
		return "type to filter · Enter confirm · Esc clear"
	}
	if tab.list.IsFilterApplied() {
		return "j/k navigate · Enter open · / re-edit · Esc clear filter · r refresh · ? help · q quit"
	}
	hint := fmt.Sprintf("j/k navigate · Enter open · %s browser · %s move · %s assign · / filter · %s sort · Tab/←→ tabs · Q JQL · r refresh · ? help · q quit",
		a.cfg.Keybindings.OpenBrowser, a.cfg.Keybindings.Transition, a.cfg.Keybindings.Assign, a.cfg.Keybindings.Sort)
	if tab.list.SortField() != "" {
		dir := "▲"
		if !tab.list.SortAsc() {
			dir = "▼"
		}
		hint = fmt.Sprintf("sorted by %s %s · %s", tab.list.SortField(), dir, hint)
	}
	return hint
}

func (a App) detailView() string {
	tabBarView := a.tabBar.SetActive(a.activeTab).SetWidth(a.width).View()
	statusView := a.statusLine.SetHint(a.detailHint()).SetWidth(a.width).View()
	contentH := a.height - lipgloss.Height(tabBarView) - lipgloss.Height(statusView)
	if contentH < 1 {
		contentH = 1
	}
	d := a.detail.SetSize(a.width, contentH, a.cfg.Detail.SidebarWidth)
	return lipgloss.JoinVertical(lipgloss.Left,
		tabBarView,
		d.View(),
		statusView,
	)
}

func (a App) detailHint() string {
	kb := a.cfg.Keybindings
	hint := fmt.Sprintf("j/k scroll · ctrl+d/u page · ←/→ tabs · Enter open link · %s status · %s labels · %s comment · %s browser · Esc list · ? help",
		kb.Transition, kb.AddLabels, kb.AddComment, kb.OpenBrowser)
	var nav []string
	if a.detailHistIdx > 0 {
		nav = append(nav, "⌫/ctrl+o back")
	}
	if a.detailHistIdx < len(a.detailHistory)-1 {
		nav = append(nav, "ctrl+i fwd")
	}
	if len(nav) > 0 {
		hint = strings.Join(nav, " · ") + " · " + hint
	}
	return hint
}

func (a App) filteredFields() []model.Field {
	q := strings.ToLower(a.fieldInput.Value())
	if q == "" {
		return a.fields
	}
	out := make([]model.Field, 0, len(a.fields))
	for _, f := range a.fields {
		val := strings.ToLower(fieldValue(a.detail.issue, f.ID))
		if strings.Contains(strings.ToLower(f.DisplayName), q) ||
			strings.Contains(strings.ToLower(f.ID), q) ||
			strings.Contains(val, q) {
			out = append(out, f)
		}
	}
	return out
}

func (a App) fieldPageHeight() int {
	// Fixed overhead: padding(2) + title(1) + filter+blank(2) + counter(1) + blank+hint(2) = 8
	// Border adds 2 more, terminal margin 2 → total budget a.height - 12 for rows.
	h := a.height - 12
	if h < 1 {
		h = 1
	}
	return h
}

func (a App) clampFieldScroll() App {
	pageH := a.fieldPageHeight()
	if a.fieldCursor < a.fieldScroll {
		a.fieldScroll = a.fieldCursor
	}
	if a.fieldCursor >= a.fieldScroll+pageH {
		a.fieldScroll = a.fieldCursor - pageH + 1
	}
	return a
}

func (a App) fieldChangesExist() bool {
	for id, sel := range a.fieldSelected {
		if sel != a.fieldOriginal[id] {
			return true
		}
	}
	for id, orig := range a.fieldOriginal {
		if orig != a.fieldSelected[id] {
			return true
		}
	}
	return false
}

// applyFieldChanges rebuilds cfg.Detail.SidebarFields from fieldSelected,
// persists the config, and rebuilds the detail sidebar.
func (a App) applyFieldChanges() App {
	// Preserve existing entries (including custom labels) for kept fields,
	// then append newly added fields at the end.
	kept := make([]config.SidebarField, 0, len(a.cfg.Detail.SidebarFields))
	for _, sf := range a.cfg.Detail.SidebarFields {
		if a.fieldSelected[sf.Field] {
			kept = append(kept, sf)
		}
	}
	for _, f := range a.fields {
		if a.fieldSelected[f.ID] && !a.fieldOriginal[f.ID] {
			kept = append(kept, config.SidebarField{Field: f.ID})
		}
	}
	a.cfg.Detail.SidebarFields = kept

	if err := config.Save(a.configPath, a.cfg); err != nil {
		a.statusLine = a.statusLine.SetMessage("sidebar saved (config write failed: "+err.Error()+")", true)
	} else {
		a.statusLine = a.statusLine.SetMessage("sidebar fields updated", false)
	}

	// Swap only the sidebar so the loaded issue and scroll position are preserved.
	a.detail.sidebar = NewSidebar(a.cfg.Detail.SidebarFields)
	return a
}

func (a App) fieldsOverlayView() string {
	filtered := a.filteredFields()
	pageH := a.fieldPageHeight()

	overlayW := 76
	if overlayW > a.width-4 {
		overlayW = a.width - 4
	}
	// inner = overlayW minus border(2) and padding(2*2)
	innerW := overlayW - 6
	// fixed columns: checkbox(4) + name(18) + gap(2) + id(14) + gap(2) = 40
	const fixedCols = 40
	valueW := innerW - fixedCols
	if valueW < 4 {
		valueW = 4
	}

	checkedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00cc44")).Bold(true)
	uncheckedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#dddddd")).Width(18)
	idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Width(14)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#aaaaff")).Italic(true)
	cursorBg := lipgloss.NewStyle().Background(lipgloss.Color("#222255"))

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true).
		Render("Sidebar Fields") + "\n")
	filterPrompt := lipgloss.NewStyle().Foreground(lipgloss.Color("#5555ff")).Bold(true).Render("/")
	sb.WriteString(filterPrompt + " " + a.fieldInput.View() + "\n\n")

	if len(filtered) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).
			Italic(true).Render("  no matches\n"))
	} else {
		start := a.fieldScroll
		if start > len(filtered) {
			start = len(filtered)
		}
		end := start + pageH
		if end > len(filtered) {
			end = len(filtered)
		}

		const nameW, idW = 18, 14
		rowW := innerW
		for i, f := range filtered[start:end] {
			abs := start + i
			var checkbox string
			if a.fieldSelected[f.ID] {
				checkbox = checkedStyle.Render("[x]")
			} else {
				checkbox = uncheckedStyle.Render("[ ]")
			}
			// Truncate each column to its fixed width so the row never wraps.
			name := f.DisplayName
			if len(name) > nameW {
				name = name[:nameW-1] + "…"
			}
			id := f.ID
			if len(id) > idW {
				id = id[:idW-1] + "…"
			}
			val := fieldValue(a.detail.issue, f.ID)
			if len(val) > valueW {
				val = val[:valueW-1] + "…"
			}
			row := checkbox + " " +
				nameStyle.Render(name) + "  " +
				idStyle.Render(id) + "  " +
				valueStyle.Render(val)
			if abs == a.fieldCursor {
				row = cursorBg.Width(rowW).Render(row)
			}
			sb.WriteString(row + "\n")
		}
		if len(filtered) > pageH {
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).
				Render(fmt.Sprintf("  %d/%d\n", a.fieldCursor+1, len(filtered))))
		}
	}

	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).
		Render("↑/↓ j/k navigate · Enter toggle · type filter · Esc done")
	sb.WriteString("\n" + hint)

	// MaxHeight fires AFTER the border is applied, so it constrains the total
	// overlay height (content + padding + border). Cap at a.height-2 to leave
	// one terminal row of margin on each side for lipgloss.Place centering.
	maxH := a.height - 2
	if maxH < 10 {
		maxH = 10
	}
	overlay := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5555ff")).
		Padding(1, 2).
		Background(lipgloss.Color("#111111")).
		Width(overlayW).
		MaxHeight(maxH).Render(sb.String())

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, overlay)
}

func (a App) fieldsConfirmView() string {
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00cc44"))
	removeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4444"))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#dddddd"))

	// Build a display-name lookup.
	nameOf := make(map[string]string, len(a.fields))
	for _, f := range a.fields {
		nameOf[f.ID] = f.DisplayName
	}

	var added, removed []string
	for id, sel := range a.fieldSelected {
		if sel && !a.fieldOriginal[id] {
			n := nameOf[id]
			if n == "" {
				n = id
			}
			added = append(added, n+" ("+id+")")
		}
	}
	for id, orig := range a.fieldOriginal {
		if orig && !a.fieldSelected[id] {
			n := nameOf[id]
			if n == "" {
				n = id
			}
			removed = append(removed, n+" ("+id+")")
		}
	}

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true).
		Render("Apply sidebar changes?") + "\n\n")

	if len(added) > 0 {
		sb.WriteString(nameStyle.Render("  Added:") + "\n")
		for _, n := range added {
			sb.WriteString(addStyle.Render("    + "+n) + "\n")
		}
	}
	if len(removed) > 0 {
		sb.WriteString(nameStyle.Render("  Removed:") + "\n")
		for _, n := range removed {
			sb.WriteString(removeStyle.Render("    - "+n) + "\n")
		}
	}

	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).
		Render("\n  Press y to confirm, any other key to go back")
	sb.WriteString(hint)

	overlayW := 60
	if overlayW > a.width-4 {
		overlayW = a.width - 4
	}
	overlay := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ffaa00")).
		Padding(1, 2).
		Background(lipgloss.Color("#111111")).
		Width(overlayW).Render(sb.String())

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, overlay)
}

func (a App) switchTab(dir int) (tea.Model, tea.Cmd) {
	next := a.activeTab + dir
	if next < 0 {
		next = len(a.tabs) - 1
	}
	if next >= len(a.tabs) {
		next = 0
	}
	return a.switchToTab(next)
}

func (a App) switchToTab(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(a.tabs) {
		return a, nil
	}
	a.activeTab = idx
	tab := a.tabs[idx]
	if tab.isSearch {
		// Only pull focus to the JQL bar when no query has been run yet.
		if tab.search.Query() == "" {
			a.tabs[idx].search = tab.search.Focus()
		}
		return a, nil
	}
	a.tabs[idx].list = tab.list.SetLoading(true)
	return a, a.cache.ForceFetchListCmd(a.runner, idx, tab.name, tab.jql)
}

// pushHistory records a new navigation target, truncating any forward history.
func (a App) pushHistory(key string) App {
	// Keep everything up to (and including) current position, then append new key.
	a.detailHistory = append(a.detailHistory[:a.detailHistIdx+1], key)
	a.detailHistIdx = len(a.detailHistory) - 1
	return a
}

func (a App) currentTabName() string {
	if a.activeTab < len(a.tabs) {
		return a.tabs[a.activeTab].name
	}
	return ""
}

// applyAddTab adds the new tab to in-memory state, saves config, and fetches.
func (a App) applyAddTab(msg actions.AddTabSubmittedMsg) (tea.Model, tea.Cmd) {
	a.overlay = overlayNone

	newTab := tabState{
		name: msg.Name,
		jql:  msg.JQL,
		list: NewIssueList(a.cfg.List.Columns).SetLoading(true),
	}
	a.tabs = append(a.tabs, newTab)

	// Update tab bar names.
	names := make([]string, len(a.tabs))
	for i, t := range a.tabs {
		names[i] = t.name
	}
	a.tabBar = NewTabBar(names)

	// Persist to config.
	a.cfg.Tabs = append(a.cfg.Tabs, config.Tab{Name: msg.Name, JQL: msg.JQL})
	if err := config.Save(a.configPath, a.cfg); err != nil {
		a.statusLine = a.statusLine.SetMessage("tab added (config save failed: "+err.Error()+")", true)
	} else {
		a.statusLine = a.statusLine.SetMessage("tab "+msg.Name+" added and saved", false)
	}

	newIdx := len(a.tabs) - 1
	a.activeTab = newIdx
	return a, a.cache.ForceFetchListCmd(a.runner, newIdx, msg.Name, msg.JQL)
}

// applyDeleteTab removes a tab, saves config, and switches to an adjacent tab.
func (a App) applyDeleteTab(idx int) (tea.Model, tea.Cmd) {
	a.overlay = overlayNone
	if idx <= 0 || idx >= len(a.tabs) {
		return a, nil
	}

	tabName := a.tabs[idx].name

	// Remove from runtime state.
	a.tabs = append(a.tabs[:idx], a.tabs[idx+1:]...)

	// Remove from config (config index = runtime index - 1, since tab 0 is Search).
	cfgIdx := idx - 1
	if cfgIdx >= 0 && cfgIdx < len(a.cfg.Tabs) {
		a.cfg.Tabs = append(a.cfg.Tabs[:cfgIdx], a.cfg.Tabs[cfgIdx+1:]...)
	}

	// Update tab bar.
	names := make([]string, len(a.tabs))
	for i, t := range a.tabs {
		names[i] = t.name
	}
	a.tabBar = NewTabBar(names)

	// Persist.
	if err := config.Save(a.configPath, a.cfg); err != nil {
		a.statusLine = a.statusLine.SetMessage("tab removed (config save failed: "+err.Error()+")", true)
	} else {
		a.statusLine = a.statusLine.SetMessage("tab "+tabName+" removed", false)
	}

	// Switch to adjacent tab.
	a.activeTab = idx
	if a.activeTab >= len(a.tabs) {
		a.activeTab = len(a.tabs) - 1
	}
	if a.activeTab < 0 {
		a.activeTab = 0
	}

	// Fetch the newly active tab if it's not Search.
	if !a.tabs[a.activeTab].isSearch {
		t := a.tabs[a.activeTab]
		a.tabs[a.activeTab].list = t.list.SetLoading(true)
		return a, a.cache.ForceFetchListCmd(a.runner, a.activeTab, t.name, t.jql)
	}
	return a, nil
}

type sortableField struct {
	ID    string
	Label string
}

// sortableFields returns the list of fields the user can sort by:
// all configured list columns plus created and updated.
func (a App) sortableFields() []sortableField {
	seen := make(map[string]bool)
	fields := make([]sortableField, 0, len(a.cfg.List.Columns)+2)
	for _, c := range a.cfg.List.Columns {
		if seen[c.Field] {
			continue
		}
		seen[c.Field] = true
		label := c.Label
		if label == "" {
			if l, ok := columnDefaultLabels[c.Field]; ok {
				label = l
			} else {
				label = c.Field
			}
		}
		fields = append(fields, sortableField{ID: c.Field, Label: label})
	}
	for _, extra := range []sortableField{
		{ID: "created", Label: "Created"},
		{ID: "updated", Label: "Updated"},
	} {
		if !seen[extra.ID] {
			fields = append(fields, extra)
		}
	}
	return fields
}

func (a App) sortOverlayView() string {
	sf := a.sortableFields()
	var currentField string
	var currentAsc bool
	if a.activeTab < len(a.tabs) {
		tab := a.tabs[a.activeTab]
		currentField = tab.list.SortField()
		currentAsc = tab.list.SortAsc()
	}

	cursorBg := lipgloss.NewStyle().Background(lipgloss.Color("#222255")).Foreground(lipgloss.Color("#ffffff")).Bold(true)
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00cc44")).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#dddddd"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true).Render("Sort by") + "\n\n")

	const rowW = 28
	for i, f := range sf {
		var indicator string
		if f.ID == currentField {
			if currentAsc {
				indicator = activeStyle.Render("▲ ")
			} else {
				indicator = activeStyle.Render("▼ ")
			}
		} else {
			indicator = dimStyle.Render("  ")
		}
		label := normalStyle.Render(fmt.Sprintf("%-*s", rowW-2, f.Label))
		row := indicator + label
		if i == a.sortCursor {
			row = cursorBg.Width(rowW).Render(indicator + fmt.Sprintf("%-*s", rowW-2, f.Label))
		}
		sb.WriteString(row + "\n")
	}

	hint := dimStyle.Render("\nj/k select · Enter apply · same field reverses · Esc close")
	sb.WriteString(hint)

	overlay := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5555ff")).
		Padding(1, 2).
		Background(lipgloss.Color("#111111")).
		Width(rowW + 8).Render(sb.String())

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, overlay)
}

// confirmDeleteView renders the "are you sure?" overlay.
func (a App) confirmDeleteView() string {
	if a.deleteTabIdx <= 0 || a.deleteTabIdx >= len(a.tabs) {
		return ""
	}
	tabName := a.tabs[a.deleteTabIdx].name

	overlayW := 52
	if overlayW > a.width-4 {
		overlayW = a.width - 4
	}

	title := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4444")).Bold(true).
		Render("Delete Tab")
	msg := lipgloss.NewStyle().Foreground(lipgloss.Color("#dddddd")).
		Render(fmt.Sprintf("Delete tab %q?", tabName))
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).
		Render("Press y to confirm, any other key to cancel")

	content := title + "\n\n" + msg + "\n\n" + hint
	overlay := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#ff4444")).
		Padding(1, 2).
		Background(lipgloss.Color("#111111")).
		Width(overlayW).Render(content)

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, overlay)
}

func (a App) openDirectURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		var err error
		switch runtime.GOOS {
		case "darwin":
			err = exec.Command("open", url).Start()
		default:
			err = exec.Command("xdg-open", url).Start()
		}
		return browserOpenDoneMsg{err: err}
	}
}

func (a App) openBrowserCmd(key string) tea.Cmd {
	baseURL := strings.TrimRight(a.cfg.Backend.URL, "/")
	cli := a.cfg.Backend.CLI
	return func() tea.Msg {
		var err error
		if baseURL != "" {
			// Use configured base URL + xdg-open / open.
			url := baseURL + "/browse/" + key
			switch runtime.GOOS {
			case "darwin":
				err = exec.Command("open", url).Start()
			default:
				err = exec.Command("xdg-open", url).Start()
			}
		} else {
			// Fall back to the CLI's own open command.
			err = exec.Command(cli, "open", key).Start()
		}
		return browserOpenDoneMsg{err: err}
	}
}

func (a App) resizeAll() App {
	a.tabBar = a.tabBar.SetWidth(a.width)
	a.help = a.help.SetSize(a.width, a.height)
	a.statusLine = a.statusLine.SetWidth(a.width)
	// Only resize overlay sub-models that are currently active; the others
	// hold a zero-value list/input model until first opened and would panic.
	switch a.overlay {
	case overlayTransition:
		a.transition = a.transition.SetSize(a.width, a.height)
	case overlayAssign:
		a.assign = a.assign.SetSize(a.width, a.height)
	case overlayLabels:
		a.labels = a.labels.SetSize(a.width, a.height)
	case overlayComment:
		a.comment = a.comment.SetSize(a.width, a.height)
	case overlayAddTab:
		a.addTabM = a.addTabM.SetSize(a.width, a.height)
	case overlayEditJQL:
		a.editJQLM = a.editJQLM.SetSize(a.width, a.height)
	}
	for i := range a.tabs {
		if a.tabs[i].isSearch {
			a.tabs[i].search = a.tabs[i].search.SetWidth(a.width)
		}
	}
	return a
}
