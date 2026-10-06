package model

// Issue holds the fields shown in the tab list view and the raw Jira fields
// needed by configured columns.
type Issue struct {
	Key      string
	Type     string
	Summary  string
	Priority string
	DueDate  string
	Status   string
	Assignee string
	Created  string
	Updated  string
	Fields   map[string]any
}

// IssueDetail embeds Issue and adds description, comments, links, and all raw fields.
type IssueDetail struct {
	Issue
	Description string
	Comments    []Comment
	Links       []IssueLink
}

// IssueLink is a single Jira issue link (e.g. "blocks", "is blocked by", "relates to").
// When URL is non-empty the link is a web/remote link; Key holds its display title.
type IssueLink struct {
	Type      string // relationship, e.g. "blocks", "subtask", "mentioned in"
	Key       string
	Summary   string
	Status    string // Jira status, or the pull request state for PR web links
	IssueType string // Jira issue type (Bug, Task, …); empty for web links
	URL       string // non-empty for web/remote links; empty for Jira issue links
}

// Comment is a single Jira comment.
type Comment struct {
	Author  string
	Created string
	Body    string
}

// Transition is a valid status move for an issue.
type Transition struct {
	ID              string
	Name            string
	IsCurrentStatus bool
}

// Field is a Jira field descriptor used in field discovery.
type Field struct {
	ID          string
	DisplayName string
}

// User is a Jira user returned by the assignable-search API.
type User struct {
	AccountID   string
	DisplayName string
	Email       string
}
