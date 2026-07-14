package render

import (
	"encoding/json"
	"time"

	"github.com/JaydenCJ/leafrake/internal/classify"
	"github.com/JaydenCJ/leafrake/internal/engine"
)

// SchemaVersion identifies the JSON envelope shape. It only changes on
// breaking changes to field names or semantics.
const SchemaVersion = 1

type jsonEnvelope struct {
	Tool          string       `json:"tool"`
	SchemaVersion int          `json:"schema_version"`
	Repo          jsonRepo     `json:"repo"`
	Base          jsonBase     `json:"base"`
	Summary       jsonSummary  `json:"summary"`
	Branches      []jsonBranch `json:"branches"`
}

type jsonRepo struct {
	Name string `json:"name"`
	Top  string `json:"top"`
	Head string `json:"head"` // "" when detached
}

type jsonBase struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type jsonSummary struct {
	Total        int `json:"total"`
	Merged       int `json:"merged"`
	SquashMerged int `json:"squash_merged"`
	Gone         int `json:"gone"`
	Stale        int `json:"stale"`
	Active       int `json:"active"`
	Protected    int `json:"protected"`
	DeletableNow int `json:"deletable_now"`
}

type jsonBranch struct {
	Name         string      `json:"name"`
	Tip          string      `json:"tip"`
	Category     string      `json:"category"`
	Deletable    bool        `json:"deletable"`
	Upstream     string      `json:"upstream,omitempty"`
	UpstreamGone bool        `json:"upstream_gone"`
	LastCommit   string      `json:"last_commit"`
	Subject      string      `json:"subject"`
	MergeBase    string      `json:"merge_base,omitempty"`
	Ahead        int         `json:"ahead"`
	Behind       int         `json:"behind"`
	SquashMatch  *jsonSquash `json:"squash_match,omitempty"`
	Evidence     []string    `json:"evidence"`
}

type jsonSquash struct {
	Commit  string `json:"commit"`
	Subject string `json:"subject"`
	Date    string `json:"date"`
}

// JSON renders the scan report as a stable, machine-readable envelope.
func JSON(r *engine.Report) ([]byte, error) {
	env := jsonEnvelope{
		Tool:          "leafrake",
		SchemaVersion: SchemaVersion,
		Repo:          jsonRepo{Name: r.RepoName, Top: r.RepoTop, Head: r.HeadBranch},
		Base:          jsonBase{Name: r.Base, Source: string(r.BaseSource)},
		Summary: jsonSummary{
			Total:        r.Summary.Total,
			Merged:       r.Summary.PerCat[classify.Merged],
			SquashMerged: r.Summary.PerCat[classify.Squashed],
			Gone:         r.Summary.PerCat[classify.Gone],
			Stale:        r.Summary.PerCat[classify.Stale],
			Active:       r.Summary.PerCat[classify.Active],
			Protected:    r.Summary.PerCat[classify.Protected],
			DeletableNow: r.Summary.Deletable,
		},
		Branches: make([]jsonBranch, 0, len(r.Verdicts)),
	}
	for _, v := range r.Verdicts {
		env.Branches = append(env.Branches, toJSONBranch(v))
	}
	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func toJSONBranch(v classify.Verdict) jsonBranch {
	b := jsonBranch{
		Name:         v.Facts.Name,
		Tip:          v.Facts.Tip,
		Category:     string(v.Category),
		Deletable:    v.Deletable,
		Upstream:     v.Facts.Upstream,
		UpstreamGone: v.Facts.UpstreamGone,
		LastCommit:   v.Facts.CommitterDate.UTC().Format(time.RFC3339),
		Subject:      v.Facts.Subject,
		MergeBase:    v.Facts.MergeBase,
		Ahead:        v.Facts.Ahead,
		Behind:       v.Facts.Behind,
		Evidence:     append([]string(nil), v.Reasons...),
	}
	if v.Squash != nil {
		b.SquashMatch = &jsonSquash{
			Commit:  v.Squash.Hash,
			Subject: v.Squash.Subject,
			Date:    v.Squash.When.UTC().Format(time.RFC3339),
		}
	}
	return b
}
