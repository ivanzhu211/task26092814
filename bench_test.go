package negaudit

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// buildFullScaleAccount builds the full-scale account: 5000 keywords with
// ~20k applicable negatives each, most of them from one shared list
// attached to every campaign.
//
//	50 campaigns x 10 ad groups x 10 keywords = 5000 keywords
//	1 shared list with 19000 negatives, attached to all campaigns
//	500 campaign-level negatives per campaign, 50 per ad group
//	=> 19000 + 500 + 50 = 19550 applicable negatives per keyword
func buildFullScaleAccount(b testing.TB) *Account {
	rng := rand.New(rand.NewSource(1))
	vocab := make([]string, 3000)
	for i := range vocab {
		vocab[i] = fmt.Sprintf("w%d", i)
	}
	randWords := func() string {
		n := 1 + rng.Intn(3)
		parts := make([]string, n)
		for i := range parts {
			parts[i] = vocab[rng.Intn(len(vocab))]
		}
		return strings.Join(parts, " ")
	}
	randEntry := func() string {
		s := randWords()
		switch rng.Intn(10) {
		case 0:
			return "[" + s + "]"
		case 1, 2:
			return `"` + s + `"`
		}
		return s
	}

	var sb strings.Builder
	sb.WriteString(`{"campaigns":[`)
	for c := 0; c < 50; c++ {
		if c > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"name":"c%d","shared_lists":["big"],"negatives":[`, c)
		for i := 0; i < 500; i++ {
			if i > 0 {
				sb.WriteByte(',')
			}
			fmt.Fprintf(&sb, "%q", randEntry())
		}
		sb.WriteString(`],"ad_groups":[`)
		for g := 0; g < 10; g++ {
			if g > 0 {
				sb.WriteByte(',')
			}
			fmt.Fprintf(&sb, `{"name":"g%d","keywords":[`, g)
			for k := 0; k < 10; k++ {
				if k > 0 {
					sb.WriteByte(',')
				}
				fmt.Fprintf(&sb, "%q", randEntry())
			}
			sb.WriteString(`],"negatives":[`)
			for i := 0; i < 50; i++ {
				if i > 0 {
					sb.WriteByte(',')
				}
				fmt.Fprintf(&sb, "%q", randEntry())
			}
			sb.WriteString(`]}`)
		}
		sb.WriteString(`]}`)
	}
	sb.WriteString(`],"shared_lists":{"big":[`)
	for i := 0; i < 19000; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%q", randEntry())
	}
	sb.WriteString(`]}}`)

	acc, invalid, err := LoadAccount(strings.NewReader(sb.String()))
	if err != nil {
		b.Fatal(err)
	}
	if len(invalid) != 0 {
		b.Fatalf("unexpected invalid entries: %d, first: %+v", len(invalid), invalid[0])
	}
	return acc
}

func BenchmarkAuditFullScale(b *testing.B) {
	acc := buildFullScaleAccount(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conflicts := Audit(acc)
		if i == 0 {
			b.Logf("conflicts: %d", len(conflicts))
		}
	}
}
