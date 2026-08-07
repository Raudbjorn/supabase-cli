# Fork Maintenance Guide

This is a maintainer's-eye summary of why this fork exists and how it stays in sync with upstream. The canonical, step-by-step sync procedure lives in `AGENTS.md` ("Updating this fork, rebuilding, and installing the Arch package") — this file intentionally does not duplicate it, to avoid the two drifting out of sync again.

## Why This Fork Exists

1. **`go-ethereum` replacement** — upstream depends on `github.com/ethereum/go-ethereum` for a single `secp256k1` curve function. That dependency is large, slow to build, and prone to checksum mismatches. `apps/cli-go/internal/go-ethereum-stub/` provides the same API backed by `decred/dcrd`'s implementation, wired in via a `replace` directive in `apps/cli-go/go.mod`.
2. **`containers/common` replacement** — same rationale, a second heavy dependency swapped for a lightweight stub at `apps/cli-go/internal/containers-common-stub/`.
3. **Functional `start` command** — upstream deleted its Docker-based `internal/start` implementation outright, on the assumption that a TypeScript CLI wraps the Go binary and talks to Docker directly instead. This package (`supabase-git` on the AUR) ships the bare Go binary as `/usr/bin/supabase` with no TypeScript wrapper, so `supabase start` must keep working without `--sandbox`. `apps/cli-go/internal/start/`, `apps/cli-go/cmd/start.go`, and `apps/cli-go/cmd/start_test.go` are therefore permanently fork-owned and restored from the fork's own history on every sync, regardless of whether git flags a conflict on them.
4. **No docker/cli dependency in `internal/utils/docker.go`** — the two functions that would otherwise need `docker/cli` (`NewDocker`, `loadRegistryAuth`) live in `apps/cli-go/internal/utils/docker_fork.go` instead, so the rest of `docker.go` stays byte-identical to upstream and merges without manual intervention.
5. **Some upstream CI workflows disabled** — `.github/dependabot.yml` is deleted from this fork's tree (re-deleted on every sync); a handful of upstream workflows not relevant to a personal AUR fork (automerge, deploy, deploy-check, release) are disabled at the GitHub repo level (Settings → Actions) rather than by editing/renaming the workflow files, so those files stay identical to upstream and merge cleanly.

## Sync Strategy: Merge, Not Rebase

This fork syncs via `git merge upstream-real/develop --no-edit`, **not** rebase. Merge is correct here because:

- The fork's own history already contains 15+ merge commits from prior syncs — a rebase would replay all of them against a moved upstream tip, conflicting repeatedly for no benefit.
- A force-push is never required, so there's no risk of clobbering work in progress on `origin/develop`.
- Fork-owned files (item 3 and 4 above) need deterministic post-merge restoration regardless of conflict status — a `git checkout <pre-merge-HEAD> -- <path>` after `git merge` handles this uniformly whether git flagged a conflict or silently deleted an untouched sibling file. A rebase, replaying many commits, would need this same fixup after every replayed commit that touches those paths, not just once.

Run the sync with `/home/svnbjrn/dev/supabase-git/update-fork.sh`, which automates the full merge + fork-owned-path restoration + `go.mod`/`go.sum` reconciliation + build/test gate described in `AGENTS.md`. It stops before commit/push/install so a human or agent can review the diff first.

## Historical Note

An earlier version of this fork used a daily automated GitHub Actions workflow (`sync-upstream.yml`) implementing a rebase-and-force-push strategy. That workflow was disabled early on and has since been deleted; it no longer reflects how this fork is maintained. If you find references to it elsewhere, they're stale.
