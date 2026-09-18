package model

// Issue holds the fields shown in the tab list view.
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
}

// IssueDetail embeds Issue and adds description, comments, links, and all raw fields.
type IssueDetail struct {
	Issue
	Description string
	Comments    []Comment
	Links       []IssueLink
	Fields      map[string]any
}

// IssueLink is a single Jira issue link (e.g. "blocks", "is blocked by", "relates to").
// When URL is non-empty the link is a web/remote link; Key holds its display title.
type IssueLink struct {
	Type    string
	Key     string
	Summary string
	Status  string
	URL     string // non-empty for web/remote links; empty for Jira issue links
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
