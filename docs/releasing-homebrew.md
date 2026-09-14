# sag Homebrew Release Playbook

The `Release (unified)` workflow updates `steipete/homebrew-tap` after independent macOS verification and publication. Do not edit release URLs or checksums manually as a normal release step.

## Normal flow

1. Follow [Releasing sag](RELEASING.md) and dispatch `release-unified.yml` from `main` with `version=X.Y.Z`.
2. Watch the caller’s `homebrew` job after the shared release finishes. It passes the exact four verified architecture-specific archive names and SHA-256 values to `steipete/homebrew-tap`’s `update-formula.yml`.
3. Confirm the tap run succeeds. The caller also checks the resulting `Formula/sag.rb` against the verified inventory; dispatch success alone is insufficient.
4. Verify installation:

   ```sh
   brew update
   brew upgrade steipete/tap/sag
   brew test steipete/tap/sag
   sag --version
   codesign --verify --strict --check-notarization -R=notarized "$(brew --prefix sag)/bin/sag"
   ```

The formula selects native binaries for both macOS and Linux architectures. Linux ARM no longer builds from the tagged source archive. Keep the Linux ALSA runtime dependency and `skip_clean "bin/sag"` so Homebrew preserves the signed executable. Its platform/CPU branches must use literal release URLs and hashes, as required by the caller’s handoff verifier. The caller owns this stage because the v1.9.0 shared parser rejects OS-specific dependencies.

## Recovery

Fix the cause, then rerun the failed jobs in the exact release run. The shared workflow pins retries to the existing annotated tag; the caller downloads and verifies the published inventory before continuing a handoff. Never overwrite a tag or replace published assets. A missing signing identity normally means a missing or incorrectly mapped signing secret; inspect names and mappings without logging secret values.
