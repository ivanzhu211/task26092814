package negaudit

import (
	"reflect"
	"testing"
)

func TestParseEntry(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		negative bool
		want     entry
		wantErr  bool
	}{
		{name: "broad plain", raw: "hello world", want: entry{Broad, []string{"hello", "world"}}},
		{name: "trim outer space", raw: "  hello\t", want: entry{Broad, []string{"hello"}}},
		{name: "exact", raw: "[hello world]", want: entry{Exact, []string{"hello", "world"}}},
		{name: "phrase", raw: `"hello world"`, want: entry{Phrase, []string{"hello", "world"}}},
		{name: "exact with inner space", raw: "[  hello  ]", want: entry{Exact, []string{"hello"}}},
		{name: "negative dash", raw: "-hello", negative: true, want: entry{Broad, []string{"hello"}}},
		{name: "negative dash exact", raw: "-[hello]", negative: true, want: entry{Exact, []string{"hello"}}},
		{name: "negative dash phrase spaced", raw: `- "hello" `, negative: true, want: entry{Phrase, []string{"hello"}}},
		{name: "keyword keeps dash as separator", raw: "-hello", want: entry{Broad, []string{"hello"}}},
		{name: "unicode lower", raw: "ÄBC Def", want: entry{Broad, []string{"äbc", "def"}}},
		{name: "punct becomes space", raw: "hello,world!yes", want: entry{Broad, []string{"hello", "world", "yes"}}},
		{name: "ampersand kept", raw: "at&t", want: entry{Broad, []string{"at&t"}}},
		{name: "digits kept", raw: "mp3 4k", want: entry{Broad, []string{"mp3", "4k"}}},
		{name: "inner bracket is separator", raw: "a[bc", want: entry{Broad, []string{"a", "bc"}}},

		{name: "err empty", raw: "", wantErr: true},
		{name: "err spaces only", raw: "   ", wantErr: true},
		{name: "err dash only", raw: "-", negative: true, wantErr: true},
		{name: "err no word chars", raw: "!!!", wantErr: true},
		{name: "err empty exact", raw: "[]", wantErr: true},
		{name: "err empty phrase", raw: `""`, wantErr: true},
		{name: "err punct only exact", raw: "[!?]", wantErr: true},

		{name: "err open bracket only", raw: "[abc", wantErr: true},
		{name: "err close bracket only", raw: "abc]", wantErr: true},
		{name: "err open quote only", raw: `"abc`, wantErr: true},
		{name: "err close quote only", raw: `abc"`, wantErr: true},
		{name: "err mismatched wrappers", raw: `[abc"`, wantErr: true},
		{name: "err trailing after exact", raw: "[a]b", wantErr: true},
		{name: "err leading before exact", raw: "b[a]", wantErr: true},
		{name: "err negative unbalanced", raw: "-[abc", negative: true, wantErr: true},

		{name: "ten words ok", raw: "a b c d e f g h i j", want: entry{Broad, []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}}},
		{name: "eleven words err", raw: "a b c d e f g h i j k", wantErr: true},
		{name: "duplicate words count", raw: "a a a a a a a a a a a", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEntry(tc.raw, tc.negative)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseEntry(%q, %v) = %+v, want error", tc.raw, tc.negative, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseEntry(%q, %v) error: %v", tc.raw, tc.negative, err)
			}
			if got.match != tc.want.match || !reflect.DeepEqual(got.words, tc.want.words) {
				t.Fatalf("parseEntry(%q, %v) = %+v, want %+v", tc.raw, tc.negative, got, tc.want)
			}
		})
	}
}
