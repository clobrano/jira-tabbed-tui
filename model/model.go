package model

// Issue holds the five list columns shown in the tab view.
type Issue struct {
	Key      string
	Type     string
	Summary  string
	Priority string
	DueDate  string
	Status   string
}

// IssueDetail embeds Issue and adds description, comments, and all raw fields.
type IssueDetail struct {
	Issue
	Description string
	Comments    []Comment
	Fields      map[string]any
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
