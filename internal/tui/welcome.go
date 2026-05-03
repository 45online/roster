package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/45online/roster/internal/version"
)

// appVersion comes from internal/version, which is the single ldflags
// target for the binary's release version.
var appVersion = version.Version

// WelcomeHeader contains data for the startup welcome banner.
type WelcomeHeader struct {
	version  string
	model    string
	cwd      string
	shown    bool // whether the header has been shown (only show once)
}

// NewWelcomeHeader creates a new welcome header with default values.
func NewWelcomeHeader(model, cwd string) WelcomeHeader {
	return WelcomeHeader{
		version: appVersion,
		model:   model,
		cwd:     cwd,
		shown:   false,
	}
}

// MarkShown marks the header as shown.
func (w WelcomeHeader) MarkShown() WelcomeHeader {
	w.shown = true
	return w
}

// IsShown returns whether the header has been shown.
func (w WelcomeHeader) IsShown() bool {
	return w.shown
}

// View renders the welcome header banner.
// Format:
//
//	┌──[ Roster vX.Y.Z ]──
//	│  <model-or-config-hint>
//	│  ~/path/to/cwd
//	│  Welcome to Roster!  /effort to tune speed vs. intelligence
//	└──
func (w WelcomeHeader) View(width int, theme Theme) string {
	if w.shown {
		return ""
	}

	var sb strings.Builder

	// ASCII art logo (employee with ID badge — see renderLogo).
	logo := renderLogo(theme)

	// Version line.
	versionLine := primaryStyle(theme).Bold(true).Render(fmt.Sprintf("Roster v%s", w.version))

	// Model line. Falls back to a config hint when no model is configured —
	// must NOT leak a default vendor/model name (undercover invariant).
	modelStr := w.model
	if modelStr == "" {
		modelStr = "(no model — set llm.model in .roster/config.yml)"
	}
	modelLine := secondaryStyle(theme).Render(modelStr)

	// Working directory.
	cwdLine := mutedStyle(theme).Render(shortenPath(w.cwd))

	// Welcome message.
	welcomeLine := lipgloss.JoinHorizontal(
		lipgloss.Left,
		successStyle(theme).Render("Welcome to Roster!"),
		mutedStyle(theme).Render("  "),
		accentStyle(theme).Render("/effort"),
		mutedStyle(theme).Render(" to tune speed vs. intelligence"),
	)

	// Combine info lines vertically
	infoBlock := lipgloss.JoinVertical(
		lipgloss.Left,
		versionLine,
		modelLine,
		cwdLine,
		welcomeLine,
	)

	// Join logo and info horizontally with center alignment.
	banner := lipgloss.JoinHorizontal(
		lipgloss.Center,
		logo,
		"  ",
		infoBlock,
	)

	// Wrap in a rounded card with the Roster brand colour on the border.
	// Padding gives the content a little breathing room from the edge.
	boxed := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(rosterBrandColor).
		Padding(0, 2).
		Render(banner)

	sb.WriteString(boxed)
	sb.WriteString("\n")

	return sb.String()
}

// rosterBrandColor is the single source for the indigo accent used by
// the logo and the welcome card border.
var rosterBrandColor = lipgloss.Color("#6366F1")

// renderLogo renders the Roster ASCII art logo: a tiny employee with an
// ID badge — visualising "AI as a virtual coworker holding a roster slot".
func renderLogo(theme Theme) string {
	logoLines := []string{
		"  (•‿•) ",
		" ┌─────┐",
		" │ID#01│",
		" └─────┘",
	}

	logoStyle := lipgloss.NewStyle().
		Foreground(rosterBrandColor).
		Bold(true)

	var rendered []string
	for _, line := range logoLines {
		rendered = append(rendered, logoStyle.Render(line))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rendered...)
}

// shortenPath shortens a path for display, replacing home dir with ~.
func shortenPath(path string) string {
	// This is a simple implementation - could be enhanced to use os.UserHomeDir()
	if strings.HasPrefix(path, "/Users/") {
		parts := strings.SplitN(path, "/", 4)
		if len(parts) >= 4 {
			return "~/" + parts[3]
		}
	}
	if strings.HasPrefix(path, "/home/") {
		parts := strings.SplitN(path, "/", 4)
		if len(parts) >= 4 {
			return "~/" + parts[3]
		}
	}
	return path
}
