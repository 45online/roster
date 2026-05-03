package slackbridge

import (
	"fmt"
	"strconv"
	"strings"
)

// Command is the parsed result of a single slash-command invocation.
type Command struct {
	Verb      string // "sync-issue" | "review-pr" | "archive-issue" | "status" | "help"
	Repo      string // "owner/name" — empty for "status" / "help"
	Number    int    // issue or PR number — 0 when not applicable
	UserID    string
	UserName  string
	ChannelID string
	Raw       string // original "text" field for diagnostics
}

// Parse takes the "text" field of a Slack slash command (everything after
// the slash) and produces a structured Command. The supported grammar:
//
//	help
//	status
//	sync-issue    <owner/name>#<n>
//	review-pr     <owner/name>#<n>
//	archive-issue <owner/name>#<n>
//
// Both "<repo>#<n>" and "<repo> <n>" forms work (Slack autocompletes vary).
func Parse(text, userID, userName, channelID string) (*Command, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return &Command{Verb: "help", UserID: userID, UserName: userName, ChannelID: channelID, Raw: text}, nil
	}
	fields := strings.Fields(text)
	cmd := &Command{
		Verb:      strings.ToLower(fields[0]),
		UserID:    userID,
		UserName:  userName,
		ChannelID: channelID,
		Raw:       text,
	}

	switch cmd.Verb {
	case "help", "status":
		return cmd, nil
	case "sync-issue", "review-pr", "archive-issue":
		// expect "owner/name#42" or "owner/name 42"
		if len(fields) < 2 {
			return nil, fmt.Errorf("missing target (try `%s owner/name#42`)", cmd.Verb)
		}
		repo, num, err := parseTarget(strings.Join(fields[1:], " "))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", cmd.Verb, err)
		}
		cmd.Repo = repo
		cmd.Number = num
		return cmd, nil
	default:
		return nil, fmt.Errorf("unknown command %q (try `help`)", cmd.Verb)
	}
}

// parseTarget normalises "owner/name#42" / "owner/name 42" / "owner/name #42"
// into (repo="owner/name", number=42).
func parseTarget(s string) (string, int, error) {
	s = strings.TrimSpace(s)
	// Replace "#" with space so we can rely on Fields.
	s = strings.ReplaceAll(s, "#", " ")
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return "", 0, fmt.Errorf("expected `owner/name#NUM`, got %q", s)
	}
	repo := parts[0]
	if !strings.Contains(repo, "/") {
		return "", 0, fmt.Errorf("repo must be `owner/name`, got %q", repo)
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("number %q is not an integer", parts[1])
	}
	if n <= 0 {
		return "", 0, fmt.Errorf("number must be > 0")
	}
	return repo, n, nil
}

// HelpText is what /roster help returns.
const HelpText = "*Roster slash commands*\n" +
	"• `/roster status` — show current Roster state\n" +
	"• `/roster sync-issue owner/name#42` — Module A: file the issue in Jira\n" +
	"• `/roster review-pr owner/name#42` — Module B: AI review on the PR\n" +
	"• `/roster archive-issue owner/name#42` — Module C: archive a closed issue to Confluence\n" +
	"• `/roster help` — this message"
