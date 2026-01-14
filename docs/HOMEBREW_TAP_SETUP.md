# Homebrew Tap Setup

This document describes how to set up the Homebrew tap repository for distributing devproxy.

## Repository Setup

1. **Create the tap repository**

   Create a new GitHub repository named `homebrew-tap` under the `munichmade` organization:
   
   ```
   https://github.com/munichmade/homebrew-tap
   ```

2. **Initialize the repository**

   ```bash
   git clone https://github.com/munichmade/homebrew-tap.git
   cd homebrew-tap
   mkdir Casks
   touch Casks/.gitkeep
   ```

3. **Add a README**

   Create `README.md`:

   ```markdown
   # MunichMade Homebrew Tap

   ## Installation

   ```bash
   brew tap munichmade/tap
   brew install devproxy
   ```

   ## Available Casks

   | Cask | Description |
   |------|-------------|
   | devproxy | Local development reverse proxy with automatic TLS and Docker integration |

   ## Updating

   ```bash
   brew update
   brew upgrade devproxy
   ```
   ```

4. **Commit and push**

   ```bash
   git add -A
   git commit -m "Initial tap setup"
   git push origin main
   ```

## Personal Access Token (PAT) Setup

GoReleaser needs write access to the tap repository to update the cask on release.

1. **Create a Fine-Grained PAT**

   Go to: https://github.com/settings/tokens?type=beta

   - **Token name:** `devproxy-homebrew-tap`
   - **Expiration:** Choose appropriate duration (e.g., 1 year)
   - **Repository access:** Select "Only select repositories" → `munichmade/homebrew-tap`
   - **Permissions:**
     - Contents: Read and write
     - Metadata: Read-only

2. **Add the secret to devproxy repository**

   Go to: https://github.com/munichmade/devproxy/settings/secrets/actions

   - Click "New repository secret"
   - **Name:** `HOMEBREW_TAP_TOKEN`
   - **Value:** Paste the PAT created above

## Testing Locally

Before creating a real release, you can test the GoReleaser configuration:

```bash
# Install goreleaser if not already installed
brew install goreleaser

# Validate the configuration
goreleaser check

# Build without releasing (creates binaries in dist/)
goreleaser build --snapshot --clean

# Full release simulation (no publish)
goreleaser release --snapshot --clean
```

The `--snapshot` flag:
- Doesn't require a git tag
- Uses `0.0.0-SNAPSHOT-<commit>` as version
- Skips publishing to GitHub and Homebrew

## Pre-release Versions

### Nightly Releases

Nightly releases are created automatically every day at 2:00 AM UTC if there are new commits on `main`. They use version tags like:

```
v0.1.0-nightly.20260114.abc1234
```

**Features:**
- Only created if there are new commits since the last nightly
- Keeps only the last 5 nightly releases (older ones are cleaned up)
- Does NOT update the Homebrew cask
- Can be triggered manually via workflow dispatch

**Installing a nightly release:**

Download directly from [GitHub Releases](https://github.com/munichmade/devproxy/releases) and look for tags containing `-nightly`.

### Manual Pre-releases

GoReleaser automatically detects pre-releases based on semantic versioning:

| Tag | Type | Homebrew Updated? |
|-----|------|-------------------|
| `v1.0.0` | Stable | Yes |
| `v1.1.0-nightly.*` | Nightly | No |
| `v1.1.0-alpha.1` | Pre-release | No |
| `v1.1.0-beta.1` | Pre-release | No |
| `v1.1.0-rc.1` | Pre-release | No |

Pre-releases are marked as such on GitHub Releases but don't update the Homebrew cask by default, ensuring users on stable don't accidentally get pre-release versions.

To install a pre-release via Homebrew, users can specify the version:

```bash
brew install munichmade/tap/devproxy@1.1.0-beta.1
```

Or download directly from GitHub Releases.

## Release Workflow

Once everything is set up, releasing is simple:

```bash
# 1. Update CHANGELOG.md
# 2. Commit changes
git add -A
git commit -m "chore: prepare release v1.0.0"

# 3. Create and push tag
git tag v1.0.0
git push origin main --tags
```

The GitHub Actions workflow will:
1. Build binaries for all platforms
2. Create archives with checksums
3. Publish GitHub Release
4. Update `Casks/devproxy.rb` in the tap repository

## Verifying the Release

After a release:

1. **Check GitHub Release**
   
   Visit: https://github.com/munichmade/devproxy/releases

2. **Check Homebrew cask was updated**
   
   Visit: https://github.com/munichmade/homebrew-tap/blob/main/Casks/devproxy.rb

3. **Test installation**

   ```bash
   brew update
   brew install munichmade/tap/devproxy
   devproxy --version
   ```

## Troubleshooting

### "Resource not accessible by integration"

The `HOMEBREW_TAP_TOKEN` secret is missing or doesn't have write access to the tap repository.

### Cask not updated after release

Check the GitHub Actions logs for the release workflow. Common issues:
- PAT expired
- PAT doesn't have write access to tap repository
- Tap repository doesn't exist

### Users getting old version

Users need to run `brew update` before `brew upgrade devproxy` to fetch the latest cask.
