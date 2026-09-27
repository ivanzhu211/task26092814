package negaudit

import (
	"sort"
	"strings"
)

// Conflict classification, derived exactly from the matching semantics
// (K = keyword with word sequence k, N = negative with word sequence w):
//
//	K exact:  critical iff N blocks the single term k; warning impossible.
//	K phrase: N exact  -> warning iff w == k (critical impossible: S(K) infinite)
//	          N phrase -> critical iff w is a contiguous subsequence of k
//	          N broad  -> critical iff set(w) ⊆ set(k)
//	K broad:  N exact  -> warning iff w == k
//	          N phrase -> critical iff |w| == 1 and w[0] ∈ set(k);
//	                      warning iff w contiguous in k and |w| >= 2
//	          N broad  -> critical iff set(w) ⊆ set(k)
//
// "N blocks term k" means: exact -> w == k; phrase -> w contiguous in k;
// broad -> set(w) ⊆ set(k).

const keySep = "\x1f"

func seqKey(words []string) string { return strings.Join(words, keySep) }

// negGroup holds the distinct raw texts of one normalized negative form
// within a single source.
type negGroup struct {
	raws   []string // unique raw texts, first-occurrence order
	first  int      // smallest entry index within the source
	nwords int      // number of words in the normalized form
}

// negSource indexes the negatives of one ad group, one campaign, or one
// shared list. A shared list is indexed once regardless of mount count.
type negSource struct {
	label  string // record source description
	exact  map[string]*negGroup
	phrase map[string]*negGroup
	broad  map[string]*negGroup
}

func newNegSource(label string, negs []Negative) *negSource {
	s := &negSource{
		label:  label,
		exact:  make(map[string]*negGroup),
		phrase: make(map[string]*negGroup),
		broad:  make(map[string]*negGroup),
	}
	for i := range negs {
		n := &negs[i]
		var table map[string]*negGroup
		var key string
		switch n.Type {
		case Exact:
			table, key = s.exact, seqKey(n.Words)
		case Phrase:
			table, key = s.phrase, seqKey(n.Words)
		default:
			table, key = s.broad, seqKey(n.WordSet)
		}
		g, ok := table[key]
		if !ok {
			g = &negGroup{first: i, nwords: len(n.Words)}
			table[key] = g
		}
		dup := false
		for _, r := range g.raws {
			if r == n.Raw {
				dup = true
				break
			}
		}
		if !dup {
			g.raws = append(g.raws, n.Raw)
		}
	}
	return s
}

// srcHit ties a matching group to its source.
type srcHit struct {
	src *negSource
	grp *negGroup
}

// globalIndex maps normalized negative forms to every source containing
// them, so a keyword enumerates its own keys only once.
type globalIndex struct {
	exact  map[string][]srcHit
	phrase map[string][]srcHit
	broad  map[string][]srcHit
}

func buildGlobalIndex(sources []*negSource) *globalIndex {
	gi := &globalIndex{
		exact:  make(map[string][]srcHit),
		phrase: make(map[string][]srcHit),
		broad:  make(map[string][]srcHit),
	}
	for _, s := range sources {
		for key, g := range s.exact {
			gi.exact[key] = append(gi.exact[key], srcHit{s, g})
		}
		for key, g := range s.phrase {
			gi.phrase[key] = append(gi.phrase[key], srcHit{s, g})
		}
		for key, g := range s.broad {
			gi.broad[key] = append(gi.broad[key], srcHit{s, g})
		}
	}
	return gi
}

// match is one conflict found for a keyword, before record emission.
type match struct {
	rank  int // source rank within the keyword's environment
	first int // first entry index within the source
	raw   string
	label string
	level Level
}

