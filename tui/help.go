package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/clobrano/jira-tabbed-tui/config"
)

var (
	helpTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#5555ff")).
			Bold(true).
			Padding(0, 2)

	helpSectionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#aaaaaa")).
				Bold(true).
				MarginTop(1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5555ff")).
			Width(14).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#dddddd"))

	helpOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#5555ff")).
				Padding(1, 2).
				Background(lipgloss.Color("#111111"))
)

// HelpOverlay renders the keybindings reference panel.
type HelpOverlay struct {
	kb       config.Keybindings
	inDetail bool
	width    int
	height   int
}

func NewHelpOverlay(kb config.Keybindings) HelpOverlay {
	return HelpOverlay{kb: kb}
}

func (h HelpOverlay) SetContext(inDetail bool) HelpOverlay {
	h.inDetail = inDetail
	return h
}

func (h HelpOverlay) SetSize(w, ww int) HelpOverlay {
	h.width = w
	h.height = ww
	return h
}

type binding struct{ key, desc string }

func (h HelpOverlay) View() string {
	var sb strings.Builder

	sb.WriteString(helpTitleStyle.Render(" Keybindings "))
	sb.WriteString("\n")

	nav := []binding{
		{"j / ↓", "Move down"},
		{"k / ↑", "Move up"},
		{"Tab / ← →", "Next / prev tab"},
		{"0–9", "Jump to tab by index"},
		{"+", "Add new tab"},
		{"-", "Delete current tab"},
		{"Q", "Edit tab JQL (temporary)"},
		{"Enter", "Open issue detail"},
		{h.kb.ForceRefresh, "Refresh current tab"},
		{h.kb.Help + " / Esc", "Close this overlay"},
		{"q / Ctrl+C", "Quit"},
	}
	sb.WriteString(h.section("Navigation", nav))

	if h.inDetail {
		detail := []binding{
			{"Esc", "Back to list"},
			{"← / →", "Switch Description / Comments"},
			{h.kb.Transition, "Change status"},
			{h.kb.AddLabels, "Add labels"},
			{h.kb.AddComment, "Add comment"},
			{h.kb.OpenBrowser, "Open in browser"},
			{h.kb.FieldDiscover, "List all fields"},
		}
		sb.WriteString(h.section("Detail View", detail))
	}

	content := sb.String()
	overlayW := 52
	if h.width > 0 && overlayW > h.width-4 {
		overlayW = h.width - 4
	}
	return lipgloss.Place(
		h.width, h.height,
		lipgloss.Center, lipgloss.Center,
		helpOverlayStyle.Width(overlayW).Render(content),
	)
}

func (h HelpOverlay) section(title string, bindings []binding) string {
	var sb strings.Builder
	sb.WriteString(helpSectionStyle.Render(title))
	sb.WriteString("\n")
	for _, b := range bindings {
		sb.WriteString(fmt.Sprintf("%s%s\n",
			helpKeyStyle.Render(b.key),
			helpDescStyle.Render(b.desc)))
	}
	return sb.String()
}
