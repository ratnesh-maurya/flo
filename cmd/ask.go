package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/ratnesh-maurya/flo/pkg/mcp"
	"github.com/ratnesh-maurya/flo/pkg/ui"
	"github.com/spf13/cobra"
)

// maxAnswersToShow controls how many answers are displayed per question.
const maxAnswersToShow = 3

var askCmd = &cobra.Command{
	Use:   `ask "<query>"`,
	Short: "Search Stack Overflow for a question",
	Long:  "Search Stack Overflow directly from your terminal.\nExample: flo ask \"how to reverse a string in go\"",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runAsk,
}

func init() {
	rootCmd.AddCommand(askCmd)
}

// runAsk is the main execution flow for the ask command:
//  1. Spawn MCP server (mcp-remote bridge to mcp.stackoverflow.com).
//  2. Call so_search to find relevant questions.
//  3. Parse the structured JSON response.
//  4. Pick the best question and render with embedded answers.
//  5. If the best match has no embedded answers, fetch via get_content.
func runAsk(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	// ── Banner ──
	banner := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF6600")).
		Render("⚡ flo")
	fmt.Println(banner)
	fmt.Println()

	spinner := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD700")).
		Bold(true)

	// ── Step 1: Connect to the Stack Overflow MCP server ──
	// The mcp-remote npm bridge is spawned as a subprocess communicating
	// over stdin/stdout using JSON-RPC (MCP stdio transport).
	// On first run it opens a browser for OAuth; subsequent runs reuse
	// the cached token automatically.
	fmt.Println(spinner.Render("⏳ Connecting to Stack Overflow MCP server..."))
	fmt.Println(spinner.Render("  (first run may open a browser for Stack Overflow login)"))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client, err := mcp.NewClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") ||
			strings.Contains(err.Error(), "not found") {
			printError(
				"Node.js not found",
				"flo requires Node.js (npx) to launch the Stack Exchange MCP server.\n\n"+
					"Install Node.js from: https://nodejs.org\n"+
					"  macOS:   brew install node\n"+
					"  Ubuntu:  sudo apt install nodejs npm\n"+
					"  Windows: choco install nodejs",
			)
			return fmt.Errorf("npx not found")
		}
		printError("Failed to start MCP server", err.Error())
		return err
	}
	defer client.Close()

	// ── Step 2: Search ──
	// JSON-RPC request: {"jsonrpc":"2.0","id":N,"method":"tools/call",
	//   "params":{"name":"so_search","arguments":{"query":"<query>"}}}
	fmt.Println(spinner.Render(fmt.Sprintf("🔍 Searching Stack Overflow for: %q", query)))
	fmt.Println()

	searchResult, err := client.CallTool(ctx, "so_search", map[string]any{
		"query": query,
	})
	if err != nil {
		printError("Search failed", err.Error())
		return err
	}

	searchText := mcp.ExtractText(searchResult)
	if searchText == "" {
		printError("No results", "No Stack Overflow results found for your query.")
		return fmt.Errorf("no results found")
	}

	// ── Step 3: Parse the structured JSON response ──
	// The server wraps results in {"Items":[...],"Errors":[]} where each
	// Item contains Site, Type, Id, and Data (the full question payload
	// including any embedded answers from so_search).
	resp, parseErr := mcp.ParseResponse(searchText)

	// ── Step 4: Pick the best question ──
	if parseErr == nil && resp != nil && len(resp.Items) > 0 {
		// Try to detect language/tag hints from the query for better ranking.
		tagHints := detectTagHints(query)

		best := mcp.BestQuestion(resp, tagHints)
		if best != nil && len(best.Answers) > 0 {
			// The search results include embedded answers — render directly.
			md := mcp.FormatQuestionMarkdown(best, maxAnswersToShow)
			renderAndPrint(md)
			return nil
		}

		// The best question exists but has no inline answers.
		// Fetch the full content via get_content (SO_Q<id> format).
		if best != nil && best.QuestionID > 0 {
			return fetchAndRender(ctx, client, spinner, best.QuestionID)
		}
	}

	// ── Step 5: Fallback — extract an ID from URLs in the raw text ──
	questionID := mcp.ExtractQuestionID(searchText)
	if questionID != "" {
		qid := 0
		fmt.Sscanf(questionID, "%d", &qid)
		if qid > 0 {
			return fetchAndRender(ctx, client, spinner, qid)
		}
	}

	// Last resort: show a summary listing of search results.
	if resp != nil && len(resp.Items) > 0 {
		md := mcp.FormatSearchResults(resp, 10)
		renderAndPrint(md)
		return nil
	}

	// Absolutely nothing parseable — dump raw text.
	fmt.Fprint(os.Stdout, searchText)
	return nil
}

