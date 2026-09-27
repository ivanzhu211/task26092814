package negaudit

import (
	"reflect"
	"testing"
)

func TestParseEntry(t *testing.T) {
	cases := []struct {
		raw      string
		negative bool
		wantOK   bool
		wantType MatchType
		wantWord []string
	}{
		// Trimming and basic match-type detection.
		{"  hello world ", false, true, Broad, []string{"hello", "world"}},
		{"[exact match]", false, true, Exact, []string{"exact", "match"}},
		{`"phrase match"`, false, true, Phrase, []string{"phrase", "match"}},
		{"plain", false, true, Broad, []string{"plain"}},

		// Leading "-" handling for negatives only.
		{"-[a]", true, true, Exact, []string{"a"}},
		{`-"a b"`, true, true, Phrase, []string{"a", "b"}},
		{"-free", true, true, Broad, []string{"free"}},
		{" - [a] ", true, true, Exact, []string{"a"}},
		{"--a", true, true, Broad, []string{"a"}}, // one "-" stripped, rest tokenized
		{"-", true, false, Broad, nil},            // nothing left
		{"- ", true, false, Broad, nil},           // nothing left
		{"-[a]", false, false, Broad, nil},        // keyword: edge "-" then "[" ... "-" is not edge punct
		// For keywords a leading "-" is just tokenized away as whitespace.
		{"-a", false, true, Broad, []string{"a"}},

		// Malformed wrappers: bracket/quote at an edge without a full wrap.
		{"[abc", false, false, Broad, nil},
		{"abc]", false, false, Broad, nil},
		{"[a]b", false, false, Broad, nil},
		{"a[b]", false, false, Broad, nil}, // ends with "]"
		{`"abc`, false, false, Broad, nil},
		{`abc"`, false, false, Broad, nil},
		{`[a"`, false, false, Broad, nil},
		{`"a]`, false, false, Broad, nil},
		{"[", false, false, Broad, nil},
		{"]", false, false, Broad, nil},
		{`"`, false, false, Broad, nil},
		{"[]", false, false, Broad, nil},  // wraps empty content
		{`""`, false, false, Broad, nil},  // wraps empty content
		{"[ ]", false, false, Broad, nil}, // wraps only whitespace

		// Interior brackets/quotes are fine and act as whitespace.
		{"a[b", false, true, Broad, []string{"a", "b"}},
		{`a"b`, false, true, Broad, []string{"a", "b"}},
		{"[[a]]", false, true, Exact, []string{"a"}},

		// Tokenization: lowercase, '&' kept, everything else is whitespace.
		{"A&B", false, true, Broad, []string{"a&b"}},
		{"a-b", false, true, Broad, []string{"a", "b"}},
		{"Äbc Ü", false, true, Broad, []string{"äbc", "ü"}},
		{"abc123", false, true, Broad, []string{"abc123"}},
		{"a,b.c", false, true, Broad, []string{"a", "b", "c"}},
		{"!!!", false, false, Broad, nil}, // zero words after tokenizing

		// Word-count limits.
		{"a b c d e f g h i j", false, true, Broad, // exactly 10
			[]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}},
		{"a b c d e f g h i j k", false, false, Broad, nil}, // 11
		{"[a b c d e f g h i j k]", false, false, Broad, nil},

		// Empty / whitespace-only raw strings.
		{"", false, false, Broad, nil},
		{"   ", false, false, Broad, nil},
	}
	for _, tc := range cases {
		words, mt, ok := parseEntry(tc.raw, tc.negative)
		if ok != tc.wantOK {
			t.Errorf("parseEntry(%q, %v): ok=%v, want %v", tc.raw, tc.negative, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if mt != tc.wantType {
			t.Errorf("parseEntry(%q, %v): type=%v, want %v", tc.raw, tc.negative, mt, tc.wantType)
		}
		if !reflect.DeepEqual(words, tc.wantWord) {
			t.Errorf("parseEntry(%q, %v): words=%v, want %v", tc.raw, tc.negative, words, tc.wantWord)
		}
	}
}
