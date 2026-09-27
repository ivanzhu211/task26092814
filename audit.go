package negaudit

import (
	"sort"
	"strings"
)

// Level is the severity of a conflict.
type Level string

const (
	Critical Level = "critical"
	Warning  Level = "warning"
)

// NegativeRef identifies the source of a negative entry.
type NegativeRef struct {
	Raw        string
	Source     string // "ad_group", "campaign" or "shared_list"
	Campaign   string
	AdGroup    string // only for Source == "ad_group"
	SharedList string // only for Source == "shared_list"
}

// Conflict is one (keyword, applicable negative) conflict record.
type Conflict struct {
	Keyword  Keyword
	Negative NegativeRef
	Level    Level
}

// srcKind identifies where a negative entry lives.
type srcKind int

const (
	srcCampaign srcKind = iota
	srcAdGroup
	srcSharedList
)

// negSource is the applicability scope of one negative entry.
type negSource struct {
	kind     srcKind
	campIdx  int
	adGrpIdx int // only for srcAdGroup
	listName string
}

// globalNeg is one negative entry stored once in the account-wide table.
type globalNeg struct {
	raw    string
	match  MatchType
	nWords int
	seqKey string // words joined by ' '
	setKey string // sorted distinct words joined by ' '
	src    negSource
	ref    NegativeRef
}

// negIndex holds account-wide lookups from a keyword signature to the ids of
// negative entries that the signature selects.
type negIndex struct {
	negs        []globalNeg
	exactBySeq  map[string][]int // Exact negatives by word sequence
	phraseBySeq map[string][]int // Phrase negatives by word sequence
	broadBySet  map[string][]int // Broad negatives by word set
}

func seqKeyOf(words []string) string { return strings.Join(words, " ") }

