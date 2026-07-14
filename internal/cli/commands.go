package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/JaydenCJ/leafrake/internal/classify"
	"github.com/JaydenCJ/leafrake/internal/engine"
	"github.com/JaydenCJ/leafrake/internal/render"
)

// runScan implements `leafrake scan`.
func runScan(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("scan", stderr)
	var sf scanFlags
	sf.register(fs)
	format := fs.String("format", "text", "output format: text or json")
	check := fs.Bool("check", false, "exit 1 when deletable branches exist")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "leafrake scan: invalid --format %q (want text or json)\n", *format)
		return ExitUsage
	}
	path, ok := pathArg(fs, stderr, "scan")
	if !ok {
		return ExitUsage
	}
	report, err := engine.Scan(sf.toOptions(path))
	if err != nil {
		fmt.Fprintf(stderr, "leafrake scan: %v\n", err)
		return ExitRuntime
	}
	if *format == "json" {
		out, err := render.JSON(report)
		if err != nil {
			fmt.Fprintf(stderr, "leafrake scan: %v\n", err)
			return ExitRuntime
		}
		stdout.Write(out)
	} else {
		io.WriteString(stdout, render.Text(report))
	}
	if *check && report.Summary.Deletable > 0 {
		return ExitDead
	}
	return ExitOK
}

// defaultSelect is what clean deletes without opt-in: only categories whose
// content provably lives on the base branch.
var defaultSelect = []string{"merged", "squash-merged"}

// validSelect maps --select tokens to categories. gone and stale are
// deliberately opt-in: they are heuristics, not proofs.
var validSelect = map[string]classify.Category{
	"merged":        classify.Merged,
	"squash-merged": classify.Squashed,
	"gone":          classify.Gone,
	"stale":         classify.Stale,
}

// runClean implements `leafrake clean`.
func runClean(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("clean", stderr)
	var sf scanFlags
	sf.register(fs)
	format := fs.String("format", "text", "output format: text or json")
	sel := fs.String("select", strings.Join(defaultSelect, ","),
		"comma-separated categories to delete")
	yes := fs.Bool("yes", false, "actually delete (without it, dry run)")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "leafrake clean: invalid --format %q (want text or json)\n", *format)
		return ExitUsage
	}
	categories, selSet, err := parseSelect(*sel)
	if err != nil {
		fmt.Fprintf(stderr, "leafrake clean: %v\n", err)
		return ExitUsage
	}
	path, ok := pathArg(fs, stderr, "clean")
	if !ok {
		return ExitUsage
	}
	report, err := engine.Scan(sf.toOptions(path))
	if err != nil {
		fmt.Fprintf(stderr, "leafrake clean: %v\n", err)
		return ExitRuntime
	}
	var selected []classify.Verdict
	for _, v := range report.Verdicts {
		if selSet[v.Category] {
			selected = append(selected, v)
		}
	}

	if !*yes {
		if *format == "json" {
			return writeCleanJSON(stdout, stderr, selected, nil, categories, true)
		}
		io.WriteString(stdout, render.CleanPlan(selected, categories))
		return ExitOK
	}

	results := engine.Delete(path, selected)
	failed := 0
	for _, r := range results {
		if r.Err != nil {
			failed++
		}
	}
	if *format == "json" {
		code := writeCleanJSON(stdout, stderr, selected, results, categories, false)
		if code != ExitOK {
			return code
		}
	} else {
		io.WriteString(stdout, render.CleanResults(results, categories))
	}
	if failed > 0 {
		return ExitDead
	}
	return ExitOK
}

// cleanJSON is the machine-readable clean envelope.
type cleanJSON struct {
	Tool          string          `json:"tool"`
	SchemaVersion int             `json:"schema_version"`
	DryRun        bool            `json:"dry_run"`
	Selection     []string        `json:"selection"`
	Branches      []cleanJSONItem `json:"branches"`
}

type cleanJSONItem struct {
	Name     string   `json:"name"`
	Tip      string   `json:"tip"`
	Category string   `json:"category"`
	Evidence []string `json:"evidence"`
	Deleted  bool     `json:"deleted"`
	Error    string   `json:"error,omitempty"`
}

func writeCleanJSON(stdout, stderr io.Writer, selected []classify.Verdict,
	results []engine.DeleteResult, categories []string, dryRun bool) int {

	env := cleanJSON{
		Tool:          "leafrake",
		SchemaVersion: render.SchemaVersion,
		DryRun:        dryRun,
		Selection:     categories,
		Branches:      make([]cleanJSONItem, 0, len(selected)),
	}
	errByBranch := make(map[string]error, len(results))
	deleted := make(map[string]bool, len(results))
	for _, r := range results {
		errByBranch[r.Branch] = r.Err
		deleted[r.Branch] = r.Err == nil
	}
	for _, v := range selected {
		item := cleanJSONItem{
			Name:     v.Facts.Name,
			Tip:      v.Facts.Tip,
			Category: string(v.Category),
			Evidence: append([]string(nil), v.Reasons...),
			Deleted:  !dryRun && deleted[v.Facts.Name],
		}
		if err := errByBranch[v.Facts.Name]; err != nil {
			item.Error = err.Error()
		}
		env.Branches = append(env.Branches, item)
	}
	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "leafrake clean: %v\n", err)
		return ExitRuntime
	}
	stdout.Write(append(out, '\n'))
	return ExitOK
}

// parseSelect validates the --select list into a category set, preserving
// the user's order for display.
func parseSelect(spec string) ([]string, map[classify.Category]bool, error) {
	set := make(map[classify.Category]bool)
	var categories []string
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		cat, ok := validSelect[tok]
		if !ok {
			return nil, nil, fmt.Errorf(
				"invalid --select %q (want a comma list of merged,squash-merged,gone,stale)", tok)
		}
		if !set[cat] {
			categories = append(categories, tok)
		}
		set[cat] = true
	}
	if len(categories) == 0 {
		return nil, nil, fmt.Errorf("--select is empty")
	}
	return categories, set, nil
}

// runExplain implements `leafrake explain <branch> [path]`.
func runExplain(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("explain", stderr)
	var sf scanFlags
	sf.register(fs)
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() < 1 || fs.NArg() > 2 {
		fmt.Fprintf(stderr, "leafrake explain: want <branch> [path], got %d arguments\n", fs.NArg())
		return ExitUsage
	}
	branch := fs.Arg(0)
	path := "."
	if fs.NArg() == 2 {
		path = fs.Arg(1)
	}
	report, err := engine.Scan(sf.toOptions(path))
	if err != nil {
		fmt.Fprintf(stderr, "leafrake explain: %v\n", err)
		return ExitRuntime
	}
	for _, v := range report.Verdicts {
		if v.Facts.Name == branch {
			io.WriteString(stdout, render.Explain(report, v))
			return ExitOK
		}
	}
	fmt.Fprintf(stderr, "leafrake explain: no local branch named %q\n", branch)
	return ExitRuntime
}
