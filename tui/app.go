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
	overlayNone       overlayMode = iota
	overlayTransition overlayMode = iota
	overlayLabels     overlayMode = iota
	overlayComment    overlayMode = iota
	overlayHelp       overlayMode = iota
	overlayFields     overlayMode = iota
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

// App is the root Bubbletea model.
type App struct {
	cfg         config.Config
	runner      backend.Runner
	cache       *backend.Cache
	tabs        []tabState
	activeTab   int
	view        viewMode
	overlay     overlayMode
	detail      Detail
	transition  actions.TransitionModel
	labels      actions.LabelsModel
	comment     actions.CommentModel
	help        HelpOverlay
	statusLine  StatusLine
	tabBar      TabBar
	fields      []model.Field
	fieldScroll int
	width       int
	height      int
	globalSpinner spinner.Model
}

// New creates the root App model.
func New(cfg config.Config, runner backend.Runner) App {
	cache := backend.NewCache()

	// Build tab states: index 0 = Search, then config tabs.
	tabs := make([]tabState, 0, 1+len(cfg.Tabs))
	tabs = append(tabs, tabState{
		name:     "Search",
		isSearch: true,
		list:     NewIssueList(),
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

	return App{
		cfg:           cfg,
		runner:        runner,
		cache:         cache,
		tabs:          tabs,
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
	// Fetch initial tab — if first tab is Search, try the second.
	if len(a.tabs) > 1 {
		a.tabs[1].list = a.tabs[1].list.SetLoading(true)
		cmds = append(cmds, a.cache.FetchListCmd(a.runner, 1, a.tabs[1].name, a.tabs[1].jql))
		// Set active tab to first non-search.
		a.activeTab = 1
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

	// ── keyboard ─────────────────────────────────────────────────────────────
	case tea.KeyMsg:
		return a.handleKey(msg)
	}

	// Forward to sub-models when overlays are active.
	return a.forwardToOverlay(msg)
}

func (a App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quit.
	switch msg.String() {
	case "ctrl+c", "q":
		if a.overlay == overlayNone && a.view == viewList {
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
		}
		return a, nil
	}

	// Delegate to active overlay.
	if a.overlay != overlayNone {
		return a.forwardToOverlay(msg)
	}

	// Help toggle.
	if msg.String() == a.cfg.Keybindings.Help {
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

	// Search tab input forwarding.
	if tab.isSearch {
		switch msg.String() {
		case "ctrl+c":
			return a, tea.Quit
		}
		var cmd tea.Cmd
		tab.search, cmd = tab.search.Update(msg)
		return a, cmd
	}

	switch msg.String() {
	case "j", "down":
		if tab.list.AtBottom() && tab.list.HasMore() && !tab.list.loadingMore {
			tab.list = tab.list.SetLoadingMore(true)
			return a, a.cache.FetchListPageCmd(
				a.runner, a.activeTab, tab.name, tab.jql, len(tab.list.issues))
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

	case "tab":
		return a, a.switchTab(1)

	case "shift+tab":
		return a, a.switchTab(-1)

	case a.cfg.Keybindings.ForceRefresh:
		tab.list = tab.list.SetLoading(true)
		return a, a.cache.ForceFetchListCmd(a.runner, a.activeTab, tab.name, tab.jql)

	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		n := int(msg.String()[0] - '0')
		if n < len(a.tabs) {
			return a, a.switchToTab(n)
		}
	}
	return a, nil
}

func (a App) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		var cmd tea.Cmd
		a.detail, cmd = a.detail.Update(msg)
		return a, cmd

	case "k", "up":
		var cmd tea.Cmd
		a.detail, cmd = a.detail.Update(msg)
		return a, cmd

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
	}

	return base
}

func (a App) listView() string {
	tab := a.tabs[a.activeTab]
	tabBarView := a.tabBar.SetActive(a.activeTab).SetWidth(a.width).View()
	statusView := a.statusLine.SetWidth(a.width).View()

	contentH := a.height - lipgloss.Height(tabBarView) - lipgloss.Height(statusView) - 1
	if contentH < 1 {
		contentH = 1
	}

	var content string
	if tab.isSearch {
		searchView := tab.search.SetWidth(a.width).View()
		listH := contentH - lipgloss.Height(searchView)
		if listH < 1 {
			listH = 1
		}
		tab.list = tab.list.SetSize(a.width, listH)
		content = searchView + "\n" + tab.list.View()
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

func (a App) detailView() string {
	tabBarView := a.tabBar.SetActive(a.activeTab).SetWidth(a.width).View()
	statusView := a.statusLine.SetWidth(a.width).View()
	contentH := a.height - lipgloss.Height(tabBarView) - lipgloss.Height(statusView) - 1
	if contentH < 1 {
		contentH = 1
	}
	d := a.detail.SetSize(a.width, contentH, a.cfg.Detail.SidebarWidth)
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).
		Render("  ← → body tabs  Esc back  ? help")

	return lipgloss.JoinVertical(lipgloss.Left,
		tabBarView,
		d.View(),
		hint,
		statusView,
	)
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

func (a App) switchTab(dir int) tea.Cmd {
	next := a.activeTab + dir
	if next < 0 {
		next = len(a.tabs) - 1
	}
	if next >= len(a.tabs) {
		next = 0
	}
	return a.switchToTab(next)
}

func (a App) switchToTab(idx int) tea.Cmd {
	if idx < 0 || idx >= len(a.tabs) {
		return nil
	}
	a.activeTab = idx
	tab := a.tabs[idx]
	if tab.isSearch {
		a.tabs[idx].search = tab.search.Focus()
		return nil
	}
	a.tabs[idx].list = tab.list.SetLoading(true)
	return a.cache.ForceFetchListCmd(a.runner, idx, tab.name, tab.jql)
}

func (a App) currentTabName() string {
	if a.activeTab < len(a.tabs) {
		return a.tabs[a.activeTab].name
	}
	return ""
}

func (a App) openBrowserCmd() tea.Cmd {
	key := a.detail.IssueKey()
	return func() tea.Msg {
		var args []string
		switch runtime.GOOS {
		case "darwin":
			args = []string{"open"}
		default:
			args = []string{"xdg-open"}
		}
		url := "https://jira.example.com/browse/" + key
		if err := exec.Command(args[0], url).Start(); err != nil {
			return backend.WriteActionDoneMsg{Action: "open browser", Err: err}
		}
		return backend.WriteActionDoneMsg{Action: "open browser"}
	}
}

func (a App) resizeAll() App {
	a.tabBar = a.tabBar.SetWidth(a.width)
	a.help = a.help.SetSize(a.width, a.height)
	a.transition = a.transition.SetSize(a.width, a.height)
	a.labels = a.labels.SetSize(a.width, a.height)
	a.comment = a.comment.SetSize(a.width, a.height)
	a.statusLine = a.statusLine.SetWidth(a.width)
	// Resize search inputs.
	for i := range a.tabs {
		if a.tabs[i].isSearch {
			a.tabs[i].search = a.tabs[i].search.SetWidth(a.width)
		}
	}
	return a
}
