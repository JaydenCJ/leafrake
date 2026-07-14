# leafrake examples

Both scripts are self-contained and fully offline.

## `make-messy-repo.sh`

Builds a demo repository with one branch of every category — merged,
squash-merged, gone (against a local bare "remote"), stale, and active —
so you can try every subcommand without risking a real repo:

```bash
bash examples/make-messy-repo.sh /tmp/leafrake-demo
leafrake scan /tmp/leafrake-demo/repo
leafrake explain feature/search /tmp/leafrake-demo/repo
leafrake clean --yes /tmp/leafrake-demo/repo
```

The squash-merged branch (`feature/search`) is the interesting one: run
`git -C /tmp/leafrake-demo/repo branch --merged main` and note that git
itself does not list it — leafrake proves it by patch-id instead.

## `weekly-tidy.sh`

A conservative maintenance routine for real repositories: `git fetch
--prune` first (so gone-upstream state is fresh), then a full evidence
scan, a dry run, and finally a deletion pass limited to the proven
categories. Point cron at it, or run it whenever `git branch` starts
scrolling:

```bash
bash examples/weekly-tidy.sh ~/work/my-project
```
