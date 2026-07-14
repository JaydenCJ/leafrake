// Package cli implements the leafrake command-line interface. Run takes
// argv and two writers and returns an exit code, so the whole surface is
// testable in-process without building a binary.
package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/JaydenCJ/leafrake/internal/engine"
	"github.com/JaydenCJ/leafrake/internal/version"
)

// Exit codes. Documented in the README; `scan --check` and failed
// deletions use ExitDead as their machine-readable verdict.
const (
	ExitOK      = 0
	ExitDead    = 1
	ExitUsage   = 2
	ExitRuntime = 3
)

// Run dispatches argv and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return runScan(nil, stdout, stderr)
	}
	switch args[0] {
	case "scan":
		return runScan(args[1:], stdout, stderr)
	case "clean":
		return runClean(args[1:], stdout, stderr)
	case "explain":
		return runExplain(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "leafrake %s\n", version.Version)
		return ExitOK
	case "help", "--help", "-h":
		usage(stdout)
		return ExitOK
	default:
		if strings.HasPrefix(args[0], "-") {
			fmt.Fprintf(stderr, "leafrake: unknown flag %q before a subcommand\n\n", args[0])
			usage(stderr)
			return ExitUsage
		}
		// Bare path: treat as `scan <path>`.
		return runScan(args, stdout, stderr)
	}
}

// multiFlag is a repeatable string flag.
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// scanFlags are shared by scan, clean, and explain — they parameterize the
// underlying scan.
type scanFlags struct {
	base         string
	staleDays    int
	squashWindow int
	protect      multiFlag
}

func (s *scanFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&s.base, "base", "", "base branch to compare against (default: auto-detect)")
	fs.IntVar(&s.staleDays, "stale-days", engine.DefaultStaleDays,
		"a branch untouched for this many days is stale (0 disables)")
	fs.IntVar(&s.squashWindow, "squash-window", engine.DefaultSquashWindow,
		"how many base commits to index for squash-merge detection")
	fs.Var(&s.protect, "protect", "never touch branches matching this glob (repeatable)")
}

func (s *scanFlags) toOptions(path string) engine.Options {
	return engine.Options{
		Path:         path,
		Base:         s.base,
		StaleDays:    s.staleDays,
		SquashWindow: s.squashWindow,
		Protect:      s.protect,
	}
}

// newFlagSet builds a silent FlagSet whose errors we render ourselves.
func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

// pathArg extracts the optional trailing repository path.
func pathArg(fs *flag.FlagSet, stderr io.Writer, cmd string) (string, bool) {
	switch fs.NArg() {
	case 0:
		return ".", true
	case 1:
		return fs.Arg(0), true
	default:
		fmt.Fprintf(stderr, "leafrake %s: at most one path argument, got %d\n", cmd, fs.NArg())
		return "", false
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `leafrake — classify local branches and delete the dead ones with evidence

Usage:
  leafrake scan    [flags] [path]     classify every local branch (default command)
  leafrake clean   [flags] [path]     delete dead branches (dry run unless --yes)
  leafrake explain [flags] <branch> [path]
                                      full evidence dossier for one branch
  leafrake version                    print the version

Shared flags:
  --base <branch>        base branch (default: origin/HEAD, then main/master/trunk/develop)
  --stale-days <n>       staleness threshold in days (default 90, 0 disables)
  --squash-window <n>    base commits indexed for squash detection (default 1000)
  --protect <glob>       never touch matching branches (repeatable)

scan flags:
  --format text|json     output format (default text)
  --check                exit 1 when deletable branches exist (for hooks)

clean flags:
  --select <categories>  comma list of merged,squash-merged,gone,stale
                         (default merged,squash-merged)
  --yes                  actually delete; without it clean is a dry run
  --format text|json     output format (default text)

Exit codes: 0 ok, 1 deletable branches found (--check) or a deletion failed,
2 usage error, 3 runtime error.
`)
}