// fetchAndRender calls get_content for a specific question ID,
// parses the response, and renders it.
func fetchAndRender(ctx context.Context, client *mcp.Client, spinner lipgloss.Style, qid int) error {
	fmt.Println(spinner.Render("📖 Fetching full question..."))
	fmt.Println()

	// get_content uses the SO_Q<id> / SO_A<id> / SO_C<id> format.
	// JSON-RPC: {"jsonrpc":"2.0","id":N,"method":"tools/call",
	//   "params":{"name":"get_content","arguments":{"query":"SO_Q<id>"}}}
	contentResult, err := client.CallTool(ctx, "get_content", map[string]any{
		"query": fmt.Sprintf("SO_Q%d", qid),
	})
	if err != nil {
		printError("Failed to fetch question", err.Error())
		return err
	}

	contentText := mcp.ExtractText(contentResult)
	if contentText == "" {
		printError("Empty response", "The server returned no content for this question.")
		return fmt.Errorf("empty content")
	}

	// Parse the get_content response (same envelope format).
	resp, parseErr := mcp.ParseResponse(contentText)
	if parseErr == nil && resp != nil && len(resp.Items) > 0 {
		q := &resp.Items[0].Data
		md := mcp.FormatQuestionMarkdown(q, maxAnswersToShow)
		renderAndPrint(md)
		return nil
	}

	// If parsing fails, try rendering the raw text as markdown.
	renderAndPrint(contentText)
	return nil
}

// renderAndPrint renders markdown text through glamour + lipgloss and
// writes the result to stdout.
func renderAndPrint(md string) {
	rendered, err := ui.RenderContent(md)
	if err != nil {
		// Glamour rendering failed — print plain text.
		fmt.Fprint(os.Stdout, md)
		return
	}
	fmt.Fprint(os.Stdout, rendered)
}

// detectTagHints extracts likely programming-language tags from the
// user's query to help rank search results.
func detectTagHints(query string) []string {
	// Map of common query keywords → Stack Overflow tag names.
	langMap := map[string]string{
		"go": "go", "golang": "go",
		"python": "python", "py": "python",
		"javascript": "javascript", "js": "javascript", "node": "node.js",
		"typescript": "typescript", "ts": "typescript",
		"java": "java",
		"c++": "c++", "cpp": "c++",
		"c#": "c#", "csharp": "c#",
		"ruby": "ruby",
		"rust": "rust",
		"swift": "swift",
		"kotlin": "kotlin",
		"php": "php",
		"bash": "bash", "shell": "bash",
		"sql": "sql", "mysql": "mysql", "postgres": "postgresql",
		"react": "reactjs",
		"docker": "docker",
		"kubernetes": "kubernetes", "k8s": "kubernetes",
		"git": "git",
	}

	words := strings.Fields(strings.ToLower(query))
	seen := make(map[string]bool)
	var hints []string
	for _, w := range words {
		if tag, ok := langMap[w]; ok && !seen[tag] {
			hints = append(hints, tag)
			seen[tag] = true
		}
	}
	return hints
}
