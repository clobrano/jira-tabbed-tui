package backend

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustParseADF(t *testing.T, raw string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("parsing ADF: %v", err)
	}
	return doc
}

func TestADFInlineCard(t *testing.T) {
	doc := mustParseADF(t, `{
		"type": "doc",
		"content": [{"type": "paragraph", "content": [
			{"type": "text", "text": "The server-side changes will be implemented in "},
			{"type": "inlineCard", "attrs": {"url": "https://example.atlassian.net/browse/OSAC-5141"}}
		]}]
	}`)
	got := renderADFDoc(doc)
	if !strings.Contains(got, "https://example.atlassian.net/browse/OSAC-5141") {
		t.Errorf("URL missing from output: %q", got)
	}
	if !strings.Contains(got, "The server-side changes will be implemented in ") {
		t.Errorf("leading text missing: %q", got)
	}
}

func TestADFBlockCard(t *testing.T) {
	doc := mustParseADF(t, `{
		"type": "doc",
		"content": [{"type": "blockCard", "attrs": {"url": "https://example.atlassian.net/browse/OSAC-999"}}]
	}`)
	got := renderADFDoc(doc)
	if !strings.Contains(got, "https://example.atlassian.net/browse/OSAC-999") {
		t.Errorf("blockCard URL missing: %q", got)
	}
}

func TestADFMention(t *testing.T) {
	doc := mustParseADF(t, `{
		"type": "doc",
		"content": [{"type": "paragraph", "content": [
			{"type": "text", "text": "Hey "},
			{"type": "mention", "attrs": {"id": "abc123", "text": "@Alice"}}
		]}]
	}`)
	got := renderADFDoc(doc)
	if !strings.Contains(got, "@Alice") {
		t.Errorf("mention text missing: %q", got)
	}
}

func TestADFEmoji(t *testing.T) {
	doc := mustParseADF(t, `{
		"type": "doc",
		"content": [{"type": "paragraph", "content": [
			{"type": "emoji", "attrs": {"shortName": ":thumbsup:", "text": "👍"}}
		]}]
	}`)
	got := renderADFDoc(doc)
	if !strings.Contains(got, "👍") {
		t.Errorf("emoji text missing: %q", got)
	}
}

func TestADFTextMarks(t *testing.T) {
	doc := mustParseADF(t, `{
		"type": "doc",
		"content": [{"type": "paragraph", "content": [
			{"type": "text", "text": "bold", "marks": [{"type": "strong"}]},
			{"type": "text", "text": " "},
			{"type": "text", "text": "italic", "marks": [{"type": "em"}]},
			{"type": "text", "text": " "},
			{"type": "text", "text": "code", "marks": [{"type": "code"}]}
		]}]
	}`)
	got := renderADFDoc(doc)
	for _, word := range []string{"bold", "italic", "code"} {
		if !strings.Contains(got, word) {
			t.Errorf("word %q missing from output: %q", word, got)
		}
	}
}

func TestADFLink(t *testing.T) {
	doc := mustParseADF(t, `{
		"type": "doc",
		"content": [{"type": "paragraph", "content": [
			{"type": "text", "text": "click here", "marks": [{"type": "link", "attrs": {"href": "https://example.com/page"}}]}
		]}]
	}`)
	got := renderADFDoc(doc)
	if !strings.Contains(got, "click here") {
		t.Errorf("link text missing: %q", got)
	}
	if !strings.Contains(got, "https://example.com/page") {
		t.Errorf("link href missing: %q", got)
	}
}

func TestADFBulletList(t *testing.T) {
	doc := mustParseADF(t, `{
		"type": "doc",
		"content": [{"type": "bulletList", "content": [
			{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Alpha"}]}]},
			{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Beta"}]}]}
		]}]
	}`)
	got := renderADFDoc(doc)
	if !strings.Contains(got, "Alpha") || !strings.Contains(got, "Beta") {
		t.Errorf("list items missing: %q", got)
	}
	if !strings.Contains(got, "•") {
		t.Errorf("bullet symbol missing: %q", got)
	}
}

func TestADFOrderedList(t *testing.T) {
	doc := mustParseADF(t, `{
		"type": "doc",
		"content": [{"type": "orderedList", "content": [
			{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "First"}]}]},
			{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Second"}]}]}
		]}]
	}`)
	got := renderADFDoc(doc)
	if !strings.Contains(got, "First") || !strings.Contains(got, "Second") {
		t.Errorf("ordered list items missing: %q", got)
	}
	if !strings.Contains(got, "1.") || !strings.Contains(got, "2.") {
		t.Errorf("ordinal numbers missing: %q", got)
	}
}

func TestADFHeading(t *testing.T) {
	doc := mustParseADF(t, `{
		"type": "doc",
		"content": [
			{"type": "heading", "attrs": {"level": 1}, "content": [{"type": "text", "text": "Title"}]},
			{"type": "heading", "attrs": {"level": 2}, "content": [{"type": "text", "text": "Subtitle"}]}
		]
	}`)
	got := renderADFDoc(doc)
	if !strings.Contains(got, "# ") || !strings.Contains(got, "Title") {
		t.Errorf("h1 missing: %q", got)
	}
	if !strings.Contains(got, "## ") || !strings.Contains(got, "Subtitle") {
		t.Errorf("h2 missing: %q", got)
	}
}

func TestADFBlockquote(t *testing.T) {
	doc := mustParseADF(t, `{
		"type": "doc",
		"content": [{"type": "blockquote", "content": [
			{"type": "paragraph", "content": [{"type": "text", "text": "Quoted text"}]}
		]}]
	}`)
	got := renderADFDoc(doc)
	if !strings.Contains(got, "Quoted text") {
		t.Errorf("blockquote content missing: %q", got)
	}
	if !strings.Contains(got, "│") {
		t.Errorf("blockquote bar missing: %q", got)
	}
}

func TestADFCodeBlock(t *testing.T) {
	doc := mustParseADF(t, `{
		"type": "doc",
		"content": [{"type": "codeBlock", "attrs": {"language": "go"}, "content": [
			{"type": "text", "text": "fmt.Println(\"hello\")"}
		]}]
	}`)
	got := renderADFDoc(doc)
	if !strings.Contains(got, "fmt.Println") {
		t.Errorf("code block content missing: %q", got)
	}
	if !strings.Contains(got, "go") {
		t.Errorf("language tag missing: %q", got)
	}
}
