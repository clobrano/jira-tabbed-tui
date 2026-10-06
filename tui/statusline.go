package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	statusSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00cc44")).Bold(true)
	statusErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4444")).Bold(true)
	statusIdleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))

	hintKeyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8888ff")).Bold(true)
	hintLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))
	hintSepStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
)

// keyHint is one footer hint: a key and what it does. drop is the order in
// which hints give way on narrow screens (higher first); 0 never drops.
// A hint with no key is plain information (e.g. the current sort).
type keyHint struct {
	key, label string
	drop       int
}

// StatusLine renders the last CLI operation result above a line of key hints.
type StatusLine struct {
	message string
	isError bool
	hints   []keyHint
	width   int
}

func (s StatusLine) SetMessage(msg string, isError bool) StatusLine {
	s.message = msg
	s.isError = isError
	return s
}

// ClearError removes the message if it is an error, keeping confirmations
// such as "transition succeeded" visible across the refetch that follows them.
func (s StatusLine) ClearError() StatusLine {
	if s.isError {
		return s.SetMessage("", false)
	}
	return s
}

func (s StatusLine) SetHints(hints []keyHint) StatusLine {
	s.hints = hints
	return s
}

// fitHints returns the hints shown in w cells: the ones that give way first
// are dropped until the line fits, so no hint is ever cut in half.
func fitHints(hints []keyHint, w int) []keyHint {
	keep := append([]keyHint(nil), hints...)
	for hintsWidth(keep) > w {
		worst := -1
		for i, h := range keep {
			if h.drop > 0 && (worst < 0 || h.drop > keep[worst].drop) {
				worst = i
			}
		}
		if worst < 0 {
			break
		}
		keep = append(keep[:worst], keep[worst+1:]...)
	}
	return keep
}

// hintsPlain is the unstyled hint line, e.g. "j/k next/prev issue · ? all keys".
func hintsPlain(hints []keyHint) string {
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = strings.TrimSpace(h.key + " " + h.label)
	}
	return strings.Join(parts, " · ")
}

func hintsWidth(hints []keyHint) int { return lipgloss.Width(hintsPlain(hints)) }

// renderHints draws as many hints as fit in w cells: keys in the accent,
// what they do in grey.
func renderHints(hints []keyHint, w int) string {
	var sb strings.Builder
	for i, h := range fitHints(hints, w) {
		if i > 0 {
			sb.WriteString(hintSepStyle.Render(" · "))
		}
		if h.key == "" {
			sb.WriteString(hintLabelStyle.Render(h.label))
			continue
		}
		sb.WriteString(hintKeyStyle.Render(h.key) + hintLabelStyle.Render(" "+h.label))
	}
	return sb.String()
}

func (s StatusLine) SetWidth(w int) StatusLine {
	s.width = w
	return s
}

func (s StatusLine) View() string {
	hintLine := "  " + renderHints(s.hints, s.width-2)
	if s.message == "" {
		return hintLine
	}
	var msgLine string
	if s.isError {
		msgLine = statusErrorStyle.Width(s.width).Render("  ✗ " + s.message)
	} else {
		msgLine = statusSuccessStyle.Width(s.width).Render("  ✓ " + s.message)
	}
	return msgLine + "\n" + hintLine
}

// MapCLIError converts common CLI error patterns to human-readable messages.
func MapCLIError(raw string) string {
	lower := raw
	switch {
	case contains(lower, "401") || contains(lower, "unauthorized"):
		return "Authentication failed — run `jira auth login`"
	case contains(lower, "invalid jql") || contains(lower, "jql"):
		return "Invalid JQL: " + raw
	case contains(lower, "no such file") || contains(lower, "not found") || contains(lower, "executable"):
		return "CLI binary not found: " + raw
	case contains(lower, "403") || contains(lower, "forbidden"):
		return "Permission denied — check your Jira permissions"
	}
	return raw
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
