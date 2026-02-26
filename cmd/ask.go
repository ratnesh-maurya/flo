package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/manifoldco/promptui"
	"github.com/ratnesh-maurya/flo/pkg/mcp"
	"github.com/ratnesh-maurya/flo/pkg/ui"
	"github.com/spf13/cobra"
)

// maxAnswersToShow limits the number of answers in the selection list.
const maxAnswersToShow = 5

var askCmd = &cobra.Command{
	Use:   `ask [query]`,
	Short: "Search Stack Overflow for a question",
	Long: `Search Stack Overflow directly from your terminal.

  One-shot:     flo ask "how to reverse a string in go"
  Interactive:  flo ask   (or just: flo)`,
	RunE: runAsk,
}

func init() {
	rootCmd.AddCommand(askCmd)
}

// ---------- styles ----------

var (
	spinnerSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Bold(true)
	successSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
	promptSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6600")).Bold(true)
	dimSty     = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)

// ---------- entry point ----------

// runAsk handles both one-shot (with args) and REPL (no args) modes.
// It connects to the official Stack Overflow MCP server once and reuses
// the connection across queries.
func runAsk(cmd *cobra.Command, args []string) error {
	// Banner
	fmt.Println(lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("#FF6600")).
		Render("⚡ flo — Stack Overflow in your terminal"))
	fmt.Println()

	// Connect to MCP server (reused across REPL iterations).
	// The mcp-remote bridge communicates over stdin/stdout JSON-RPC.
	// First run opens a browser for OAuth; subsequent runs reuse the token.
	fmt.Println(spinnerSty.Render("⏳ Connecting to Stack Overflow MCP server..."))
	fmt.Println(dimSty.Render("  (first run may open a browser for Stack Overflow login)"))

	connectCtx, connectCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer connectCancel()

	client, err := mcp.NewClient(connectCtx)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			printError("Node.js not found",
				"flo requires Node.js (npx).\n\n"+
					"  macOS:   brew install node\n"+
					"  Ubuntu:  sudo apt install nodejs npm\n"+
					"  Windows: choco install nodejs")
			return fmt.Errorf("npx not found")
		}
		printError("Connection failed", err.Error())
		return err
	}
	defer client.Close()

	fmt.Println(successSty.Render("✅ Connected!"))
	fmt.Println()

	// One-shot mode: query provided as arguments.
	if len(args) > 0 {
		query := strings.Join(args, " ")
		return searchAndDisplay(client, query)
	}

	// REPL mode: keep asking questions until the user quits.
	return replLoop(client)
}

// ---------- REPL ----------

// replLoop reads questions from stdin in a loop and displays results
// interactively.  The MCP connection is shared across iterations.
func replLoop(client *mcp.Client) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print(promptSty.Render("❓ Ask: "))

		line, err := reader.ReadString('\n')
		if err != nil {
			break // EOF or read error
		}
		query := strings.TrimSpace(line)

		if query == "" {
			continue
		}
		if query == "quit" || query == "exit" || query == "q" {
			break
		}

		_ = searchAndDisplay(client, query)
		fmt.Println()
	}

	fmt.Println(dimSty.Render("\n👋 Goodbye!"))
	return nil
}

// ---------- search + display ----------

// searchAndDisplay is the core flow:
//  1. Call so_search via MCP JSON-RPC to find relevant questions.
//  2. Parse the structured JSON response.
//  3. Pick the best question (prefer ones with embedded answers).
//  4. If no embedded answers, fetch accepted answer via get_content.
//  5. Render the question, then show interactive answer selection.
func searchAndDisplay(client *mcp.Client, query string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Println(spinnerSty.Render(fmt.Sprintf("\n🔍 Searching for: %q\n", query)))

	// MCP tool call: so_search
	// JSON-RPC: {"jsonrpc":"2.0","id":N,"method":"tools/call",
	//   "params":{"name":"so_search","arguments":{"query":"<text>"}}}
	searchResult, err := client.CallTool(ctx, "so_search", map[string]any{"query": query})
	if err != nil {
		printError("Search failed", err.Error())
		return err
	}

	searchText := mcp.ExtractText(searchResult)
	if searchText == "" {
		printError("No results", "No results found for your query.")
		return nil
	}

	resp, parseErr := mcp.ParseResponse(searchText)
	if parseErr != nil || resp == nil || len(resp.Items) == 0 {
		printError("No results", "Could not parse search results.")
		return nil
	}

	tagHints := detectTagHints(query)

	// Strategy 1: Find a question that already has embedded answers
	// (so_search sometimes includes full answer bodies in the response).
	best := mcp.BestQuestionWithAnswers(resp, tagHints)

	if best == nil {
		// Strategy 2: Find the best question by score/tags,
		// then fetch its accepted answer separately.
		best = mcp.BestQuestion(resp, tagHints)
		if best == nil {
			// Strategy 3: Show a list of search results.
			md := mcp.FormatSearchResults(resp, 10)
			renderAndPrint(md)
			return nil
		}
		// Fetch the accepted answer via get_content "SO_A<id>".
		if best.AcceptedAnswerID > 0 {
			fmt.Println(spinnerSty.Render("📖 Fetching accepted answer..."))
			fetchAcceptedAnswer(ctx, client, best)
		}
	}

	// Display question header (title, meta, tags, body).
	header := mcp.FormatQuestionHeader(best)
	renderAndPrint(header)

	// Interactive answer selection with arrow-key navigation.
	if len(best.Answers) > 0 {
		return answerSelectionLoop(best.Answers)
	}

	// No answers could be fetched.
	if best.Link != "" {
		fmt.Println(dimSty.Render(fmt.Sprintf("  View on Stack Overflow: %s\n", best.Link)))
	}
	return nil
}

