// Command negaudit audits a paid-search account JSON file for
// negative-keyword conflicts, printing one line per invalid entry and one
// line per conflict.
package main

import (
	"fmt"
	"os"

	"negaudit"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: negaudit <account.json>\n")
		os.Exit(2)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "negaudit: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	acc, invalid, err := negaudit.LoadAccount(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "negaudit: %v\n", err)
		os.Exit(1)
	}
	w := os.Stdout
	for _, iv := range invalid {
		fmt.Fprintf(w, "INVALID %s %q: %s\n", iv.Path, iv.Raw, iv.Reason)
	}
	for _, rec := range negaudit.Audit(acc) {
		fmt.Fprintf(w,
			"%s campaigns[%d].ad_groups[%d].keywords[%d] %q blocked by %q from %s\n",
			rec.Level, rec.CampaignIndex, rec.AdGroupIndex, rec.KeywordIndex,
			rec.KeywordRaw, rec.NegativeRaw, rec.Source)
	}
}
