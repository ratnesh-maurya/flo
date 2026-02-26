package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// version is set at build time via ldflags.
var version = "dev"

var (
	brandStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6600")).
			MarginBottom(1)

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF0000")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF4444")).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true)
)

var rootCmd = &cobra.Command{
	Use:     "flo",
	Short:   "Search Stack Overflow from your terminal",
	Version: version,
	// When no subcommand is given, enter interactive REPL mode.
	// runAsk is defined in ask.go (same package); with zero args it
	// starts a REPL, with args it does a one-shot search.
	RunE: runAsk,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// printError prints a styled error message to stderr.
func printError(title, body string) {
	msg := fmt.Sprintf("\u2716 %s\n\n%s", title, body)
	fmt.Fprintln(os.Stderr, errorStyle.Render(msg))
}
