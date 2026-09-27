package negaudit

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Keyword is a single keyword with its position in the account.
type Keyword struct {
	Raw         string
	Campaign    string
	CampaignIdx int
	AdGroup     string
	AdGroupIdx  int
	KeywordIdx  int
}

// negEntry is a parsed negative together with its raw text.
type negEntry struct {
	raw string
	e   entry
}

// AdGroup is one ad group inside a campaign.
type AdGroup struct {
	Name      string
	Keywords  []Keyword
	keywords  []entry
	negatives []negEntry
}

// Campaign is one campaign with its attached shared list names.
type Campaign struct {
	Name        string
	SharedLists []string
	AdGroups    []AdGroup
	negatives   []negEntry
}

// Account is the loaded and validated account. Shared list entries are
// stored exactly once, no matter how many campaigns attach a list.
type Account struct {
	Campaigns []Campaign

	sharedLists map[string][]negEntry
	listNames   []string // sorted, for deterministic iteration
}

// InvalidEntry describes a skipped keyword/negative/list-attachment.
type InvalidEntry struct {
	Raw      string
	Reason   string
	Location string
}

// JSON wire format.
type accountJSON struct {
	Campaigns []struct {
		Name        string   `json:"name"`
		Negatives   []string `json:"negatives"`
		SharedLists []string `json:"shared_lists"`
		AdGroups    []struct {
			Name      string   `json:"name"`
			Keywords  []string `json:"keywords"`
			Negatives []string `json:"negatives"`
		} `json:"ad_groups"`
	} `json:"campaigns"`
	SharedLists map[string][]string `json:"shared_lists"`
}

// LoadAccount reads the account JSON and returns the account plus the list
// of invalid entries (which are skipped, not fatal).
func LoadAccount(r io.Reader) (*Account, []InvalidEntry, error) {
	var aj accountJSON
	dec := json.NewDecoder(r)
	if err := dec.Decode(&aj); err != nil {
		return nil, nil, fmt.Errorf("negaudit: decoding account JSON: %w", err)
	}

	acc := &Account{sharedLists: make(map[string][]negEntry)}
	var invalid []InvalidEntry

	// Shared lists first, so attachments can be validated. Entries are
	// stored once per list regardless of how many campaigns attach it.
	for name, raws := range aj.SharedLists {
		loc := fmt.Sprintf("shared_list %q", name)
		var negs []negEntry
		for _, raw := range raws {
			e, err := parseEntry(raw, true)
			if err != nil {
				invalid = append(invalid, InvalidEntry{Raw: raw, Reason: err.Error(), Location: loc})
				continue
			}
			negs = append(negs, negEntry{raw: raw, e: e})
		}
		acc.sharedLists[name] = negs
	}
	for name := range acc.sharedLists {
		acc.listNames = append(acc.listNames, name)
	}
	sort.Strings(acc.listNames)

	for ci, cj := range aj.Campaigns {
		camp := Campaign{Name: cj.Name}
		campLoc := fmt.Sprintf("campaign %q", cj.Name)

		for _, raw := range cj.Negatives {
			e, err := parseEntry(raw, true)
			if err != nil {
				invalid = append(invalid, InvalidEntry{Raw: raw, Reason: err.Error(), Location: campLoc})
				continue
			}
			camp.negatives = append(camp.negatives, negEntry{raw: raw, e: e})
		}

		for _, listName := range cj.SharedLists {
			if _, ok := acc.sharedLists[listName]; !ok {
				invalid = append(invalid, InvalidEntry{
					Raw:      listName,
					Reason:   "attachment to non-existent shared list",
					Location: campLoc,
				})
				continue
			}
			camp.SharedLists = append(camp.SharedLists, listName)
		}

		for gi, gj := range cj.AdGroups {
			ag := AdGroup{Name: gj.Name}
			agLoc := fmt.Sprintf("campaign %q ad_group %q", cj.Name, gj.Name)
			for ki, raw := range gj.Keywords {
				e, err := parseEntry(raw, false)
				if err != nil {
					invalid = append(invalid, InvalidEntry{Raw: raw, Reason: err.Error(), Location: agLoc})
					continue
				}
				ag.Keywords = append(ag.Keywords, Keyword{
					Raw:         raw,
					Campaign:    cj.Name,
					CampaignIdx: ci,
					AdGroup:     gj.Name,
					AdGroupIdx:  gi,
					KeywordIdx:  ki,
				})
				ag.keywords = append(ag.keywords, e)
			}
			for _, raw := range gj.Negatives {
				e, err := parseEntry(raw, true)
				if err != nil {
					invalid = append(invalid, InvalidEntry{Raw: raw, Reason: err.Error(), Location: agLoc})
					continue
				}
				ag.negatives = append(ag.negatives, negEntry{raw: raw, e: e})
			}
			camp.AdGroups = append(camp.AdGroups, ag)
		}
		acc.Campaigns = append(acc.Campaigns, camp)
	}

	return acc, invalid, nil
}
