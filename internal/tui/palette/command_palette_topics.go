package palette

import "strings"

// A lone topic named by an inspection command is ambiguous: "queue" should
// open the queue before offering to clear it. Verb phrases retain fuzzy ranking.
func isTopicQuery(commands []PaletteCommand, query string) bool {
	words := strings.Fields(query)
	if len(words) != 1 {
		return false
	}
	q := strings.ToLower(words[0])
	for _, command := range commands {
		if command.Intent != IntentInspect {
			continue
		}
		name := strings.ToLower(command.Name)
		for _, prefix := range []string{"go to ", "show ", "jump to "} {
			name = strings.TrimPrefix(name, prefix)
		}
		for _, word := range strings.Fields(name) {
			word = strings.Trim(word, ",:…")
			if topicWordMatches(word, q) {
				return true
			}
		}
	}
	return false
}

func topicPriority(command PaletteCommand, query string) int {
	if strings.EqualFold(command.Name, query) {
		return 7
	}
	for _, alias := range command.Aliases {
		if strings.EqualFold(alias, query) {
			return 6
		}
	}
	if !command.IsEnabled() {
		return 0
	}
	switch command.Intent {
	case IntentInspect:
		return 5
	case IntentListening:
		return 4
	case IntentRemoval:
		return 1
	default:
		return 2
	}
}

func topicWordMatches(word, query string) bool {
	switch word {
	case "in", "the", "to", "or", "and", "next", "previous":
		return false
	}
	return strings.TrimSuffix(word, "s") == strings.TrimSuffix(query, "s")
}
