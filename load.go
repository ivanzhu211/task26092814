package negaudit

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

type accountJSON struct {
	Campaigns   []campaignJSON      `json:"campaigns"`
	SharedLists map[string][]string `json:"shared_lists"`
}

type campaignJSON struct {
	Name        string        `json:"name"`
	Negatives   []string      `json:"negatives"`
	SharedLists []string      `json:"shared_lists"`
	AdGroups    []adGroupJSON `json:"ad_groups"`
}

type adGroupJSON struct {
	Name      string   `json:"name"`
	Keywords  []string `json:"keywords"`
	Negatives []string `json:"negatives"`
}

// LoadAccount reads an account JSON document, returning the account and the
// list of invalid entries (malformed items and dangling shared-list mounts)
// that were skipped. Invalid entries are reported in a deterministic order:
// shared lists by name, then campaigns in document order.
func LoadAccount(r io.Reader) (*Account, []InvalidEntry, error) {
	var doc accountJSON
	dec := json.NewDecoder(r)
	if err := dec.Decode(&doc); err != nil {
		return nil, nil, fmt.Errorf("decoding account JSON: %w", err)
	}
	acc := &Account{SharedLists: make(map[string]*SharedList, len(doc.SharedLists))}
	var invalid []InvalidEntry

	names := make([]string, 0, len(doc.SharedLists))
	for name := range doc.SharedLists {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lst := &SharedList{Name: name}
		for i, raw := range doc.SharedLists[name] {
			neg, ok := parseNegative(raw)
			if !ok {
				invalid = append(invalid, InvalidEntry{
					Path:   fmt.Sprintf("shared_lists[%q][%d]", name, i),
					Raw:    raw,
					Reason: "malformed entry",
				})
				continue
			}
			lst.Negatives = append(lst.Negatives, neg)
		}
		acc.SharedLists[name] = lst
	}

	for ci, c := range doc.Campaigns {
		camp := Campaign{Name: c.Name}
		cpath := fmt.Sprintf("campaigns[%d]", ci)
		for i, raw := range c.Negatives {
			neg, ok := parseNegative(raw)
			if !ok {
				invalid = append(invalid, InvalidEntry{
					Path:   fmt.Sprintf("%s.negatives[%d]", cpath, i),
					Raw:    raw,
					Reason: "malformed entry",
				})
				continue
			}
			camp.Negatives = append(camp.Negatives, neg)
		}
		for i, lname := range c.SharedLists {
			lst, ok := acc.SharedLists[lname]
			if !ok {
				invalid = append(invalid, InvalidEntry{
					Path:   fmt.Sprintf("%s.shared_lists[%d]", cpath, i),
					Raw:    lname,
					Reason: "mount of unknown shared list",
				})
				continue
			}
			camp.Lists = append(camp.Lists, lst)
		}
		for gi, g := range c.AdGroups {
			grp := AdGroup{Name: g.Name}
			gpath := fmt.Sprintf("%s.ad_groups[%d]", cpath, gi)
			for i, raw := range g.Keywords {
				kw, ok := parseKeyword(raw)
				if !ok {
					invalid = append(invalid, InvalidEntry{
						Path:   fmt.Sprintf("%s.keywords[%d]", gpath, i),
						Raw:    raw,
						Reason: "malformed entry",
					})
					continue
				}
				grp.Keywords = append(grp.Keywords, kw)
			}
			for i, raw := range g.Negatives {
				neg, ok := parseNegative(raw)
				if !ok {
					invalid = append(invalid, InvalidEntry{
						Path:   fmt.Sprintf("%s.negatives[%d]", gpath, i),
						Raw:    raw,
						Reason: "malformed entry",
					})
					continue
				}
				grp.Negatives = append(grp.Negatives, neg)
			}
			camp.AdGroups = append(camp.AdGroups, grp)
		}
		acc.Campaigns = append(acc.Campaigns, camp)
	}
	return acc, invalid, nil
}

func parseKeyword(raw string) (Keyword, bool) {
	words, mt, ok := parseEntry(raw, false)
	if !ok {
		return Keyword{}, false
	}
	return Keyword{Raw: raw, Type: mt, Words: words, WordSet: wordSet(words)}, true
}

func parseNegative(raw string) (Negative, bool) {
	words, mt, ok := parseEntry(raw, true)
	if !ok {
		return Negative{}, false
	}
	return Negative{Raw: raw, Type: mt, Words: words, WordSet: wordSet(words)}, true
}
