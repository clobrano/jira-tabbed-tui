package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
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
	overlayAddTab        overlayMode = iota
	overlayConfirmDelete overlayMode = iota
	overlayEditJQL       overlayMode = iota
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
	addTabM      actions.AddTabModel
	editJQLM     actions.EditJQLModel
	deleteTabIdx int // tab index pending confirmation
	help         HelpOverlay
	statusLine   StatusLine
	tabBar       TabBar
	fields       []model.Field
	fieldScroll  int
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
		list:     NewIssueList().SetLoading(false),
		search:   NewSearchInput(),
	})
	for _, t := range cfg.Tabs {
		tabs = append(tabs, tabState{
			name: t.Name,
			jql:  t.JQL,
			list: NewIssueList(),
		})
	}

	tabNames := make([]string, len(tabs))
	for i, t := range tabs {
		tabNames[i] = t.name
	}

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
		globalSpinner: gs,
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
		} else {
			a.detail = a.detail.SetIssue(msg.Issue)
			a.statusLine = a.statusLine.SetMessage("", false)
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
		a.fieldScroll = 0
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
		if a.overlay == overlayHelp || a.overlay == overlayFields {
			a.overlay = overlayNone
			return a, nil
		}
	}

	// Escape handling.
	if msg.Type == tea.KeyEsc {
		switch {
		case a.overlay != overlayNone:
			if a.overlay == overlayTransition || a.overlay == overlayLabels ||
				a.overlay == overlayComment {
				// Let the sub-model handle Esc.
				return a.forwardToOverlay(msg)
			}
			a.overlay = overlayNone
			return a, nil
		case a.view == viewDetail:
			a.view = viewList
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
	}
	return a, nil
}

func (a App) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left":
		a.detail = a.detail.SwitchBodyTab(-1)

	case "right":
		a.detail = a.detail.SwitchBodyTab(1)

	case a.cfg.Keybindings.Transition:
		key := a.detail.IssueKey()
		a.transition = actions.NewTransitionModel(key).SetSize(a.width, a.height)
		a.overlay = overlayTransition
		return a, backend.FetchTransitionsCmd(a.runner, key)

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
		return a, a.openBrowserCmd()

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
			switch key.String() {
			case "j", "down":
				if a.fieldScroll < len(a.fields)-1 {
					a.fieldScroll++
				}
			case "k", "up":
				if a.fieldScroll > 0 {
					a.fieldScroll--
				}
			case "esc", "q":
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
	case overlayLabels:
		return a.labels.View()
	case overlayComment:
		return a.comment.View()
	case overlayFields:
		return a.fieldsOverlayView()
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
	return "j/k navigate · Enter open · / filter · Tab/←→ tabs · Q JQL · r refresh · ? help · q quit"
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
	return fmt.Sprintf("j/k scroll · ctrl+d/u page · ←/→ body · %s status · %s labels · %s comment · %s browser · Esc back · ? help",
		kb.Transition, kb.AddLabels, kb.AddComment, kb.OpenBrowser)
}

func (a App) fieldsOverlayView() string {
	var sb strings.Builder
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true).
		Render("All Fields (j/k scroll, Esc close)")
	sb.WriteString(title + "\n\n")

	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#dddddd")).Width(24)
	idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	overlayH := a.height - 8
	if overlayH < 5 {
		overlayH = 5
	}
	end := a.fieldScroll + overlayH
	if end > len(a.fields) {
		end = len(a.fields)
	}
	for _, f := range a.fields[a.fieldScroll:end] {
		sb.WriteString(nameStyle.Render(f.DisplayName) + "  " + idStyle.Render(f.ID) + "\n")
	}
	if len(a.fields) > overlayH {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).
			Render(fmt.Sprintf("\n  %d/%d", a.fieldScroll+1, len(a.fields))))
	}

	overlayW := 60
	if overlayW > a.width-4 {
		overlayW = a.width - 4
	}
	overlay := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5555ff")).
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
		list: NewIssueList().SetLoading(true),
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

func (a App) openBrowserCmd() tea.Cmd {
	key := a.detail.IssueKey()
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
