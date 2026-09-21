package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/clobrano/jira-tabbed-tui/model"
)

// FetchIssueList fetches issues matching jql. maxResults=0 uses the server default.
func FetchIssueList(r Runner, jql string, maxResults int) ([]model.Issue, int, error) {
	args := []string{"issue", "list", "--raw", "-q", jql}
	if maxResults > 0 {
		args = append(args, "--paginate", fmt.Sprintf("0:%d", maxResults))
	}
	data, err := r.Run(args...)
	if err != nil {
		return nil, 0, err
	}
	return parseIssueList(data)
}

// FetchIssueListPage fetches a page of issues starting at startAt.
func FetchIssueListPage(r Runner, jql string, maxResults, startAt int) ([]model.Issue, int, error) {
	args := []string{"issue", "list", "--raw", "-q", jql,
		"--paginate", fmt.Sprintf("%d:%d", startAt, maxResults)}
	data, err := r.Run(args...)
	if err != nil {
		return nil, 0, err
	}
	return parseIssueList(data)
}

type listResponse struct {
	StartAt    int        `json:"startAt"`
	MaxResults int        `json:"maxResults"`
	Total      int        `json:"total"`
	Issues     []rawIssue `json:"issues"`
}

type rawIssue struct {
	Key    string          `json:"key"`
	Fields json.RawMessage `json:"fields"`
}

type rawIssueFields struct {
	Summary   string          `json:"summary"`
	IssueType struct {
		Name string `json:"name"`
	} `json:"issuetype"`
	Priority struct {
		Name string `json:"name"`
	} `json:"priority"`
	Status struct {
		Name string `json:"name"`
	} `json:"status"`
	DueDate  *string `json:"duedate"`
	Assignee *struct {
		DisplayName string `json:"displayName"`
	} `json:"assignee"`
	Reporter *struct {
		DisplayName string `json:"displayName"`
	} `json:"reporter"`
	Labels      []string        `json:"labels"`
	Created     string          `json:"created"`
	Updated     string          `json:"updated"`
	Description json.RawMessage `json:"description"`
	Comment     *struct {
		Comments []rawComment `json:"comments"`
	} `json:"comment"`
	IssueLinks []rawIssueLink   `json:"issuelinks"`
	Subtasks   []rawLinkedIssue `json:"subtasks"`
}

type rawIssueLink struct {
	Type struct {
		Inward  string `json:"inward"`
		Outward string `json:"outward"`
	} `json:"type"`
	InwardIssue  *rawLinkedIssue `json:"inwardIssue"`
	OutwardIssue *rawLinkedIssue `json:"outwardIssue"`
}

type rawLinkedIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  struct {
			Name string `json:"name"`
		} `json:"status"`
	} `json:"fields"`
}

type rawComment struct {
	Author  struct{ DisplayName string `json:"displayName"` } `json:"author"`
	Created string                                            `json:"created"`
	Body    json.RawMessage                                   `json:"body"`
}

func parseFieldMap(data json.RawMessage) map[string]any {
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return make(map[string]any)
	}

	fieldMap := make(map[string]any, len(rawMap))
	for k, v := range rawMap {
		var val any
		if err := json.Unmarshal(v, &val); err == nil {
			fieldMap[k] = val
		}
	}
	return fieldMap
}

