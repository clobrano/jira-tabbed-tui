package backend

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles used when rendering ADF nodes to styled terminal strings.
var (
	adfBoldStyle = lipgloss.NewStyle().Bold(true)

	adfHeadingStyle = [4]lipgloss.Style{
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")),         // h1
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#dddddd")),         // h2
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#aaaaaa")),         // h3
		lipgloss.NewStyle().Bold(true).Faint(true).Foreground(lipgloss.Color("#888888")), // h4+
	}

	adfInlineCodeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7ec8e3"))

	adfCodeBlockStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#aaddaa")).
				Faint(true)

	adfLinkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5588ff")).
			Underline(true)

	adfLinkRefStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4466aa")).
			Faint(true)

	adfMentionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#aa88ff"))

	adfQuoteBarStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#5555ff")).
				Bold(true)

	adfQuoteTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888")).
				Italic(true)

	adfRuleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#444444"))
)

// renderADFDoc converts a parsed ADF node tree to a styled terminal string.
// It is the entry point called after JSON unmarshalling.
func renderADFDoc(node map[string]any) string {
	return renderNode(node)
}

func renderNode(node map[string]any) string {
	nodeType, _ := node["type"].(string)
	attrs, _ := node["attrs"].(map[string]any)

	switch nodeType {
	case "text":
		return renderTextNode(node)

	case "hardBreak":
		return "\n"

	case "inlineCard":
		url, _ := attrs["url"].(string)
		return adfLinkStyle.Render(url)

	case "blockCard":
		url, _ := attrs["url"].(string)
		return adfLinkStyle.Render(url) + "\n"

	case "mention":
		text, _ := attrs["text"].(string)
		return adfMentionStyle.Render(text)

	case "emoji":
		if text, ok := attrs["text"].(string); ok && text != "" {
			return text
		}
		shortName, _ := attrs["shortName"].(string)
		return shortName

	case "paragraph":
		return renderChildren(node) + "\n"

	case "heading":
		level := 1
		if l, ok := attrs["level"].(float64); ok {
			level = int(l)
		}
		idx := level - 1
		if idx > 3 {
			idx = 3
		}
		prefix := strings.Repeat("#", level) + " "
		content := strings.TrimRight(renderChildren(node), "\n")
		return "\n" + adfHeadingStyle[idx].Render(prefix+content) + "\n"

	case "blockquote":
		inner := renderChildren(node)
		return quoteLines(inner) + "\n"

	case "codeBlock":
		lang, _ := attrs["language"].(string)
		code := rawText(node)
		var sb strings.Builder
		if lang != "" {
			sb.WriteString(adfInlineCodeStyle.Faint(true).Render("["+lang+"]") + "\n")
		}
		for _, line := range strings.Split(strings.TrimRight(code, "\n"), "\n") {
			sb.WriteString(adfCodeBlockStyle.Render("  "+line) + "\n")
		}
		return sb.String()

	case "rule":
		return adfRuleStyle.Render(strings.Repeat("─", 48)) + "\n"

	case "bulletList":
		return renderList(node, false)

	case "orderedList":
		return renderList(node, true)

	case "listItem":
		return renderListItem(node)
	}

	// "doc" and unknown containers: recurse into children.
	return renderChildren(node)
}

// renderTextNode applies marks (bold, italic, link, code …) to a text leaf.
func renderTextNode(node map[string]any) string {
	text, _ := node["text"].(string)
	if text == "" {
		return ""
	}

	s := lipgloss.NewStyle()
	var linkHref string

	marks, _ := node["marks"].([]any)
	for _, m := range marks {
		mark, _ := m.(map[string]any)
		markType, _ := mark["type"].(string)
		markAttrs, _ := mark["attrs"].(map[string]any)

		switch markType {
		case "strong":
			s = s.Bold(true)
		case "em":
			s = s.Italic(true)
		case "strike":
			s = s.Strikethrough(true)
		case "underline":
			s = s.Underline(true)
		case "code":
			s = s.Foreground(lipgloss.Color("#7ec8e3"))
		case "textColor":
			if color, ok := markAttrs["color"].(string); ok {
				s = s.Foreground(lipgloss.Color(color))
			}
		case "link":
			href, _ := markAttrs["href"].(string)
			linkHref = href
			s = s.Foreground(lipgloss.Color("#5588ff")).Underline(true)
		}
	}

	result := s.Render(text)

	// For named links (display text ≠ URL) append the href in a dim style.
	if linkHref != "" && linkHref != text {
		result += adfLinkRefStyle.Render(" [" + linkHref + "]")
	}

	return result
}

func renderChildren(node map[string]any) string {
	content, _ := node["content"].([]any)
	var sb strings.Builder
	for _, child := range content {
		if childMap, ok := child.(map[string]any); ok {
			sb.WriteString(renderNode(childMap))
		}
	}
	return sb.String()
}

func renderList(node map[string]any, ordered bool) string {
	content, _ := node["content"].([]any)
	var sb strings.Builder
	for i, child := range content {
		childMap, _ := child.(map[string]any)
		var bullet string
		if ordered {
			bullet = adfBoldStyle.Render(fmt.Sprintf("%d.", i+1)) + " "
		} else {
			bullet = adfBoldStyle.Render("•") + " "
		}
		sb.WriteString(bullet)
		sb.WriteString(renderListItem(childMap))
	}
	return sb.String()
}

func renderListItem(node map[string]any) string {
	content, _ := node["content"].([]any)
	var sb strings.Builder
	first := true
	for _, child := range content {
		childMap, _ := child.(map[string]any)
		childType, _ := childMap["type"].(string)

		if first && childType == "paragraph" {
			// First paragraph: render inline on the bullet's line.
			text := strings.TrimRight(renderChildren(childMap), "\n")
			sb.WriteString(text + "\n")
			first = false
		} else {
			// Nested list or continuation paragraph: indent by 3 spaces.
			nested := renderNode(childMap)
			for _, line := range strings.Split(strings.TrimRight(nested, "\n"), "\n") {
				sb.WriteString("   " + line + "\n")
			}
		}
	}
	return sb.String()
}

// quoteLines prefixes each non-blank line with a styled "│ ".
func quoteLines(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	out := make([]string, 0, len(lines))
	bar := adfQuoteBarStyle.Render("│")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			out = append(out, adfQuoteBarStyle.Render("│"))
		} else {
			out = append(out, bar+" "+adfQuoteTextStyle.Render(stripANSI(line)))
		}
	}
	return strings.Join(out, "\n")
}

// rawText extracts unstyled text from a node (used for code blocks where marks are irrelevant).
func rawText(node map[string]any) string {
	if text, ok := node["text"].(string); ok {
		return text
	}
	content, _ := node["content"].([]any)
	var sb strings.Builder
	for _, child := range content {
		if childMap, ok := child.(map[string]any); ok {
			sb.WriteString(rawText(childMap))
		}
	}
	return sb.String()
}

// stripANSI removes ANSI escape sequences from s.
// Used when re-styling already-rendered text (e.g. blockquote lines).
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until 'm'
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // skip 'm'
		} else {
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}
