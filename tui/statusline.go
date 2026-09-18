package tui

import "github.com/charmbracelet/lipgloss"

var (
	statusSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00cc44")).Bold(true)
	statusErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4444")).Bold(true)
	statusIdleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
)

// StatusLine renders the last CLI operation result (or a context hint when idle).
type StatusLine struct {
	message string
	isError bool
	hint    string
	width   int
}

func (s StatusLine) SetMessage(msg string, isError bool) StatusLine {
	s.message = msg
	s.isError = isError
	return s
}

func (s StatusLine) SetHint(hint string) StatusLine {
	s.hint = hint
	return s
}

func (s StatusLine) SetWidth(w int) StatusLine {
	s.width = w
	return s
}

func (s StatusLine) View() string {
	hintLine := statusIdleStyle.Width(s.width).Render("  " + s.hint)
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