// fetchAcceptedAnswer calls get_content for the accepted answer and
// appends it to the question's Answers slice.
// JSON-RPC: {"method":"tools/call","params":{"name":"get_content",
//
//	"arguments":{"query":"SO_A<id>"}}}
func fetchAcceptedAnswer(ctx context.Context, client *mcp.Client, q *mcp.QuestionData) {
	ansResult, err := client.CallTool(ctx, "get_content", map[string]any{
		"query": fmt.Sprintf("SO_A%d", q.AcceptedAnswerID),
	})
	if err != nil {
		return
	}
	ansText := mcp.ExtractText(ansResult)
	ansResp, err := mcp.ParseResponse(ansText)
	if err != nil || ansResp == nil || len(ansResp.Items) == 0 {
		return
	}
	ans := mcp.AnswerFromItem(ansResp.Items[0])
	q.Answers = append(q.Answers, ans)
}

// ---------- interactive answer selection ----------

// answerSelectionLoop shows a promptui list of answers with arrow-key
// navigation. The user selects an answer to view it, then can go back
// to pick another or exit.
func answerSelectionLoop(answers []mcp.AnswerData) error {
	sorted := mcp.SortAnswers(answers)
	if len(sorted) > maxAnswersToShow {
		sorted = sorted[:maxAnswersToShow]
	}

	for {
		// Build the selection items (one-line previews).
		items := make([]string, len(sorted))
		for i := range sorted {
			items[i] = mcp.FormatAnswerPreview(&sorted[i], i)
		}

		sel := promptui.Select{
			Label: "Select an answer (↑↓ navigate, Enter to view, Ctrl+C to go back)",
			Items: items,
			Size:  len(items),
			Templates: &promptui.SelectTemplates{
				Label:    "{{ . }}",
				Active:   "▸ {{ . | cyan }}",
				Inactive: "  {{ . }}",
				Selected: "▸ {{ . | green }}",
			},
		}

		idx, _, err := sel.Run()
		if err != nil {
			// Ctrl+C or interrupt → exit answer loop
			return nil
		}

		// Render the selected answer with glamour + lipgloss.
		md := mcp.FormatSingleAnswer(&sorted[idx])
		renderAndPrint(md)

		// Post-answer navigation.
		fmt.Println(dimSty.Render("  [Enter] back to answers  |  [n] new question  |  [q] quit"))
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "n", "q":
			return nil
		default:
			continue // back to answer list
		}
	}
}

// ---------- helpers ----------

// renderAndPrint renders markdown through glamour + lipgloss and prints.
func renderAndPrint(md string) {
	rendered, err := ui.RenderContent(md)
	if err != nil {
		fmt.Fprint(os.Stdout, md)
		return
	}
	fmt.Fprint(os.Stdout, rendered)
}

// detectTagHints extracts likely programming-language tags from the
// user's query to help rank search results.
func detectTagHints(query string) []string {
	langMap := map[string]string{
		"go": "go", "golang": "go",
		"python": "python", "py": "python",
		"javascript": "javascript", "js": "javascript", "node": "node.js",
		"typescript": "typescript", "ts": "typescript",
		"java": "java",
		"c++": "c++", "cpp": "c++",
		"c#": "c#", "csharp": "c#",
		"ruby": "ruby", "rust": "rust", "swift": "swift",
		"kotlin": "kotlin", "php": "php",
		"bash": "bash", "shell": "bash",
		"sql": "sql", "mysql": "mysql", "postgres": "postgresql",
		"react": "reactjs", "docker": "docker",
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
