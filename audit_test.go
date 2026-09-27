package negaudit

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"
)

// refMatch reports whether a pattern with the given match type matches (or,
// for a negative, blocks) a search term, directly from the definitions:
// exact -> word sequences equal; phrase -> pattern occurs contiguously and
// in order; broad -> term contains every distinct pattern word.
func refMatch(mt MatchType, pat, term []string) bool {
	switch mt {
	case Exact:
		return slices.Equal(pat, term)
	case Phrase:
		if len(pat) > len(term) {
			return false
		}
		for i := 0; i+len(pat) <= len(term); i++ {
			if slices.Equal(pat, term[i:i+len(pat)]) {
				return true
			}
		}
		return false
	default: // Broad
		for _, w := range wordSet(pat) {
			if !slices.Contains(term, w) {
				return false
			}
		}
		return true
	}
}

// refLevel computes the conflict level of (keyword, negative) by exhaustive
// enumeration of a finite set of search terms.
//
// Why this finite set is sufficient: a counterexample to "critical" is a
// term matched by the keyword but not blocked by the negative. Whenever one
// exists, one also exists with length <= len(k)+1 over the alphabet
// words(k) ∪ {fresh}, where fresh is any word absent from the negative:
//
//   - K exact: the only matched term is k itself.
//   - K phrase: if N does not block k, then k is the witness. If N blocks k,
//     then for phrase/broad N the blocking extends to every term containing
//     k contiguously (subsequence/subset containment is hereditary), and for
//     exact N the term k++[fresh] (length len(k)+1) is matched but not
//     blocked.
//   - K broad: if some negative word is missing from set(k), then k is the
//     witness. Otherwise broad N blocks everything; phrase N with |w| == 1
//     blocks everything; phrase N with |w| >= 2 is avoided by some
//     permutation of the distinct words of k (length <= len(k)); exact N
//     (w == k) is avoided by a permutation of length <= len(k) when k has
//     two positions to swap or a repeated word, and otherwise by
//     k++[fresh] or [k[0], k[0]] (length <= len(k)+1).
//
// So enumerating all terms of length 1..len(k)+1 over
// words(k) ∪ words(n) ∪ {fresh} finds a counterexample iff one exists, and
// "warning" only depends on the single term k, which is always enumerated.
func refLevel(kw, neg entry) Level {
	vocab := append([]string{}, kw.words...)
	vocab = append(vocab, neg.words...)
	vocab = append(vocab, "zzfresh")
	vocab = wordSet(vocab)

	blocksK := refMatch(neg.mt, neg.words, kw.words)
	critical := true
	var term []string
	var visit func(depth, maxDepth int) bool
	visit = func(depth, maxDepth int) bool {
		if depth > 0 {
			if refMatch(kw.mt, kw.words, term) && !refMatch(neg.mt, neg.words, term) {
				critical = false
				return true
			}
		}
		if depth == maxDepth {
			return false
		}
		for _, w := range vocab {
			term = append(term, w)
			stop := visit(depth+1, maxDepth)
			term = term[:len(term)-1]
			if stop {
				return true
			}
		}
		return false
	}
	visit(0, len(kw.words)+1)

	switch {
	case critical:
		return Critical
	case blocksK:
		return Warning
	default:
		return -1 // no conflict
	}
}

type entry struct {
	words []string
	mt    MatchType
}

func (e entry) raw(negative bool) string {
	var s string
	switch e.mt {
	case Exact:
		s = "[" + strings.Join(e.words, " ") + "]"
	case Phrase:
		s = `"` + strings.Join(e.words, " ") + `"`
	default:
		s = strings.Join(e.words, " ")
	}
	if negative {
		s = "-" + s
	}
	return s
}

// buildAccount places kw in one ad group and neg at the given placement:
// 0 = ad group negatives, 1 = campaign negatives, 2 = mounted shared list,
// 3 = unmounted shared list.
func buildAccount(kwRaw, negRaw string, placement int) *Account {
	lst := &SharedList{Name: "mounted"}
	other := &SharedList{Name: "other"}
	c := Campaign{Name: "c", Lists: []*SharedList{lst}}
	g := AdGroup{Name: "g"}
	kw, ok := parseKeyword(kwRaw)
	if !ok {
		panic("bad keyword in test")
	}
	g.Keywords = []Keyword{kw}
	n, ok := parseNegative(negRaw)
	if !ok {
		panic("bad negative in test")
	}
	switch placement {
	case 0:
		g.Negatives = []Negative{n}
	case 1:
		c.Negatives = []Negative{n}
	case 2:
		lst.Negatives = []Negative{n}
	case 3:
		other.Negatives = []Negative{n}
	}
	c.AdGroups = []AdGroup{g}
	return &Account{
		Campaigns:   []Campaign{c},
		SharedLists: map[string]*SharedList{"mounted": lst, "other": other},
	}
}

