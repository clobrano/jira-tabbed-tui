package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	tabInactiveStyle = lipgloss.NewStyle().
				Padding(0, 2).
				Foreground(lipgloss.Color("#aaaaaa"))

	tabActiveStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#5555ff")).
			Bold(true)

	tabBarContainerStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(lipgloss.Color("#444444"))
)

// TabBar renders the horizontal tab strip.
type TabBar struct {
	names     []string // index 0 is always "Search"
	activeIdx int
	width     int
}

func NewTabBar(names []string) TabBar {
	return TabBar{names: names}
}

func (t TabBar) SetActive(idx int) TabBar {
	t.activeIdx = idx
	return t
}

func (t TabBar) SetWidth(w int) TabBar {
	t.width = w
	return t
}

func (t TabBar) View() string {
	row := ""
	for i, name := range t.names {
		label := fmt.Sprintf("%d:%s", i, name)
		if i == t.activeIdx {
			row += tabActiveStyle.Render(label)
		} else {
			row += tabInactiveStyle.Render(label)
		}
	}
	return tabBarContainerStyle.Width(t.width).Render(row)
}
