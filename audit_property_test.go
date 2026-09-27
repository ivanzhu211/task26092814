package negaudit

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// This file compares Audit against a brute-force reference that applies the
// matching definitions literally over a finite set of search terms.
//
// Why the finite set is enough to expose every counterexample:
//
// For a fixed pair (K, N), whether a search term S is matched by K and
// blocked by N depends only on four relations: S == seq(K), S == seq(N),
// seq(K)/seq(N) occurring as contiguous subsequences of S, and containment
// of set(K)/set(N) in set(S). Let V = words(K) ∪ words(N) ∪ {⊕}, where ⊕ is
// a fresh token outside words(K) ∪ words(N). Replacing every word of S that
// is not in words(K) ∪ words(N) by ⊕ preserves all four relations: the
// words of K and N are untouched, and ⊕ is a single token distinct from all
// of them, so sequence equalities and contiguous occurrences involving
// seq(K)/seq(N) (which contain no ⊕) are preserved in both directions, and
// word-set containment is preserved. Hence a counterexample over the
// infinite vocabulary exists iff one exists over V.
//
// Length: any K-matching, non-blocked term can be shortened to one of these
// canonical forms, all over V:
//   - seq(K) itself (length len(K)): always matches K;
//   - seq(K) followed by ⊕ (length len(K)+1): still matches K (phrase/broad)
//     and differs from any seq(N);
//   - the distinct words of K interleaved with ⊕, w1 ⊕ w2 ⊕ ... ⊕ wm
//     (length 2m-1 ≤ 2·len(K)-1): matches a broad K, and any contiguous
//     window of length ≥ 2 contains ⊕, so no multi-word phrase negative can
//     occur in it;
//   - that sequence plus one trailing ⊕ (length ≤ 2·len(K)) for the case
//     where the interleaved sequence itself equals seq(N) of an exact
//     negative.
// So restricting to lengths 1..2·len(K)+1 loses no counterexample, and the
// reference below enumerates exactly that finite set.

const freshToken = "⊕"

func refMatch(match MatchType, pat []string, s []string) bool {
	switch match {
	case Exact:
		if len(pat) != len(s) {
			return false
		}
		for i := range pat {
			if pat[i] != s[i] {
				return false
			}
		}
		return true
	case Phrase:
		if len(pat) > len(s) {
			return false
		}
		for start := 0; start+len(pat) <= len(s); start++ {
			ok := true
			for i := range pat {
				if pat[i] != s[start+i] {
					ok = false
					break
				}
			}
			if ok {
				return true
			}
		}
		return false
	case Broad:
		have := make(map[string]bool, len(s))
		for _, w := range s {
			have[w] = true
		}
		for _, w := range pat {
			if !have[w] {
				return false
			}
		}
		return true
	}
	return false
}

// refLevel computes the conflict level of the pair by exhaustive enumeration
// over the finite search-term set described above.
func refLevel(kw, neg entry) Level {
	vocabSet := map[string]bool{freshToken: true}
	for _, w := range kw.words {
		vocabSet[w] = true
	}
	for _, w := range neg.words {
		vocabSet[w] = true
	}
	vocab := make([]string, 0, len(vocabSet))
	for w := range vocabSet {
		vocab = append(vocab, w)
	}

	critical := true
	maxLen := 2*len(kw.words) + 1
	var visit func(s []string)
	visit = func(s []string) {
		if len(s) > 0 {
			if refMatch(kw.match, kw.words, s) && !refMatch(neg.match, neg.words, s) {
				critical = false
			}
		}
		if len(s) == maxLen || !critical {
			return
		}
		for _, w := range vocab {
			visit(append(s, w))
		}
	}
	visit(nil)

	if critical {
		return Critical
	}
	if refMatch(neg.match, neg.words, kw.words) {
		return Warning
	}
	return ""
}

func rawOf(e entry) string {
	s := strings.Join(e.words, " ")
	switch e.match {
	case Exact:
		return "[" + s + "]"
	case Phrase:
		return `"` + s + `"`
	}
	return s
}

func TestAuditMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(20260928))
	wordList := []string{"a", "b", "c"}

	randEntry := func() entry {
		words := make([]string, 1+rng.Intn(3))
		for i := range words {
			words[i] = wordList[rng.Intn(len(wordList))]
		}
		return entry{match: MatchType(rng.Intn(3)), words: words}
	}

	// Generate unique keyword and negative raws covering all nine
	// match-type combinations.
	var kws, negs []entry
	seenKw, seenNeg := map[string]bool{}, map[string]bool{}
	combos := map[[2]MatchType]bool{}
	for len(kws) < 60 || len(combos) < 9 {
		k := randEntry()
		if r := rawOf(k); !seenKw[r] {
			seenKw[r] = true
			kws = append(kws, k)
		}
		n := randEntry()
		if r := rawOf(n); !seenNeg[r] {
			seenNeg[r] = true
			negs = append(negs, n)
		}
		combos[[2]MatchType{k.match, n.match}] = true
	}

	// Build the account through the public JSON loader: one campaign, one
	// ad group, every negative attached at campaign level, so that every
	// (keyword, negative) pair is applicable.
	type wireAccount struct {
		Campaigns []struct {
			Name      string   `json:"name"`
			Negatives []string `json:"negatives"`
			AdGroups  []struct {
				Name     string   `json:"name"`
				Keywords []string `json:"keywords"`
			} `json:"ad_groups"`
		} `json:"campaigns"`
	}
	var wa wireAccount
	wa.Campaigns = append(wa.Campaigns, struct {
		Name      string   `json:"name"`
		Negatives []string `json:"negatives"`
		AdGroups  []struct {
			Name     string   `json:"name"`
			Keywords []string `json:"keywords"`
		} `json:"ad_groups"`
	}{Name: "c0"})
	for _, n := range negs {
		wa.Campaigns[0].Negatives = append(wa.Campaigns[0].Negatives, rawOf(n))
	}
	wa.Campaigns[0].AdGroups = append(wa.Campaigns[0].AdGroups, struct {
		Name     string   `json:"name"`
		Keywords []string `json:"keywords"`
	}{Name: "g0"})
	for _, k := range kws {
		wa.Campaigns[0].AdGroups[0].Keywords = append(wa.Campaigns[0].AdGroups[0].Keywords, rawOf(k))
	}
	data, err := json.Marshal(wa)
	if err != nil {
		t.Fatal(err)
	}
	acc, invalid, err := LoadAccount(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(invalid) != 0 {
		t.Fatalf("unexpected invalid entries: %+v", invalid)
	}

	negIdxByRaw := map[string]int{}
	for i, n := range negs {
		negIdxByRaw[rawOf(n)] = i
	}
	got := map[[2]int]Level{}
	for _, c := range Audit(acc) {
		ni, ok := negIdxByRaw[c.Negative.Raw]
		if !ok {
			t.Fatalf("record with unknown negative %q", c.Negative.Raw)
		}
		key := [2]int{c.Keyword.KeywordIdx, ni}
		if _, dup := got[key]; dup {
			t.Fatalf("duplicate record for pair %v", key)
		}
		got[key] = c.Level
	}

	for ki, k := range kws {
		for ni, n := range negs {
			want := refLevel(k, n)
			g := got[[2]int{ki, ni}]
			if want == "" {
				if g != "" {
					t.Errorf("pair kw=%v neg=%v: got %s, want no record", k, n, g)
				}
				continue
			}
			if g != want {
				t.Errorf("pair kw=%v neg=%v: got %q, want %q", k, n, g, want)
			}
		}
	}
}

// TestAuditDeterministic checks that repeated calls produce identical output.
func TestAuditDeterministic(t *testing.T) {
	jsonAcct := `{
	  "campaigns": [
	    {"name": "c1", "negatives": ["[a b]", "-c"], "shared_lists": ["L"],
	     "ad_groups": [
	       {"name": "g1", "keywords": ["a b", "[a b c]", "x"], "negatives": ["\"a b\""]},
	       {"name": "g2", "keywords": ["a", "b a"], "negatives": []}
	     ]}
	  ],
	  "shared_lists": {"L": ["a", "[x]", "b a c"]}
	}`
	acc, _, err := LoadAccount(strings.NewReader(jsonAcct))
	if err != nil {
		t.Fatal(err)
	}
	first := Audit(acc)
	second := Audit(acc)
	if fmt.Sprintf("%+v", first) != fmt.Sprintf("%+v", second) {
		t.Fatalf("non-deterministic output:\n%+v\n%+v", first, second)
	}
	if len(first) == 0 {
		t.Fatal("expected some conflicts in fixture")
	}
}