// Audit returns all conflict records for the account in a deterministic
// order: campaign order, ad group order, keyword order, then negatives in
// canonical source order (ad group, campaign, mounted lists in mount order,
// entry order within each source).
func Audit(acc *Account) []ConflictRecord {
	// Build one negSource per ad group, campaign and shared list.
	srcs := make(map[*negSource]struct{})
	var all []*negSource
	campSrc := make([]*negSource, len(acc.Campaigns))
	grpSrc := make([][]*negSource, len(acc.Campaigns))
	listSrc := make(map[*SharedList]*negSource, len(acc.SharedLists))

	add := func(s *negSource) *negSource {
		if _, ok := srcs[s]; !ok {
			srcs[s] = struct{}{}
			all = append(all, s)
		}
		return s
	}
	for ci := range acc.Campaigns {
		c := &acc.Campaigns[ci]
		campSrc[ci] = add(newNegSource(`campaign "`+c.Name+`"`, c.Negatives))
		grpSrc[ci] = make([]*negSource, len(c.AdGroups))
		for gi := range c.AdGroups {
			g := &c.AdGroups[gi]
			grpSrc[ci][gi] = add(newNegSource(`ad group "`+g.Name+`"`, g.Negatives))
		}
		for _, lst := range c.Lists {
			if _, ok := listSrc[lst]; !ok {
				listSrc[lst] = add(newNegSource(`shared list "`+lst.Name+`"`, lst.Negatives))
			}
		}
	}
	gi := buildGlobalIndex(all)

	var out []ConflictRecord
	for ci := range acc.Campaigns {
		c := &acc.Campaigns[ci]
		for gj := range c.AdGroups {
			g := &c.AdGroups[gj]
			// Environment: ordered applicable sources and their ranks.
			env := make(map[*negSource]int, 2+len(c.Lists))
			env[grpSrc[ci][gj]] = 0
			env[campSrc[ci]] = 1
			for i, lst := range c.Lists {
				env[listSrc[lst]] = 2 + i
			}
			for ki := range g.Keywords {
				kw := &g.Keywords[ki]
				for _, m := range auditKeyword(gi, env, kw) {
					out = append(out, ConflictRecord{
						CampaignIndex: ci,
						AdGroupIndex:  gj,
						KeywordIndex:  ki,
						CampaignName:  c.Name,
						AdGroupName:   g.Name,
						KeywordRaw:    kw.Raw,
						NegativeRaw:   m.raw,
						Source:        m.label,
						Level:         m.level,
					})
				}
			}
		}
	}
	return out
}

// auditKeyword finds every applicable negative conflicting with kw by
// enumerating the keyword's own keys (one exact key, all contiguous
// subsequences, all word-set subsets) instead of scanning negatives.
func auditKeyword(gi *globalIndex, env map[*negSource]int, kw *Keyword) []match {
	seen := make(map[srcHit]struct{})
	var matches []match

	collect := func(hits []srcHit, level func(g *negGroup) (Level, bool)) {
		for _, h := range hits {
			rank, ok := env[h.src]
			if !ok {
				continue
			}
			if _, dup := seen[h]; dup {
				continue
			}
			lvl, ok := level(h.grp)
			if !ok {
				continue
			}
			seen[h] = struct{}{}
			for _, raw := range h.grp.raws {
				matches = append(matches, match{
					rank:  rank,
					first: h.grp.first,
					raw:   raw,
					label: h.src.label,
					level: lvl,
				})
			}
		}
	}

	// Exact negatives: sequence equality with the keyword's own term.
	collect(gi.exact[seqKey(kw.Words)], func(g *negGroup) (Level, bool) {
		if kw.Type == Exact {
			return Critical, true
		}
		return Warning, true // phrase or broad keyword
	})

	// Phrase negatives: every contiguous subsequence of the keyword term.
	words := kw.Words
	for i := 0; i < len(words); i++ {
		for j := i + 1; j <= len(words); j++ {
			collect(gi.phrase[seqKey(words[i:j])], func(g *negGroup) (Level, bool) {
				switch kw.Type {
				case Exact, Phrase:
					return Critical, true
				default: // broad keyword
					if g.nwords == 1 {
						return Critical, true
					}
					return Warning, true
				}
			})
		}
	}

	// Broad negatives: every non-empty subset of the keyword's word set.
	set := kw.WordSet
	for mask := 1; mask < 1<<uint(len(set)); mask++ {
		var sub []string
		for b := 0; b < len(set); b++ {
			if mask&(1<<uint(b)) != 0 {
				sub = append(sub, set[b])
			}
		}
		collect(gi.broad[seqKey(sub)], func(g *negGroup) (Level, bool) {
			return Critical, true
		})
	}

	sort.Slice(matches, func(a, b int) bool {
		if matches[a].rank != matches[b].rank {
			return matches[a].rank < matches[b].rank
		}
		if matches[a].first != matches[b].first {
			return matches[a].first < matches[b].first
		}
		return matches[a].raw < matches[b].raw
	})
	return matches
}
