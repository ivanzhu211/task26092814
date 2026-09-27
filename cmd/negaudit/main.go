package main

import (
	"fmt"
	"os"

	"negaudit"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <account.json>\n", os.Args[0])
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

	for _, c := range negaudit.Audit(acc) {
		fmt.Printf("%s\tkeyword %q (campaign %q, ad_group %q)\tnegative %q (%s)\n",
			c.Level, c.Keyword.Raw, c.Keyword.Campaign, c.Keyword.AdGroup,
			c.Negative.Raw, negSourceDesc(c.Negative))
	}
	for _, inv := range invalid {
		fmt.Printf("invalid\t%q (%s) at %s\n", inv.Raw, inv.Reason, inv.Location)
	}
}

func negSourceDesc(n negaudit.NegativeRef) string {
	switch n.Source {
	case "ad_group":
		return fmt.Sprintf("ad_group %q, campaign %q", n.AdGroup, n.Campaign)
	case "campaign":
		return fmt.Sprintf("campaign %q", n.Campaign)
	case "shared_list":
		return fmt.Sprintf("shared_list %q", n.SharedList)
	}
	return n.Source
}