func parseIssueList(data []byte) ([]model.Issue, int, error) {
	// jira-cli --raw can return either the Jira API wrapper object
	// {"total":N,"issues":[...]} or a bare array [...].
	trimmed := bytes.TrimSpace(data)
	var rawIssues []rawIssue
	total := 0
	if len(trimmed) > 0 && trimmed[0] == '[' {
		if err := json.Unmarshal(data, &rawIssues); err != nil {
			return nil, 0, fmt.Errorf("parsing issue list (array): %w", err)
		}
		total = len(rawIssues)
	} else {
		var resp listResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, 0, fmt.Errorf("parsing issue list: %w", err)
		}
		rawIssues = resp.Issues
		total = resp.Total
	}
	issues := make([]model.Issue, 0, len(rawIssues))
	for _, raw := range rawIssues {
		var f rawIssueFields
		if err := json.Unmarshal(raw.Fields, &f); err != nil {
			continue
		}
		iss := model.Issue{
			Key:      raw.Key,
			Type:     f.IssueType.Name,
			Summary:  f.Summary,
			Priority: f.Priority.Name,
			Status:   f.Status.Name,
			Fields:   parseFieldMap(raw.Fields),
		}
		if f.DueDate != nil {
			iss.DueDate = *f.DueDate
		}
		if f.Assignee != nil {
			iss.Assignee = f.Assignee.DisplayName
		}
		iss.Created = formatDate(f.Created)
		iss.Updated = formatDate(f.Updated)
		issues = append(issues, iss)
	}
	return issues, total, nil
}

// FetchIssueDetail fetches full detail for one issue.
func FetchIssueDetail(r Runner, key string) (model.IssueDetail, error) {
	data, err := r.Run("issue", "view", key, "--raw")
	if err != nil {
		return model.IssueDetail{}, err
	}
	return parseIssueDetail(data)
}

func parseIssueDetail(data []byte) (model.IssueDetail, error) {
	var raw rawIssue
	if err := json.Unmarshal(data, &raw); err != nil {
		return model.IssueDetail{}, fmt.Errorf("parsing issue detail: %w", err)
	}
	var f rawIssueFields
	if err := json.Unmarshal(raw.Fields, &f); err != nil {
		return model.IssueDetail{}, fmt.Errorf("parsing issue fields: %w", err)
	}

	// Collect all fields into a generic map for list columns, sidebar, and discovery.
	fieldMap := parseFieldMap(raw.Fields)

	detail := model.IssueDetail{
		Issue: model.Issue{
			Key:      raw.Key,
			Type:     f.IssueType.Name,
			Summary:  f.Summary,
			Priority: f.Priority.Name,
			Status:   f.Status.Name,
			Fields:   fieldMap,
		},
	}
	if f.DueDate != nil {
		detail.DueDate = *f.DueDate
		fieldMap["duedate"] = *f.DueDate
	}
	if f.Assignee != nil {
		fieldMap["assignee"] = f.Assignee.DisplayName
	} else {
		fieldMap["assignee"] = ""
	}
	if f.Reporter != nil {
		fieldMap["reporter"] = f.Reporter.DisplayName
	}
	fieldMap["labels"] = strings.Join(f.Labels, ", ")
	fieldMap["priority"] = f.Priority.Name
	fieldMap["status"] = f.Status.Name
	fieldMap["issuetype"] = f.IssueType.Name
	fieldMap["summary"] = f.Summary

	detail.Description = extractText(f.Description)

	if f.Comment != nil {
		for _, c := range f.Comment.Comments {
			detail.Comments = append(detail.Comments, model.Comment{
				Author:  c.Author.DisplayName,
				Created: formatDate(c.Created),
				Body:    extractText(c.Body),
			})
		}
		// Reverse to newest-first.
		for i, j := 0, len(detail.Comments)-1; i < j; i, j = i+1, j-1 {
			detail.Comments[i], detail.Comments[j] = detail.Comments[j], detail.Comments[i]
		}
	}

	for _, sub := range f.Subtasks {
		detail.Links = append(detail.Links, model.IssueLink{
			Type:    "subtask",
			Key:     sub.Key,
			Summary: sub.Fields.Summary,
			Status:  sub.Fields.Status.Name,
		})
	}

	for _, link := range f.IssueLinks {
		var linkType string
		var linked *rawLinkedIssue
		if link.InwardIssue != nil {
			linkType = link.Type.Inward
			linked = link.InwardIssue
		} else if link.OutwardIssue != nil {
			linkType = link.Type.Outward
			linked = link.OutwardIssue
		}
		if linked != nil {
			detail.Links = append(detail.Links, model.IssueLink{
				Type:    linkType,
				Key:     linked.Key,
				Summary: linked.Fields.Summary,
				Status:  linked.Fields.Status.Name,
			})
		}
	}

	return detail, nil
}

