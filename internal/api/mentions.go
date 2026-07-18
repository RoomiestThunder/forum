package api

import (
	"regexp"
	"strings"
)

var mentionPattern = regexp.MustCompile(`@([a-zA-Z0-9_]{3,20})`)

// maxMentionsPerComment caps how many distinct users a single comment can
// ping, so mass-mentioning isn't a cheap way to spam notifications.
const maxMentionsPerComment = 5

// extractMentions returns up to maxMentionsPerComment unique usernames
// referenced via @username in text, in first-seen order. Case is preserved
// as typed; GetUserByUsername lookups are case-insensitive.
func extractMentions(text string) []string {
	matches := mentionPattern.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var out []string
	for _, m := range matches {
		key := strings.ToLower(m[1])
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, m[1])
		if len(out) >= maxMentionsPerComment {
			break
		}
	}
	return out
}
