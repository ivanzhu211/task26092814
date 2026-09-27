// Package negaudit audits paid-search accounts for negative-keyword conflicts.
package negaudit

// MatchType is the match type of a keyword or negative entry.
type MatchType int

const (
	Broad MatchType = iota
	Phrase
	Exact
)

func (m MatchType) String() string {
	switch m {
	case Phrase:
		return "phrase"
	case Exact:
		return "exact"
	default:
		return "broad"
	}
}

// Level is the severity of a conflict record.
type Level int

const (
	Warning Level = iota
	Critical
)

func (l Level) String() string {
	if l == Critical {
		return "critical"
	}
	return "warning"
}

// Keyword is a normalized keyword entry.
type Keyword struct {
	Raw     string
	Type    MatchType
	Words   []string // word sequence after normalization
	WordSet []string // sorted distinct words
}

// Negative is a normalized negative entry.
type Negative struct {
	Raw     string
	Type    MatchType
	Words   []string
	WordSet []string
}

// AdGroup is one ad group inside a campaign.
type AdGroup struct {
	Name      string
	Keywords  []Keyword
	Negatives []Negative
}

// SharedList is a named shared negative list, stored once no matter how
// many campaigns mount it.
type SharedList struct {
	Name      string
	Negatives []Negative
}

// Campaign is one campaign; Lists holds the resolved shared-list mounts in
// declared order.
type Campaign struct {
	Name      string
	Negatives []Negative
	Lists     []*SharedList
	AdGroups  []AdGroup
}

// Account is a fully loaded account.
type Account struct {
	Campaigns   []Campaign
	SharedLists map[string]*SharedList
}

// InvalidEntry reports a skipped malformed entry or dangling list mount.
type InvalidEntry struct {
	Path   string // e.g. campaigns[0].ad_groups[1].negatives[2]
	Raw    string // raw text (empty for dangling mounts)
	Reason string
}

// ConflictRecord is one (keyword, applicable negative) conflict.
type ConflictRecord struct {
	CampaignIndex int
	AdGroupIndex  int
	KeywordIndex  int
	CampaignName  string
	AdGroupName   string
	KeywordRaw    string
	NegativeRaw   string
	Source        string // e.g. ad group "g", campaign "c", shared list "l"
	Level         Level
}
