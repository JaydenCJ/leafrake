// Package baseref picks the base branch every other branch is compared
// against — with zero configuration. The pick is a pure function over facts
// the caller gathered, so every rule is unit-testable.
package baseref

import "fmt"

// Source records how the base was chosen, so reports can show it.
type Source string

const (
	SourceFlag       Source = "--base flag"
	SourceOriginHead Source = "origin/HEAD"
	SourceLocalName  Source = "well-known local branch name"
)

// wellKnown are the conventional base branch names, tried in order when the
// repository has no origin/HEAD to consult.
var wellKnown = []string{"main", "master", "trunk", "develop"}

// Pick chooses the base branch.
//
//   - explicit: value of --base, "" when not given (always wins when set).
//   - originHead: what origin/HEAD points at ("origin/main", "" when unset).
//   - locals: the short names of all local branches.
//
// When origin/HEAD names a branch that also exists locally, the local
// branch wins (it is what the user merges into); otherwise the remote ref
// itself is used, which works for every comparison leafrake performs.
func Pick(explicit, originHead string, locals []string) (name string, src Source, err error) {
	if explicit != "" {
		return explicit, SourceFlag, nil
	}
	localSet := make(map[string]bool, len(locals))
	for _, l := range locals {
		localSet[l] = true
	}
	if originHead != "" {
		if short := stripRemote(originHead); localSet[short] {
			return short, SourceOriginHead, nil
		}
		return originHead, SourceOriginHead, nil
	}
	for _, candidate := range wellKnown {
		if localSet[candidate] {
			return candidate, SourceLocalName, nil
		}
	}
	return "", "", fmt.Errorf(
		"cannot detect a base branch (no origin/HEAD, none of %v exists locally); pass --base",
		wellKnown)
}

// stripRemote turns "origin/main" into "main". Branch names may themselves
// contain slashes, so only the first segment (the remote name) is removed.
func stripRemote(remoteRef string) string {
	for i := 0; i < len(remoteRef); i++ {
		if remoteRef[i] == '/' {
			return remoteRef[i+1:]
		}
	}
	return remoteRef
}