func TestAuditDifferential(t *testing.T) {
	rng := rand.New(rand.NewSource(20260928))
	vocab := []string{"a", "b", "c"} // small vocab forces repeated words
	randEntry := func() entry {
		n := 1 + rng.Intn(3)
		w := make([]string, n)
		for i := range w {
			w[i] = vocab[rng.Intn(len(vocab))]
		}
		return entry{words: w, mt: MatchType(rng.Intn(3))}
	}
	const trials = 4000
	for trial := 0; trial < trials; trial++ {
		kw := randEntry()
		neg := randEntry()
		placement := rng.Intn(4)
		want := refLevel(kw, neg)

		acc := buildAccount(kw.raw(false), neg.raw(true), placement)
		recs := Audit(acc)

		if placement == 3 {
			if len(recs) != 0 {
				t.Fatalf("trial %d: unmounted list produced %d records", trial, len(recs))
			}
			continue
		}
		if want == -1 {
			if len(recs) != 0 {
				t.Fatalf("trial %d: kw=%v neg=%v: got %v, want no record",
					trial, kw, neg, recs[0].Level)
			}
			continue
		}
		if len(recs) != 1 {
			t.Fatalf("trial %d: kw=%v neg=%v: got %d records, want 1 (%v)",
				trial, kw, neg, len(recs), want)
			continue
		}
		if recs[0].Level != want {
			t.Fatalf("trial %d: kw=%v(%v) neg=%v(%v): got %v, want %v",
				trial, kw.words, kw.mt, neg.words, neg.mt, recs[0].Level, want)
		}
		if recs[0].KeywordRaw != kw.raw(false) || recs[0].NegativeRaw != neg.raw(true) {
			t.Fatalf("trial %d: record raws = %q / %q", trial, recs[0].KeywordRaw, recs[0].NegativeRaw)
		}
	}
}

func TestAuditDeterministic(t *testing.T) {
	acc := buildAccount("buy shoes online", "-shoes", 2)
	first := Audit(acc)
	for i := 0; i < 5; i++ {
		got := Audit(acc)
		if !slices.Equal(first, got) {
			t.Fatalf("run %d differs from first", i)
		}
	}
}

func TestAuditTable(t *testing.T) {
	cases := []struct {
		kw, neg string
		want    Level // -1 means no record
	}{
		{"[a b]", "-[a b]", Critical},
		{"[a b]", `-"a"`, Critical},
		{"[a b]", "-b", Critical},
		{"[a b]", "-c", -1},
		{`"a b"`, "-[a b]", Warning}, // exact neg never critical vs phrase kw
		{`"a b"`, `-"a b"`, Critical},
		{`"a b"`, `-"b"`, Critical},
		{`"a b"`, "-a", Critical},
		{"a b", "-[a b]", Warning},
		{"a b a", "-[a b a]", Warning}, // witness "a b" avoids exact w
		{"a b", `-"a b"`, Warning},     // witness "b a" avoids the phrase
		{"a", `-"a"`, Critical},        // single-word phrase blocks every superset
		{"a b", `-"a"`, Critical},
		{"a b", "-a", Critical},
		{"a b", "-b a", Critical}, // broad negative, order irrelevant
		{"a b", "-a c", -1},
		{"a", "-[a]", Warning}, // witness "a a" avoids exact w
	}
	for i, tc := range cases {
		acc := buildAccount(tc.kw, tc.neg, 0)
		recs := Audit(acc)
		if tc.want == -1 {
			if len(recs) != 0 {
				t.Errorf("case %d (%q vs %q): got %d records, want none", i, tc.kw, tc.neg, len(recs))
			}
			continue
		}
		if len(recs) != 1 || recs[0].Level != tc.want {
			lvl := Level(-1)
			if len(recs) == 1 {
				lvl = recs[0].Level
			}
			t.Errorf("case %d (%q vs %q): got %d records level %v, want %v",
				i, tc.kw, tc.neg, len(recs), lvl, tc.want)
		}
	}
}

func TestAuditSourceLabelsAndOrder(t *testing.T) {
	lst := &SharedList{Name: "L", Negatives: []Negative{
		mustNeg(t, "-x"),
	}}
	c := Campaign{
		Name:      "C",
		Negatives: []Negative{mustNeg(t, "-y")},
		Lists:     []*SharedList{lst},
	}
	g := AdGroup{
		Name:      "G",
		Keywords:  []Keyword{mustKw(t, "x y z")},
		Negatives: []Negative{mustNeg(t, "-z")},
	}
	c.AdGroups = []AdGroup{g}
	acc := &Account{Campaigns: []Campaign{c}, SharedLists: map[string]*SharedList{"L": lst}}

	recs := Audit(acc)
	if len(recs) != 3 {
		t.Fatalf("got %d records, want 3", len(recs))
	}
	wantSrc := []string{`ad group "G"`, `campaign "C"`, `shared list "L"`}
	wantNeg := []string{"-z", "-y", "-x"}
	for i := range recs {
		if recs[i].Source != wantSrc[i] || recs[i].NegativeRaw != wantNeg[i] {
			t.Errorf("record %d: got (%q, %q), want (%q, %q)",
				i, recs[i].Source, recs[i].NegativeRaw, wantSrc[i], wantNeg[i])
		}
		if recs[i].Level != Critical {
			t.Errorf("record %d: got level %v, want critical", i, recs[i].Level)
		}
	}
}

func mustKw(t *testing.T, raw string) Keyword {
	t.Helper()
	kw, ok := parseKeyword(raw)
	if !ok {
		t.Fatalf("bad keyword %q", raw)
	}
	return kw
}

func mustNeg(t *testing.T, raw string) Negative {
	t.Helper()
	n, ok := parseNegative(raw)
	if !ok {
		t.Fatalf("bad negative %q", raw)
	}
	return n
}

func ExampleConflictRecord() {
	acc := buildAccount("buy shoes", "-shoes", 1)
	for _, r := range Audit(acc) {
		fmt.Println(r.Level, r.KeywordRaw, r.NegativeRaw, r.Source)
	}
	// Output: critical buy shoes -shoes campaign "c"
}
