package negaudit

import (
	"sort"
	"strings"
	"unicode"
)

const maxWords = 10

// parseEntry normalizes one raw keyword/negative string.
// Negative entries allow one leading "-" which is stripped before judging.
// It returns the word sequence, the match type, and whether the entry is
// valid.
func parseEntry(raw string, negative bool) (words []string, mt MatchType, ok bool) {
	s := strings.TrimSpace(raw)
	if negative {
		s = strings.TrimSpace(strings.TrimPrefix(s, "-"))
	}
	if s == "" {
		return nil, Broad, false
	}
	first, last := s[0], s[len(s)-1]
	switch {
	case len(s) >= 2 && first == '[' && last == ']':
		mt = Exact
		s = s[1 : len(s)-1]
	case len(s) >= 2 && first == '"' && last == '"':
		mt = Phrase
		s = s[1 : len(s)-1]
	default:
		mt = Broad
		// A bracket or quote at either edge that does not form a complete
		// wrapper makes the entry malformed.
		if isEdgePunct(first) || isEdgePunct(last) {
			return nil, Broad, false
		}
	}
	words = tokenize(s)
	if len(words) == 0 || len(words) > maxWords {
		return nil, Broad, false
	}
	return words, mt, true
}

func isEdgePunct(b byte) bool {
	return b == '[' || b == ']' || b == '"'
}

// tokenize lowercases (Unicode aware), treats every rune that is not a
// letter, digit or '&' as whitespace, and splits on whitespace.
func tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '&'
	})
}

// wordSet returns the sorted distinct words of ws.
func wordSet(ws []string) []string {
	set := make([]string, 0, len(ws))
	seen := make(map[string]struct{}, len(ws))
	for _, w := range ws {
		if _, dup := seen[w]; !dup {
			seen[w] = struct{}{}
			set = append(set, w)
		}
	}
	sort.Strings(set)
	return set
}
