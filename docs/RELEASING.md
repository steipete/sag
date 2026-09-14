---
summary: 'Release checklist for sag (signed GitHub release + Homebrew tap)'
---

# Releasing sag

The manually dispatched `Release (unified)` workflow calls the shared [Go CLI archetype](https://github.com/openclaw/release-workflows) at v1.9.0, commit `f613cbfed2b043159c850c353e7facb8c89833b0`. It freezes protected `main`, creates an annotated version tag, builds, Developer ID signs and notarizes macOS binaries, and publishes only after independent Apple Silicon and Intel verification. Pushing a tag alone does not start a release.

## Repository setup

Protect `main` and require the CI jobs `docs-build`, `lint-test`, `macos-release-build`, and `linux-release-build`. Actions workflow permissions must allow writes and creation of pull requests for the closeout PR; each workflow declares its own permissions.

The caller uses `repository-type: personal`: `Developer ID Application: Peter Steinberger (Y5PE65HELJ)`, with stable identifier `com.steipete.sag.sag`. Configure repository secrets `MACOS_SIGN_P12`, `MACOS_SIGN_P12_PASSWORD`, `ASC_KEY_ID`, `ASC_ISSUER_ID`, `ASC_PRIVATE_KEY`, and `HOMEBREW_TAP_TOKEN`. The caller maps these to the shared workflow’s signing, App Store Connect, and tap inputs. The tap token must permit Contents read and Actions read/write on `steipete/homebrew-tap` (plus public read access to sag).

## Prepare and publish

1. Start from clean, synchronized `main`. Check `gh release list --limit 3 --json tagName` and `git tag` before choosing the next version.
2. Update the version in `cmd/root.go` and `package.json`. Finalize the matching changelog section as `## X.Y.Z - YYYY-MM-DD`, including Highlights and all release changes.
3. Run `pnpm check`, `pnpm docs:build`, `actionlint`, `goreleaser check`, `goreleaser check --config .goreleaser-linux-windows.yaml`, and `goreleaser release --snapshot --clean --skip=publish` on macOS. `scripts/check-release-metadata` checks the CLI, package manifest, and latest dated changelog. CI also builds the complete Linux/Windows matrix on Ubuntu.
4. Review and merge the release PR, then wait for all CI on the exact merged commit. Recheck that the proposed tag and release do not already exist.
5. Dispatch from the current default-branch head, using the version without `v`:

   ```sh
   gh workflow run release-unified.yml --repo steipete/sag --ref main -f version=X.Y.Z
   ```

6. Watch that exact run through publication, Homebrew handoff, and closeout. The workflow owns annotated tag creation; never move or replace a release tag. If a job fails, repair the cause and rerun the failed jobs. A retry uses the existing frozen tag even if `main` has advanced.

## Build and artifact contracts

`.goreleaser.yaml` builds both Darwin architectures on macOS. Keep `MACOSX_DEPLOYMENT_TARGET`, `CGO_CFLAGS`, and `CGO_LDFLAGS` pinned to macOS 15.0. Darwin uses external linking so the system linker honors that minimum; the explicit compiler/linker flags also enter Go’s cgo cache key. Every build runs `scripts/check-macos-target`, which requires `otool -l` to report `LC_BUILD_VERSION minos 15.0` before signing.

`.goreleaser-linux-windows.yaml` builds Linux amd64/arm64 and Windows amd64 on Ubuntu. The credential-free build hook installs both ALSA development architectures and the arm64 cross-compiler from Ubuntu repositories. Windows does not use cgo. Linux executables dynamically link the system ALSA runtime.

Native archives retain `sag_<version>_<os>_<arch>.tar.gz` (Windows: `.zip`). The shared workflow emits `sag_<version>_universal_darwin_all.tar.gz` for universal macOS, replacing the former `darwin_universal` suffix. Every archive includes `sag` (or `sag.exe`), `README.md`, and `LICENSE`. `SHA256SUMS` replaces the old versioned manifest and per-archive `.sha256` assets.

The release includes `ASSET-INVENTORY.json` and `RELEASE-NOTES.md`. The inventory binds payload names, hashes, platforms, and source commit; the manifest covers all payloads and the inventory. Both macOS verification jobs independently check those bytes, signing identity, hardened runtime, notarization, and native execution. The publisher downloads the draft assets again and requires exact agreement before publishing. The published body must equal the frozen changelog section and `RELEASE-NOTES.md` byte-for-byte; put any intended release-note content in the changelog before dispatch.

## Verify and close out

Download the published archives afresh and verify `SHA256SUMS`. On macOS, inspect the extracted executable:

```sh
codesign -dvv ./sag
codesign --verify --deep --strict --verbose=4 ./sag
codesign --verify --strict --check-notarization -R=notarized ./sag
spctl -a -vv -t open --context context:primary-signature ./sag
otool -l ./sag | grep -A3 LC_BUILD_VERSION
./sag --version
```

Require Peter’s Developer ID and team, an accepted notarization ticket, and minimum macOS 15.0. Test with `com.apple.quarantine` present on the extracted binary; command-line `curl` may not set quarantine automatically, so explicitly apply it for this check. Raw CLI binaries cannot carry stapled tickets; the online notarization check is the ticket proof.

Verify `go list -m github.com/steipete/sag@vX.Y.Z` and the [Homebrew handoff](releasing-homebrew.md). The caller runs that job after the shared release completes: v1.9.0’s built-in formula parser cannot accept sag’s Linux-only ALSA/patchelf dependencies. The caller downloads and verifies the published inventory and checksums, dispatches exact assets, waits for the uniquely identified tap run, and verifies each resulting platform/CPU URL and hash. The shared closeout PR can appear before this final tap check; merge it only after both succeed. After all verification, review the workflow’s closeout PR, rename its section to `## X.Y.(Z+1) - Unreleased` to preserve house style, and merge it. Leave `main` clean and synchronized with `origin/main`.
