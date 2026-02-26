// Package mcp – parse.go handles deserialization of JSON responses
// from the Stack Overflow MCP server tools (so_search, get_content).
//
// Both tools return responses in this envelope:
//
//	{
//	  "Items": [
//	    {
//	      "Site":  "Stack Overflow",
//	      "Type":  "Question",
//	      "Id":    "1752414",
//	      "Data":  { ... question/answer fields ... }
//	    }
//	  ],
//	  "Errors": []
//	}
//
// "Data" includes tags, score, body_markdown, owner, answers (when
// the item was returned by so_search), and more.  The "answers"
// sub-array is embedded inside each question's Data block.
package mcp

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"
)

// ---------- JSON structs matching the MCP server response ----------

// SOResponse is the top-level envelope returned by so_search / get_content.
type SOResponse struct {
	Items  []SOItem `json:"Items"`
	Errors []any    `json:"Errors"`
}

// SOItem represents a single search result (question, answer, etc.).
type SOItem struct {
	Site string       `json:"Site"`
	Type string       `json:"Type"`
	ID   string       `json:"Id"`
	Data QuestionData `json:"Data"`
}

// QuestionData holds the rich payload for a question (and inline answers).
type QuestionData struct {
	Tags             []string     `json:"tags"`
	Owner            OwnerData    `json:"owner"`
	IsAnswered       bool         `json:"is_answered"`
	ViewCount        int          `json:"view_count"`
	AnswerCount      int          `json:"answer_count"`
	Score            int          `json:"score"`
	AcceptedAnswerID int          `json:"accepted_answer_id"`
	CreationDate     int64        `json:"creation_date"`
	LastActivityDate int64        `json:"last_activity_date"`
	QuestionID       int          `json:"question_id"`
	BodyMarkdown     string       `json:"body_markdown"`
	Link             string       `json:"link"`
	Title            string       `json:"title"`
	Answers          []AnswerData `json:"answers"` // embedded in so_search results
}

// AnswerData holds a single answer embedded inside a question's search result.
type AnswerData struct {
	Owner            OwnerData `json:"owner"`
	IsAccepted       bool      `json:"is_accepted"`
	LastActivityDate int64     `json:"last_activity_date"`
	AnswerID         int       `json:"answer_id"`
	Score            int       `json:"score"`
	BodyMarkdown     string    `json:"body_markdown"`
	Link             string    `json:"link"`
	Title            string    `json:"title"`
}

// OwnerData holds the author information.
type OwnerData struct {
	DisplayName string `json:"display_name"`
	Link        string `json:"link"`
}

// ---------- Parsing helpers ----------

// ParseResponse deserializes the JSON text returned by an MCP tool call.
func ParseResponse(text string) (*SOResponse, error) {
	var resp SOResponse
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return nil, fmt.Errorf("parse SO response: %w", err)
	}
	return &resp, nil
}

// BestQuestion returns the highest-scored question from the response,
// optionally preferring questions whose tags intersect with hints.
// Tag hints are lowercase strings like "go", "python", "javascript".
func BestQuestion(resp *SOResponse, tagHints []string) *QuestionData {
	if resp == nil || len(resp.Items) == 0 {
		return nil
	}

	// Filter to questions only (skip if Type is populated and not "Question").
	var questions []*QuestionData
	for i := range resp.Items {
		item := &resp.Items[i]
		if item.Type != "" && item.Type != "Question" {
			continue
		}
		questions = append(questions, &item.Data)
	}
	if len(questions) == 0 {
		return nil
	}

	// If tag hints are provided, prefer questions that match any tag.
	if len(tagHints) > 0 {
		hintSet := make(map[string]bool, len(tagHints))
		for _, h := range tagHints {
			hintSet[strings.ToLower(h)] = true
		}
		var matching []*QuestionData
		for _, q := range questions {
			for _, t := range q.Tags {
				if hintSet[strings.ToLower(t)] {
					matching = append(matching, q)
					break
				}
			}
		}
		if len(matching) > 0 {
			questions = matching
		}
	}

	// Sort by score descending; tie-break by view count.
	sort.Slice(questions, func(i, j int) bool {
		if questions[i].Score != questions[j].Score {
			return questions[i].Score > questions[j].Score
		}
		return questions[i].ViewCount > questions[j].ViewCount
	})

	return questions[0]
}

// ---------- Markdown formatting ----------

