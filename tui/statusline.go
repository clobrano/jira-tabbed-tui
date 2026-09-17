package tui

import "github.com/charmbracelet/lipgloss"

var (
	statusSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00cc44")).Bold(true)
	statusErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4444")).Bold(true)
	statusIdleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)

// StatusLine renders the last CLI operation result below the tab bar.
type StatusLine struct {
	message string
	isError bool
	width   int
}

func (s StatusLine) SetMessage(msg string, isError bool) StatusLine {
	s.message = msg
	s.isError = isError
	return s
}

func (s StatusLine) SetWidth(w int) StatusLine {
	s.width = w
	return s
}

func (s StatusLine) View() string {
	if s.message == "" {
		return statusIdleStyle.Width(s.width).Render("  Ready")
	}
	if s.isError {
		return statusErrorStyle.Width(s.width).Render("  ✗ " + s.message)
	}
	return statusSuccessStyle.Width(s.width).Render("  ✓ " + s.message)
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