func setKeyOf(words []string) string {
	uniq := make(map[string]struct{}, len(words))
	out := make([]string, 0, len(words))
	for _, w := range words {
		if _, ok := uniq[w]; !ok {
			uniq[w] = struct{}{}
			out = append(out, w)
		}
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// buildNegIndex collects every negative entry in the account exactly once
// (shared list entries are not duplicated per attachment) and assigns each a
// canonical id: campaign negatives and ad group negatives in account order,
// then shared list entries in sorted list-name order. The id order is the
// deterministic output order within one keyword's records.
func buildNegIndex(acc *Account) *negIndex {
	idx := &negIndex{
		exactBySeq:  make(map[string][]int),
		phraseBySeq: make(map[string][]int),
		broadBySet:  make(map[string][]int),
	}
	add := func(raw string, e entry, src negSource, ref NegativeRef) {
		g := globalNeg{
			raw:    raw,
			match:  e.match,
			nWords: len(e.words),
			seqKey: seqKeyOf(e.words),
			src:    src,
			ref:    ref,
		}
		if e.match == Broad {
			g.setKey = setKeyOf(e.words)
		}
		id := len(idx.negs)
		idx.negs = append(idx.negs, g)
		switch e.match {
		case Exact:
			idx.exactBySeq[g.seqKey] = append(idx.exactBySeq[g.seqKey], id)
		case Phrase:
			idx.phraseBySeq[g.seqKey] = append(idx.phraseBySeq[g.seqKey], id)
		case Broad:
			idx.broadBySet[g.setKey] = append(idx.broadBySet[g.setKey], id)
		}
	}

	for ci := range acc.Campaigns {
		c := &acc.Campaigns[ci]
		for _, n := range c.negatives {
			add(n.raw, n.e, negSource{kind: srcCampaign, campIdx: ci},
				NegativeRef{Raw: n.raw, Source: "campaign", Campaign: c.Name})
		}
		for gi := range c.AdGroups {
			ag := &c.AdGroups[gi]
			for _, n := range ag.negatives {
				add(n.raw, n.e, negSource{kind: srcAdGroup, campIdx: ci, adGrpIdx: gi},
					NegativeRef{Raw: n.raw, Source: "ad_group", Campaign: c.Name, AdGroup: ag.Name})
			}
		}
	}
	for _, name := range acc.listNames {
		for _, n := range acc.sharedLists[name] {
			add(n.raw, n.e, negSource{kind: srcSharedList, listName: name},
				NegativeRef{Raw: n.raw, Source: "shared_list", SharedList: name})
		}
	}
	return idx
}

// appliesTo reports whether the negative's source scope covers the keyword
// located at (campIdx, adGrpIdx) in a campaign mounting lists.
func (s negSource) appliesTo(campIdx, adGrpIdx int, lists map[string]struct{}) bool {
	switch s.kind {
	case srcCampaign:
		return s.campIdx == campIdx
	case srcAdGroup:
		return s.campIdx == campIdx && s.adGrpIdx == adGrpIdx
	case srcSharedList:
		_, ok := lists[s.listName]
		return ok
	}
	return false
}

// levelFor returns the conflict level for a (keyword, negative) pair whose
// selecting condition already matched, per the semantic rules:
//
//	K\N     Exact            Phrase                        Broad
//	exact   C: seq equal     C: N subseq of K              C: set(N) ⊆ set(K)
//	phrase  W: seq equal     C: N subseq of K              C: set(N) ⊆ set(K)
//	broad   W: seq equal     C: N=[w], w∈K / W: subseq     C: set(N) ⊆ set(K)
func levelFor(kwMatch MatchType, neg globalNeg) Level {
	switch kwMatch {
	case Exact:
		return Critical
	case Phrase:
		if neg.match == Exact {
			return Warning
		}
		return Critical
	case Broad:
		switch neg.match {
		case Exact:
			return Warning
		case Phrase:
			if neg.nWords == 1 {
				return Critical
			}
			return Warning
		}
		return Critical
	}
	return Critical
}

// Audit returns all conflict records for the account. Records are grouped by
// keyword in account order; within one keyword they are ordered by the
// canonical negative id (account order of negatives, shared lists last in
// sorted name order). Repeated calls on the same input produce identical
// output.
func Audit(acc *Account) []Conflict {
	idx := buildNegIndex(acc)
	var out []Conflict

	for ci := range acc.Campaigns {
		c := &acc.Campaigns[ci]
		lists := make(map[string]struct{}, len(c.SharedLists))
		for _, name := range c.SharedLists {
			lists[name] = struct{}{}
		}
		for gi := range c.AdGroups {
			ag := &c.AdGroups[gi]
			for ki, kw := range ag.Keywords {
				ke := ag.keywords[ki]
				out = auditKeyword(idx, kw, ke, ci, gi, lists, out)
			}
		}
	}
	return out
}

func auditKeyword(idx *negIndex, kw Keyword, ke entry, campIdx, adGrpIdx int, lists map[string]struct{}, out []Conflict) []Conflict {
	// matched holds the best (highest) level found per negative id; a pair
	// yields at most one record.
	matched := make(map[int]Level)

	consider := func(ids []int) {
		for _, id := range ids {
			g := &idx.negs[id]
			if !g.src.appliesTo(campIdx, adGrpIdx, lists) {
				continue
			}
			lvl := levelFor(ke.match, *g)
			if cur, ok := matched[id]; !ok || lvl == Critical && cur == Warning {
				matched[id] = lvl
			}
		}
	}

	seqKey := seqKeyOf(ke.words)

	// Exact negatives with the same word sequence as the keyword.
	consider(idx.exactBySeq[seqKey])

	// Phrase negatives whose word sequence is a contiguous subsequence of
	// the keyword's word sequence (length-1 subsequences included, which
	// also covers the Broad-keyword / single-word-Phrase-negative case).
	seenSub := make(map[string]struct{})
	for i := 0; i < len(ke.words); i++ {
		for j := i + 1; j <= len(ke.words); j++ {
			sub := strings.Join(ke.words[i:j], " ")
			if _, ok := seenSub[sub]; ok {
				continue
			}
			seenSub[sub] = struct{}{}
			consider(idx.phraseBySeq[sub])
		}
	}

	// Broad negatives whose word set is a subset of the keyword's word set:
	// enumerate every non-empty subset of the keyword's distinct words.
	distinct := distinctSorted(ke.words)
	for mask := 1; mask < 1<<uint(len(distinct)); mask++ {
		var buf []string
		for b, w := range distinct {
			if mask&(1<<uint(b)) != 0 {
				buf = append(buf, w)
			}
		}
		consider(idx.broadBySet[strings.Join(buf, " ")])
	}

	if len(matched) == 0 {
		return out
	}
	ids := make([]int, 0, len(matched))
	for id := range matched {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		out = append(out, Conflict{
			Keyword:  kw,
			Negative: idx.negs[id].ref,
			Level:    matched[id],
		})
	}
	return out
}

func distinctSorted(words []string) []string {
	uniq := make(map[string]struct{}, len(words))
	out := make([]string, 0, len(words))
	for _, w := range words {
		if _, ok := uniq[w]; !ok {
			uniq[w] = struct{}{}
			out = append(out, w)
		}
	}
	sort.Strings(out)
	return out
}
