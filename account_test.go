package negaudit

import (
	"strings"
	"testing"
)

func TestLoadAccountInvalidEntries(t *testing.T) {
	jsonAcct := `{
	  "campaigns": [
	    {"name": "c1",
	     "negatives": ["[broken", "ok"],
	     "shared_lists": ["L", "missing"],
	     "ad_groups": [
	       {"name": "g1",
	        "keywords": ["good", "[unbalanced", "a b c d e f g h i j k"],
	        "negatives": ["!!!", "fine"]}
	     ]}
	  ],
	  "shared_lists": {"L": ["valid", "[bad"]}
	}`
	acc, invalid, err := LoadAccount(strings.NewReader(jsonAcct))
	if err != nil {
		t.Fatal(err)
	}
	if len(invalid) != 6 {
		t.Fatalf("got %d invalid entries, want 6: %+v", len(invalid), invalid)
	}
	var listMount, kwBad, negBad, sharedBad int
	for _, inv := range invalid {
		switch {
		case strings.Contains(inv.Reason, "non-existent shared list"):
			listMount++
			if inv.Raw != "missing" {
				t.Errorf("bad list mount entry: %+v", inv)
			}
		case strings.Contains(inv.Location, "ad_group") && strings.Contains(inv.Location, "g1") && inv.Raw == "[unbalanced":
			kwBad++
		case inv.Raw == "!!!":
			negBad++
		case strings.Contains(inv.Location, `shared_list "L"`):
			sharedBad++
		}
	}
	if listMount != 1 || kwBad != 1 || negBad != 1 || sharedBad != 1 {
		t.Errorf("invalid entry classification wrong: %+v", invalid)
	}

	// Valid parts survive.
	if len(acc.Campaigns) != 1 || len(acc.Campaigns[0].AdGroups) != 1 {
		t.Fatalf("account structure broken: %+v", acc.Campaigns)
	}
	ag := acc.Campaigns[0].AdGroups[0]
	if len(ag.Keywords) != 1 || ag.Keywords[0].Raw != "good" {
		t.Errorf("valid keywords not kept: %+v", ag.Keywords)
	}
	if len(acc.Campaigns[0].SharedLists) != 1 || acc.Campaigns[0].SharedLists[0] != "L" {
		t.Errorf("valid list attachment not kept: %+v", acc.Campaigns[0].SharedLists)
	}
}

func TestSharedListStoredOnceAndAppliesEverywhere(t *testing.T) {
	jsonAcct := `{
	  "campaigns": [
	    {"name": "c1", "shared_lists": ["L"],
	     "ad_groups": [{"name": "g1", "keywords": ["[a b]"], "negatives": []}]},
	    {"name": "c2", "shared_lists": ["L"],
	     "ad_groups": [{"name": "g2", "keywords": ["[a b]"], "negatives": []}]}
	  ],
	  "shared_lists": {"L": ["[a b]"]}
	}`
	acc, invalid, err := LoadAccount(strings.NewReader(jsonAcct))
	if err != nil {
		t.Fatal(err)
	}
	if len(invalid) != 0 {
		t.Fatalf("unexpected invalid: %+v", invalid)
	}
	if got := len(acc.sharedLists["L"]); got != 1 {
		t.Fatalf("shared list stored %d times, want 1", got)
	}
	conflicts := Audit(acc)
	if len(conflicts) != 2 {
		t.Fatalf("got %d conflicts, want 2 (one per campaign): %+v", len(conflicts), conflicts)
	}
	for _, c := range conflicts {
		if c.Level != Critical || c.Negative.Source != "shared_list" || c.Negative.SharedList != "L" {
			t.Errorf("bad conflict: %+v", c)
		}
	}
}

func TestAuditNineCombos(t *testing.T) {
	// One keyword and one negative per match-type combination, all in a
	// single ad group so every pair is applicable. Levels follow the
	// semantic table documented in levelFor.
	cases := []struct {
		name string
		kw   string
		neg  string
		want Level
	}{
		{"exact/exact critical", "[a b]", "[a b]", Critical},
		{"exact/exact none", "[a b]", "[b a]", ""},
		{"exact/phrase critical", "[a b c]", `"b c"`, Critical},
		{"exact/phrase none", "[a b c]", `"c b"`, ""},
		{"exact/broad critical", "[a b]", "b a a", Critical},
		{"exact/broad none", "[a b]", "a c", ""},

		{"phrase/exact warning", `"a b"`, "[a b]", Warning},
		{"phrase/exact none", `"a b"`, "[a b c]", ""},
		{"phrase/phrase critical", `"a b c"`, `"a b"`, Critical},
		{"phrase/phrase none", `"a b"`, `"a b c"`, ""},
		{"phrase/broad critical", `"a b"`, "b a", Critical},
		{"phrase/broad none", `"a b"`, "a z", ""},

		{"broad/exact warning", "a b", "[a b]", Warning},
		{"broad/exact none", "a b", "[a b c]", ""},
		{"broad/phrase critical single word", "a b", `"a"`, Critical},
		{"broad/phrase warning multi word", "a b c", `"b c"`, Warning},
		{"broad/phrase none", "a b", `"b a"`, ""},
		{"broad/broad critical", "a b c", "c a", Critical},
		{"broad/broad none", "a b", "a z", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jsonAcct := `{"campaigns":[{"name":"c","ad_groups":[{"name":"g","keywords":[` +
				jsonString(tc.kw) + `],"negatives":[` + jsonString(tc.neg) + `]}]}]}`
			acc, invalid, err := LoadAccount(strings.NewReader(jsonAcct))
			if err != nil {
				t.Fatal(err)
			}
			if len(invalid) != 0 {
				t.Fatalf("unexpected invalid: %+v", invalid)
			}
			got := Audit(acc)
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("got %+v, want no record", got)
				}
				return
			}
			if len(got) != 1 || got[0].Level != tc.want {
				t.Fatalf("got %+v, want one %s record", got, tc.want)
			}
		})
	}
}

func jsonString(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