// FormatQuestionMarkdown builds a human-readable Markdown document from
// a single question (and its embedded answers, if available).
// The result is ready to be rendered by glamour.
func FormatQuestionMarkdown(q *QuestionData, maxAnswers int) string {
	if q == nil {
		return ""
	}

	var b strings.Builder

	// --- Title ---
	title := decodeHTML(q.Title)
	b.WriteString(fmt.Sprintf("# %s\n\n", title))

	// --- Meta line ---
	meta := fmt.Sprintf("Score: **%d**  |  Views: **%s**  |  Answers: **%d**",
		q.Score, formatNumber(q.ViewCount), q.AnswerCount)
	if q.IsAnswered {
		meta += "  |  ✅ Answered"
	}
	b.WriteString(meta + "\n\n")

	// --- Tags ---
	if len(q.Tags) > 0 {
		var tagParts []string
		for _, t := range q.Tags {
			tagParts = append(tagParts, "`"+t+"`")
		}
		b.WriteString(strings.Join(tagParts, "  ") + "\n\n")
	}

	// --- Asked by / date ---
	if q.Owner.DisplayName != "" {
		dateStr := ""
		if q.CreationDate > 0 {
			dateStr = time.Unix(q.CreationDate, 0).Format("Jan 2, 2006")
		}
		b.WriteString(fmt.Sprintf("Asked by **%s**", decodeHTML(q.Owner.DisplayName)))
		if dateStr != "" {
			b.WriteString(fmt.Sprintf(" on %s", dateStr))
		}
		b.WriteString("\n\n")
	}

	b.WriteString("---\n\n")

	// --- Question body ---
	body := decodeHTML(q.BodyMarkdown)
	b.WriteString(body + "\n\n")

	// --- Link ---
	if q.Link != "" {
		b.WriteString(fmt.Sprintf("🔗 %s\n\n", q.Link))
	}

	// --- Answers ---
	if len(q.Answers) > 0 {
		// Sort: accepted first, then by score descending.
		answers := make([]AnswerData, len(q.Answers))
		copy(answers, q.Answers)
		sort.Slice(answers, func(i, j int) bool {
			if answers[i].IsAccepted != answers[j].IsAccepted {
				return answers[i].IsAccepted
			}
			return answers[i].Score > answers[j].Score
		})

		shown := maxAnswers
		if shown <= 0 || shown > len(answers) {
			shown = len(answers)
		}

		b.WriteString("---\n\n")
		b.WriteString(fmt.Sprintf("## Top %d Answer(s)\n\n", shown))

		for i := 0; i < shown; i++ {
			a := answers[i]
			label := fmt.Sprintf("### Answer %d", i+1)
			if a.IsAccepted {
				label += "  ✅ Accepted"
			}
			if a.Score > 0 {
				label += fmt.Sprintf("  (Score: %d)", a.Score)
			}
			b.WriteString(label + "\n\n")

			if a.Owner.DisplayName != "" {
				b.WriteString(fmt.Sprintf("By **%s**\n\n", decodeHTML(a.Owner.DisplayName)))
			}

			ansBody := decodeHTML(a.BodyMarkdown)
			b.WriteString(ansBody + "\n\n")

			if i < shown-1 {
				b.WriteString("---\n\n")
			}
		}

		if len(answers) > shown {
			b.WriteString(fmt.Sprintf("\n*(%d more answers on Stack Overflow)*\n", len(answers)-shown))
		}
	} else if q.AnswerCount > 0 && q.Link != "" {
		// The server didn't embed answers (common with get_content responses).
		// Show a clear call-to-action so the user knows answers exist.
		b.WriteString("---\n\n")
		answerWord := "answers"
		if q.AnswerCount == 1 {
			answerWord = "answer"
		}
		b.WriteString(fmt.Sprintf("📝 **%d %s** available on Stack Overflow:\n", q.AnswerCount, answerWord))
		b.WriteString(fmt.Sprintf("%s\n", q.Link))
	}

	return b.String()
}

// FormatSearchResults builds a Markdown summary listing the top N search
// results as a quick-pick list.  Used when no single best question
// can be identified.
func FormatSearchResults(resp *SOResponse, maxResults int) string {
	if resp == nil || len(resp.Items) == 0 {
		return "No results found."
	}

	var b strings.Builder
	b.WriteString("# Stack Overflow Search Results\n\n")

	shown := maxResults
	if shown <= 0 || shown > len(resp.Items) {
		shown = len(resp.Items)
	}

	for i := 0; i < shown; i++ {
		q := resp.Items[i].Data
		title := decodeHTML(q.Title)
		tags := ""
		if len(q.Tags) > 0 {
			var ts []string
			for _, t := range q.Tags {
				ts = append(ts, "`"+t+"`")
			}
			tags = " — " + strings.Join(ts, " ")
		}
		accepted := ""
		if q.IsAnswered {
			accepted = " ✅"
		}
		b.WriteString(fmt.Sprintf("%d. **%s**%s  \n   Score: %d | Answers: %d%s  \n   %s\n\n",
			i+1, title, accepted, q.Score, q.AnswerCount, tags, q.Link))
	}

	return b.String()
}

// ---------- Utility ----------

// decodeHTML unescapes HTML entities (&#39; → ', &amp; → &, etc.)
// that Stack Overflow embeds in body_markdown fields.
func decodeHTML(s string) string {
	// Go's html.UnescapeString handles standard named and numeric entities.
	return html.UnescapeString(s)
}

// formatNumber returns a human-friendly number string (e.g., 178410 → "178,410").
func formatNumber(n int) string {
	if n < 0 {
		return "-" + formatNumber(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}
