// Package ui handles terminal rendering with Glamour and Lipgloss.
//
// glamour converts Markdown → ANSI escape sequences for rich terminal
// output (syntax-highlighted code blocks, bold/italic, etc.).
// lipgloss adds structural styling: rounded borders, padding, colors.
package ui

import (
	"fmt"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

const termWidth = 100

var (
	// resultBoxStyle wraps the entire rendered output in a rounded border.
	resultBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#444444")).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1).
			Width(termWidth + 6)

	// footerStyle renders the attribution line below the result box.
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true).
			MarginTop(1)
)

// RenderContent renders Markdown text beautifully for the terminal.
// The input is expected to be valid Markdown (e.g., from
// mcp.FormatQuestionMarkdown); glamour converts it to ANSI and
// lipgloss adds a decorative border frame.
func RenderContent(text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("empty content")
	}

	// glamour.Render processes Markdown with the "dark" terminal theme,
	// producing syntax-highlighted code, styled headers, and more.
	rendered, err := glamour.Render(text, "dark")
	if err != nil {
		return "", fmt.Errorf("glamour render failed: %w", err)
	}

	output := resultBoxStyle.Render(rendered)
	footer := footerStyle.Render("  Powered by Stack Overflow via MCP")
	output += "\n" + footer + "\n"

	return output, nil
}

// RenderError produces a styled error panel for terminal display.
func RenderError(title, body string) string {
	errorBox := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF0000")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FF4444")).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1).
		Width(termWidth + 6)

	msg := fmt.Sprintf("✖ %s\n\n%s", title, body)
	return errorBox.Render(msg)
}
