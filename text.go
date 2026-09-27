package negaudit

import (
	"errors"
	"strings"
	"unicode"
)

// MatchType is the match syntax of a keyword or negative entry.
type MatchType int

const (
	Broad MatchType = iota
	Phrase
	Exact
)

// maxWords is the maximum number of words allowed in a keyword or negative.
const maxWords = 10

// entry is a parsed keyword or negative: match type plus normalized words.
type entry struct {
	match MatchType
	words []string
}

var (
	errUnbalancedWrapper = errors.New("unbalanced or misplaced [ ] \" wrapper")
	errNoWords           = errors.New("no words after normalization")
	errTooManyWords      = errors.New("more than 10 words after normalization")
)

// parseEntry parses a raw keyword/negative string. If negative is true, one
// leading '-' is allowed and stripped before further analysis.
func parseEntry(raw string, negative bool) (entry, error) {
	s := strings.TrimSpace(raw)
	if negative {
		s = strings.TrimSpace(strings.TrimPrefix(s, "-"))
	}

	match := Broad
	if len(s) > 0 {
		first := s[0]
		last := s[len(s)-1]
		isBracket := func(c byte) bool { return c == '[' || c == ']' || c == '"' }
		switch {
		case first == '[' && last == ']' && len(s) >= 2:
			match = Exact
			s = s[1 : len(s)-1]
		case first == '"' && last == '"' && len(s) >= 2:
			match = Phrase
			s = s[1 : len(s)-1]
		case isBracket(first) || isBracket(last):
			return entry{}, errUnbalancedWrapper
		}
	}

	words := splitWords(s)
	if len(words) == 0 {
		return entry{}, errNoWords
	}
	if len(words) > maxWords {
		return entry{}, errTooManyWords
	}
	return entry{match: match, words: words}, nil
}

// splitWords lowercases s (Unicode aware), treats every rune that is not a
// letter, digit or '&' as whitespace, and splits on whitespace.
func splitWords(s string) []string {
	mapped := strings.Map(func(r rune) rune {
		r = unicode.ToLower(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '&' {
			return r
		}
		return ' '
	}, s)
	return strings.Fields(mapped)
}