// FetchTransitions fetches the available status transitions for an issue.
func FetchTransitions(r Runner, key string) ([]model.Transition, error) {
	data, err := r.Run("issue", "transitions", key, "--raw")
	if err != nil {
		return nil, err
	}
	return parseTransitions(data)
}

type transitionsResponse struct {
	Transitions []struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		IsCurrentStatus bool   `json:"isCurrentStatus"`
	} `json:"transitions"`
}

func parseTransitions(data []byte) ([]model.Transition, error) {
	var resp transitionsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing transitions: %w", err)
	}
	ts := make([]model.Transition, 0, len(resp.Transitions))
	for _, t := range resp.Transitions {
		ts = append(ts, model.Transition{
			ID:              t.ID,
			Name:            t.Name,
			IsCurrentStatus: t.IsCurrentStatus,
		})
	}
	return ts, nil
}

// ApplyTransition moves an issue to a new status by transition name.
func ApplyTransition(r Runner, key, transitionName string) error {
	_, err := r.Run("issue", "move", key, transitionName)
	return err
}

// AddLabels adds one or more labels to an issue via jira issue edit --label.
func AddLabels(r Runner, key string, labels []string) error {
	args := make([]string, 0, 2+2*len(labels))
	args = append(args, "issue", "edit", key)
	for _, l := range labels {
		args = append(args, "--label", l)
	}
	_, err := r.Run(args...)
	return err
}

// EditField sets a single field value on an issue via jira issue edit.
// For built-in flags (priority, summary, duedate) it uses the named flag;
// for custom fields it uses --custom-field fieldID=value.
func EditField(r Runner, key, fieldID, value string) error {
	var args []string
	switch fieldID {
	case "priority":
		args = []string{"issue", "edit", key, "--priority", value}
	case "summary":
		args = []string{"issue", "edit", key, "--summary", value}
	case "duedate":
		args = []string{"issue", "edit", key, "--no-input", "--custom-field", fieldID + "=" + value}
	default:
		args = []string{"issue", "edit", key, "--no-input", "--custom-field", fieldID + "=" + value}
	}
	_, err := r.Run(args...)
	return err
}

// AssignIssue assigns an issue to a user. login accepts email or display name
// (jira-cli resolves it internally via a user search).
func AssignIssue(r Runner, key, login string) error {
	_, err := r.Run("issue", "assign", key, login)
	return err
}

// AddComment posts a new comment on an issue.
func AddComment(r Runner, key, body string) error {
	_, err := r.Run("issue", "comment", "add", key, body)
	return err
}

// FetchAllFields returns every field present on an issue, with display names.
func FetchAllFields(r Runner, key string) ([]model.Field, error) {
	detail, err := FetchIssueDetail(r, key)
	if err != nil {
		return nil, err
	}
	fields := make([]model.Field, 0, len(detail.Fields))
	for k := range detail.Fields {
		fields = append(fields, model.Field{
			ID:          k,
			DisplayName: fieldDisplayName(k),
		})
	}
	return fields, nil
}

var knownFieldNames = map[string]string{
	"summary":     "Summary",
	"description": "Description",
	"issuetype":   "Issue Type",
	"priority":    "Priority",
	"status":      "Status",
	"assignee":    "Assignee",
	"reporter":    "Reporter",
	"labels":      "Labels",
	"duedate":     "Due Date",
	"created":     "Created",
	"updated":     "Updated",
}

func fieldDisplayName(id string) string {
	if name, ok := knownFieldNames[id]; ok {
		return name
	}
	return id
}

// extractText converts a JSON value (string or ADF document) to a styled terminal string.
func extractText(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	// Jira Cloud uses ADF objects; older/simple bodies may be plain strings.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return s
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return string(data)
	}
	return renderADFDoc(doc)
}

func formatDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}
