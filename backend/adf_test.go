package backend

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestADFInlineCard(t *testing.T) {
	// Jira Smart Link pasted inline — the URL lives in attrs.url, not in text or content.
	raw := `{
		"type": "doc",
		"content": [{
			"type": "paragraph",
			"content": [
				{"type": "text", "text": "The server-side changes will be implemented in "},
				{"type": "inlineCard", "attrs": {"url": "https://example.atlassian.net/browse/OSAC-5141"}}
			]
		}]
	}`
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	got := adfToText(doc)
	want := "https://example.atlassian.net/browse/OSAC-5141"
	if !strings.Contains(got, want) {
		t.Errorf("expected URL %q in output, got %q", want, got)
	}
	if !strings.Contains(got, "The server-side changes will be implemented in ") {
		t.Errorf("leading text missing from output: %q", got)
	}
}

func TestADFBlockCard(t *testing.T) {
	raw := `{
		"type": "doc",
		"content": [{
			"type": "blockCard",
			"attrs": {"url": "https://example.atlassian.net/browse/OSAC-999"}
		}]
	}`
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	got := adfToText(doc)
	if !strings.Contains(got, "https://example.atlassian.net/browse/OSAC-999") {
		t.Errorf("blockCard URL not found in output: %q", got)
	}
}

func TestADFMention(t *testing.T) {
	raw := `{
		"type": "doc",
		"content": [{
			"type": "paragraph",
			"content": [
				{"type": "text", "text": "Hey "},
				{"type": "mention", "attrs": {"id": "abc123", "text": "@Alice"}}
			]
		}]
	}`
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	got := adfToText(doc)
	if !strings.Contains(got, "@Alice") {
		t.Errorf("mention text not found in output: %q", got)
	}
}

func TestADFEmoji(t *testing.T) {
	raw := `{
		"type": "doc",
		"content": [{
			"type": "paragraph",
			"content": [
				{"type": "emoji", "attrs": {"shortName": ":thumbsup:", "text": "👍"}}
			]
		}]
	}`
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	got := adfToText(doc)
	if !strings.Contains(got, "👍") {
		t.Errorf("emoji text not found in output: %q", got)
	}
}
