package negaudit

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// buildFullScaleAccount builds the full-scale account from the requirements:
// 5000 keywords, each with 20000 applicable negatives in total, most of them
// coming from a single shared list mounted on every campaign.
func buildFullScaleAccount() *Account {
	rng := rand.New(rand.NewSource(42))

	const vocabSize = 2000
	vocab := make([]string, vocabSize)
	for i := range vocab {
		vocab[i] = fmt.Sprintf("w%d", i)
	}
	randWords := func(max int) []string {
		n := 1 + rng.Intn(max)
		ws := make([]string, n)
		for i := range ws {
			ws[i] = vocab[rng.Intn(vocabSize)]
		}
		return ws
	}
	randEntry := func(negative bool, maxWords int) string {
		ws := randWords(maxWords)
		var s string
		switch rng.Intn(3) {
		case 0:
			s = "[" + strings.Join(ws, " ") + "]"
		case 1:
			s = `"` + strings.Join(ws, " ") + `"`
		default:
			s = strings.Join(ws, " ")
		}
		if negative {
			s = "-" + s
		}
		return s
	}

	// One shared list with 19800 negatives, mounted on every campaign.
	global := &SharedList{Name: "global"}
	for i := 0; i < 19800; i++ {
		n, ok := parseNegative(randEntry(true, 3))
		if !ok {
			panic("generated invalid negative")
		}
		global.Negatives = append(global.Negatives, n)
	}

	// 50 campaigns x 10 ad groups x 10 keywords = 5000 keywords.
	// Per keyword: 19800 (list) + 100 (campaign) + 100 (ad group) = 20000
	// applicable negatives.
	acc := &Account{SharedLists: map[string]*SharedList{"global": global}}
	for ci := 0; ci < 50; ci++ {
		c := Campaign{Name: fmt.Sprintf("camp-%d", ci), Lists: []*SharedList{global}}
		for i := 0; i < 100; i++ {
			n, _ := parseNegative(randEntry(true, 3))
			c.Negatives = append(c.Negatives, n)
		}
		for gi := 0; gi < 10; gi++ {
			g := AdGroup{Name: fmt.Sprintf("grp-%d", gi)}
			for i := 0; i < 100; i++ {
				n, _ := parseNegative(randEntry(true, 3))
				g.Negatives = append(g.Negatives, n)
			}
			for i := 0; i < 10; i++ {
				kw, ok := parseKeyword(randEntry(false, 5))
				if !ok {
					panic("generated invalid keyword")
				}
				g.Keywords = append(g.Keywords, kw)
			}
			c.AdGroups = append(c.AdGroups, g)
		}
		acc.Campaigns = append(acc.Campaigns, c)
	}
	return acc
}

func BenchmarkAuditFullScale(b *testing.B) {
	acc := buildFullScaleAccount()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recs := Audit(acc)
		if len(recs) == 0 {
			b.Fatal("expected conflict records")
		}
	}
}

// TestFullScaleRuns guards the benchmark shape: the full-scale account must
// audit to a non-trivial, deterministic set of records.
func TestFullScaleRuns(t *testing.T) {
	acc := buildFullScaleAccount()
	recs := Audit(acc)
	if len(recs) == 0 {
		t.Fatal("expected conflict records")
	}
	again := Audit(acc)
	if len(again) != len(recs) {
		t.Fatalf("non-deterministic record count: %d vs %d", len(recs), len(again))
	}
	for i := range recs {
		if recs[i] != again[i] {
			t.Fatalf("record %d differs between runs", i)
		}
	}
	t.Logf("full-scale account produced %d conflict records", len(recs))
}
