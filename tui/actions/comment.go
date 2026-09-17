package actions

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommentSubmittedMsg is sent when the comment body is ready to post.
type CommentSubmittedMsg struct {
	IssueKey string
	Body     string
}

// CommentCancelledMsg is sent when the user cancels.
type CommentCancelledMsg struct{}

var commentOverlayStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#5555ff")).
	Padding(1, 2).
	Background(lipgloss.Color("#111111"))

// CommentModel handles the add-comment flow.
// When $EDITOR is set it suspends the TUI; otherwise it shows an inline textarea.
type CommentModel struct {
	issueKey  string
	textarea  textarea.Model
	useEditor bool
	tempFile  string
	width     int
	height    int
}

func NewCommentModel(issueKey string) CommentModel {
	_, useEditor := os.LookupEnv("EDITOR")
	ta := textarea.New()
	ta.Placeholder = "Write your comment here…"
	ta.CharLimit = 4096
	ta.SetWidth(46)
	ta.SetHeight(6)
	ta.Focus()
	return CommentModel{issueKey: issueKey, textarea: ta, useEditor: useEditor}
}

func (m CommentModel) SetSize(w, h int) CommentModel {
	m.width = w
	m.height = h
	taW := 50
	if taW > w-8 {
		taW = w - 8
	}
	m.textarea.SetWidth(taW - 4)
	return m
}

// LaunchEditorCmd suspends the TUI and opens $EDITOR on a temp file.
// Returns a tea.Cmd that can be returned from Init.
func (m *CommentModel) LaunchEditorCmd() tea.Cmd {
	if !m.useEditor {
		return nil
	}
	f, err := os.CreateTemp("", "jira-comment-*.md")
	if err != nil {
		return nil
	}
	f.Close()
	m.tempFile = f.Name()

	editor := os.Getenv("EDITOR")
	issueKey := m.issueKey
	tempFile := m.tempFile

	return tea.ExecProcess(exec.Command(editor, tempFile), func(err error) tea.Msg {
		if err != nil {
			return CommentCancelledMsg{}
		}
		data, readErr := os.ReadFile(tempFile)
		os.Remove(tempFile)
		if readErr != nil || len(strings.TrimSpace(string(data))) == 0 {
			return CommentCancelledMsg{}
		}
		return CommentSubmittedMsg{IssueKey: issueKey, Body: strings.TrimSpace(string(data))}
	})
}

func (m CommentModel) Update(msg tea.Msg) (CommentModel, tea.Cmd) {
	if m.useEditor {
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}
	switch key.String() {
	case "ctrl+d":
		body := strings.TrimSpace(m.textarea.Value())
		if body == "" {
			return m, func() tea.Msg { return CommentCancelledMsg{} }
		}
		return m, func() tea.Msg {
			return CommentSubmittedMsg{IssueKey: m.issueKey, Body: body}
		}
	case "esc":
		return m, func() tea.Msg { return CommentCancelledMsg{} }
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m CommentModel) View() string {
	overlayW := 58
	if m.width > 0 && overlayW > m.width-4 {
		overlayW = m.width - 4
	}

	var content string
	if m.useEditor {
		content = fmt.Sprintf(
			"%s\n\nOpening %s…\nSave and quit to post; leave empty or quit without saving to cancel.",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true).Render("Add Comment"),
			os.Getenv("EDITOR"),
		)
	} else {
		title := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true).Render("Add Comment")
		hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).
			Render("Ctrl+D to submit, Esc to cancel")
		content = title + "\n\n" + m.textarea.View() + "\n\n" + hint
	}

	overlay := commentOverlayStyle.Width(overlayW).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}
