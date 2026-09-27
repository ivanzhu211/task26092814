package negaudit

import (
	"strings"
	"testing"
)

func TestLoadAccount(t *testing.T) {
	doc := `{
	  "campaigns": [
	    {
	      "name": "c1",
	      "negatives": ["-a", "[broken"],
	      "shared_lists": ["L1", "nope"],
	      "ad_groups": [
	        {"name": "g1", "keywords": ["x y", "[bad"], "negatives": ["\"p q\""]}
	      ]
	    },
	    {"name": "c2", "shared_lists": ["L1"], "ad_groups": [
	      {"name": "g2", "keywords": ["z"]}
	    ]}
	  ],
	  "shared_lists": {"L1": ["-m", "!!!"], "L2": []}
	}`
	acc, invalid, err := LoadAccount(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("LoadAccount: %v", err)
	}

	if len(acc.Campaigns) != 2 {
		t.Fatalf("got %d campaigns, want 2", len(acc.Campaigns))
	}
	// The shared list is stored once and mounted by both campaigns.
	if acc.Campaigns[0].Lists[0] != acc.Campaigns[1].Lists[0] {
		t.Fatal("shared list L1 not shared between campaigns")
	}
	if len(acc.SharedLists["L1"].Negatives) != 1 {
		t.Fatalf("L1 has %d negatives, want 1", len(acc.SharedLists["L1"].Negatives))
	}
	if len(acc.Campaigns[0].Negatives) != 1 {
		t.Fatalf("c1 has %d negatives, want 1", len(acc.Campaigns[0].Negatives))
	}
	g1 := acc.Campaigns[0].AdGroups[0]
	if len(g1.Keywords) != 1 || len(g1.Negatives) != 1 {
		t.Fatalf("g1 has %d keywords, %d negatives; want 1, 1",
			len(g1.Keywords), len(g1.Negatives))
	}

	wantPaths := []string{
		`shared_lists["L1"][1]`,
		"campaigns[0].negatives[1]",
		"campaigns[0].shared_lists[1]",
		"campaigns[0].ad_groups[0].keywords[1]",
	}
	if len(invalid) != len(wantPaths) {
		t.Fatalf("got %d invalid entries %v, want %d", len(invalid), invalid, len(wantPaths))
	}
	for i, p := range wantPaths {
		if invalid[i].Path != p {
			t.Errorf("invalid[%d].Path = %q, want %q", i, invalid[i].Path, p)
		}
	}
	if invalid[2].Raw != "nope" || invalid[2].Reason != "mount of unknown shared list" {
		t.Errorf("invalid[2] = %+v", invalid[2])
	}

	// End to end: keyword "x y" in g1 is critically blocked by "-x y"? No:
	// campaign negative is "-a"; list negative is "-m". Add a real conflict
	// check via Audit on keyword "z" with no negatives -> no records.
	if recs := Audit(acc); len(recs) != 0 {
		t.Fatalf("got %d conflict records, want 0", len(recs))
	}
}

func TestLoadAccountBadJSON(t *testing.T) {
	if _, _, err := LoadAccount(strings.NewReader("{")); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}
