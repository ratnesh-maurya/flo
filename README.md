# flo

Search Stack Overflow from your terminal using the [MCP protocol](https://modelcontextprotocol.io/).

**flo** connects to the official Stack Overflow MCP server, searches for questions, and lets you browse answers interactively — all without leaving your terminal.

## Features

- **Interactive REPL** — type questions, get answers in a loop
- **Arrow-key navigation** — browse multiple answers with ↑↓ keys
- **Beautiful rendering** — syntax-highlighted code, styled output via [glamour](https://github.com/charmbracelet/glamour) + [lipgloss](https://github.com/charmbracelet/lipgloss)
- **One-shot mode** — `flo ask "your question"` for quick lookups
- **Cross-platform** — Linux, macOS, Windows (amd64 & arm64)

## Install

### Homebrew (macOS/Linux)

```bash
brew install ratnesh-maurya/tap/flo
```

### Go

```bash
go install github.com/ratnesh-maurya/flo@latest
```

### From releases

Download the latest binary from [GitHub Releases](https://github.com/ratnesh-maurya/flo/releases).

## Prerequisites

- **Node.js** (for the `npx` command) — flo uses [`mcp-remote`](https://www.npmjs.com/package/mcp-remote) to connect to Stack Overflow's MCP server.

```bash
# macOS
brew install node

# Ubuntu/Debian
sudo apt install nodejs npm
```

## Usage

### Interactive mode (REPL)

```bash
flo
```

Type your questions, browse answers with arrow keys, and keep going:

```
⚡ flo — Stack Overflow in your terminal

❓ Ask: how to reverse a string in go

  # How to reverse a string in Go?
  Score: 171  |  Views: 178,411  |  Answers: 39  |  ✅ Answered

Select an answer (↑↓ navigate, Enter to view, Ctrl+C to go back)
  ▸ #1 by Jonathan Wright — A version which I think works on unicode...
    #2 by user181548 — [Russ Cox, on the golang-nuts mailing list]...
    #3 by Oliver Mason — **NOTE:** This answer is from 2009...
```

### One-shot mode

```bash
flo ask "how to center a div in css"
```

### Commands

| Command | Description |
|---------|-------------|
| `flo` | Start interactive REPL |
| `flo ask "<query>"` | One-shot search |
| `flo --help` | Show help |
| `flo --version` | Show version |

### REPL controls

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate answers |
| `Enter` | View selected answer |
| `Ctrl+C` | Back to answer list |
| `n` | Ask a new question |
| `q` / `quit` / `exit` | Exit flo |

## How it works

1. **flo** spawns [`mcp-remote`](https://www.npmjs.com/package/mcp-remote) as a subprocess
2. Connects to the official Stack Overflow MCP server at `mcp.stackoverflow.com`
3. Sends search queries via JSON-RPC (`so_search` tool)
4. Parses the structured response and renders it with terminal styling
5. On first run, opens a browser for Stack Overflow OAuth (token is cached)

## Development

```bash
git clone https://github.com/ratnesh-maurya/flo.git
cd flo
go build -o flo .
./flo
```

## Release

Releases are automated via GitHub Actions + [GoReleaser](https://goreleaser.com/):

```bash
git tag v1.0.0
git push origin v1.0.0
```

This triggers the release workflow which builds cross-platform binaries and creates a GitHub Release.

## License

MIT
