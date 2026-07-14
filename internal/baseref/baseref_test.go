// Unit tests for zero-configuration base branch detection: the --base
// override, origin/HEAD, and the well-known-name fallback chain.
package baseref

import "testing"

func TestExplicitFlagAlwaysWins(t *testing.T) {
	name, src, err := Pick("develop", "origin/main", []string{"main", "develop"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "develop" || src != SourceFlag {
		t.Fatalf("got %q from %q", name, src)
	}
}

func TestExplicitFlagMayNameARemoteRef(t *testing.T) {
	// --base origin/release is legal even with no matching local branch;
	// the engine verifies resolvability separately.
	name, src, err := Pick("origin/release", "", []string{"main"})
	if err != nil || name != "origin/release" || src != SourceFlag {
		t.Fatalf("got %q from %q, err=%v", name, src, err)
	}
}

func TestOriginHeadPrefersLocalTwin(t *testing.T) {
	name, src, err := Pick("", "origin/main", []string{"feature", "main"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "main" || src != SourceOriginHead {
		t.Fatalf("got %q from %q", name, src)
	}
}

func TestOriginHeadFallsBackToRemoteRef(t *testing.T) {
	// No local "trunk" exists, so comparisons run against origin/trunk itself.
	name, src, err := Pick("", "origin/trunk", []string{"feature-a", "feature-b"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "origin/trunk" || src != SourceOriginHead {
		t.Fatalf("got %q from %q", name, src)
	}
}

func TestOriginHeadWithSlashInBranchName(t *testing.T) {
	// Only the remote name is stripped; the branch may itself contain '/'.
	name, _, err := Pick("", "origin/release/2026", []string{"release/2026"})
	if err != nil || name != "release/2026" {
		t.Fatalf("got %q, err=%v", name, err)
	}
}

func TestWellKnownNamesTriedInOrder(t *testing.T) {
	// master outranks develop; main outranks master.
	name, src, err := Pick("", "", []string{"develop", "master"})
	if err != nil || name != "master" || src != SourceLocalName {
		t.Fatalf("got %q from %q, err=%v", name, src, err)
	}
	name, _, err = Pick("", "", []string{"develop", "master", "main"})
	if err != nil || name != "main" {
		t.Fatalf("got %q, err=%v", name, err)
	}
}

func TestTrunkAndDevelopAreRecognized(t *testing.T) {
	name, _, err := Pick("", "", []string{"trunk"})
	if err != nil || name != "trunk" {
		t.Fatalf("trunk: got %q, err=%v", name, err)
	}
	name, _, err = Pick("", "", []string{"develop", "topic"})
	if err != nil || name != "develop" {
		t.Fatalf("develop: got %q, err=%v", name, err)
	}
}

func TestNoBaseDetectableIsAnError(t *testing.T) {
	_, _, err := Pick("", "", []string{"feature-a", "feature-b"})
	if err == nil {
		t.Fatal("want error when nothing identifies a base")
	}
}
