# Fork Maintenance Guide

This fork of Supabase CLI replaces the heavy `go-ethereum` dependency with a lightweight stub that redirects to decred's optimized secp256k1 implementation.

## Why This Fork Exists

**Problem:** go-ethereum is a massive dependency that:
- Significantly increases build time
- Causes frequent checksum mismatch errors
- Adds unnecessary bloat for a single crypto function

**Solution:** A lightweight internal stub at `internal/go-ethereum-stub/` that provides the same `crypto/secp256k1` API but uses decred's implementation, which:
- Supports modern CPU features (AVX-512)
- Is actively maintained and optimized
- Has a much smaller footprint

## Automated Syncing

The fork automatically syncs with upstream daily at 3 AM UTC via GitHub Actions.

### How It Works

1. **Rebase Strategy**: Uses `git rebase` (not merge) to maintain a clean linear history
2. **Smart Conflict Resolution**: Automatically handles expected conflicts in `go.mod`/`go.sum`
3. **Verification**: Ensures the ethereum stub remains intact after sync
4. **Testing**: Runs build tests before pushing
5. **Notifications**: Creates an issue if manual intervention is needed

### Manual Trigger

You can manually trigger a sync from the GitHub Actions tab:
1. Go to "Actions" → "Sync Fork with Upstream (Rebase)"
2. Click "Run workflow" → "Run workflow"

## Manual Syncing

If you need to sync manually:

```bash
# Clone your fork
git clone https://github.com/Raudbjorn/supabase-cli.git
cd supabase-cli
git checkout develop

# Add upstream remote (one-time)
git remote add upstream https://github.com/supabase/cli.git

# Fetch latest upstream changes
git fetch upstream --tags

# Rebase on upstream (preferred over merge for clean history)
git rebase upstream/develop

# If conflicts occur (usually in go.mod/go.sum):
# Keep our ethereum replacement
git checkout --ours go.mod
git add go.mod go.sum
git rebase --continue

# Verify the stub is intact
test -d internal/go-ethereum-stub && echo "✓ Stub intact"
grep "replace github.com/ethereum/go-ethereum" go.mod && echo "✓ Replace directive intact"

# Test build
export GOSUMDB=off CGO_ENABLED=0
go mod tidy
go build ./...

# Push with force-with-lease (safe force push)
git push --force-with-lease origin develop
```

## Why Rebase Instead of Merge?

**Rebase is better for this use case because:**

1. **Clean History**: Single linear commit history, no merge commits
   ```
   # With Rebase (clean):
   A---B---C---D (our ethereum fix)

   # With Merge (messy):
   A---B---C---M---M---M (merge commits)
   ```

2. **Easier to Review**: Each commit shows exactly what changed
3. **Simpler Conflicts**: Our changes are minimal (one stub + go.mod replace)
4. **Bisect-Friendly**: `git bisect` works better with linear history

**When Merge is Better:**
- Feature branches with many collaborators
- Complex branching strategies
- When you want to preserve exact branch history

**Our Case:** We have a simple, persistent change (ethereum stub) that needs to stay on top of upstream. Rebase is perfect here.

## Expected Conflicts

During sync, you may see conflicts in:
- `go.mod`: Our `replace` directive vs upstream's dependency updates
- `go.sum`: Checksum changes from dependency updates

**Resolution:** Always keep our version (`--ours`) for the ethereum replacement, accept upstream for everything else.

## Monitoring

The workflow will:
- ✅ Run daily automatically
- ✅ Create an issue if sync fails
- ✅ Provide detailed summary in Actions tab
- ✅ Force-push only when changes are successfully rebased and tested

## Testing After Sync

The automated workflow runs:
```bash
go mod tidy
go build -v ./...
```

For comprehensive testing:
```bash
# Run tests
go test -short ./...

# Build with optimizations (as PKGBUILD does)
export GOAMD64=v4
export GOMAXPROCS=64
go build -v -p=64 \
  -ldflags="-s -w" \
  -o supabase .

# Test the binary
./supabase --version
```

## Keeping PKGBUILD Updated

After successful upstream sync, update your PKGBUILD:

```bash
cd ~/dev/supabase-git
rm -rf src/ pkg/

# Build will automatically fetch latest from your fork
makepkg -sf --noconfirm
```

The PKGBUILD's `pkgver()` function will automatically detect the new version.

## Troubleshooting

### Workflow Failed with Conflicts

Check the Actions run for details, then manually sync:
```bash
git fetch upstream
git rebase upstream/develop
# Resolve conflicts
git push --force-with-lease origin develop
```

### Ethereum Stub Missing After Sync

Re-create it:
```bash
mkdir -p internal/go-ethereum-stub/crypto/secp256k1

cat > internal/go-ethereum-stub/go.mod <<'EOF'
module github.com/ethereum/go-ethereum
go 1.21
require github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0
EOF

cat > internal/go-ethereum-stub/crypto/secp256k1/curve.go <<'EOF'
package secp256k1
import (
	"crypto/elliptic"
	dcrdSecp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
)
func S256() elliptic.Curve {
	return dcrdSecp256k1.S256()
}
EOF

# Add replace directive to go.mod
echo "replace github.com/ethereum/go-ethereum v1.15.8 => ./internal/go-ethereum-stub" >> go.mod

git add internal/go-ethereum-stub go.mod
git commit -m "Restore ethereum stub"
```

### Build Fails After Sync

```bash
# Clean and retry
rm -rf ~/go/pkg/mod/github.com/ethereum/go-ethereum*
export GOSUMDB=off
go clean -modcache
go mod tidy
go build ./...
```

## Contact

For issues with:
- **The fork itself**: Open issue at `Raudbjorn/supabase-cli`
- **Upstream Supabase**: Open issue at `supabase/cli`
- **PKGBUILD**: Check your local repository

## License

This fork maintains the same MIT license as upstream Supabase CLI.
