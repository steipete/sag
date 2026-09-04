---
summary: 'Release checklist for sag (GitHub release + Homebrew tap)'
---

# Releasing sag

Follow these steps for each release. Title GitHub releases as `sag <version>`.

## Checklist
- Start from a clean, synchronized `main` checkout and confirm the target version follows the changelog/project SemVer convention.
- Update CLI version in `cmd/root.go` (`Version` field).
- Update `package.json` to the same version.
- Update `CHANGELOG.md` with a section for the new version.
- Run the gates: `pnpm format && pnpm lint && pnpm test && pnpm build`.
- Commit the release changes, then create an annotated tag: `git tag -a v<version> -m "Release <version>"`.
- Push `main` and the tag. The `Release Binaries` workflow builds macOS arm64/amd64/universal, Linux arm64/amd64, and Windows amd64 archives, verifies their checksums, attaches them to the GitHub release, dispatches the Homebrew tap update, and waits for that workflow.
- Keep the macOS deployment target and cgo compiler/linker flags pinned to macOS 15.0, matching the existing released minimum. The workflow verifies both architecture slices with `xcrun vtool -show-build` before packaging.
- Watch the exact tag workflow through completion. Repair or rerun failures before continuing.
- Verify the GitHub release:
  - title is `sag <version>`
  - body contains the complete changelog section plus links to the release commit, exact-head CI, live/behavior proof, and checksum manifest
  - all expected archives, per-archive checksums, and the combined checksum manifest are attached
- Verify `steipete/homebrew-tap` updated `Formula/sag.rb` to the new release URLs and matching checksums. The release workflow owns this update; use the [Homebrew playbook](releasing-homebrew.md) for recovery.
- Verify Homebrew install from tap:
  - `brew update && brew reinstall steipete/tap/sag`
  - `brew test steipete/tap/sag`
  - `sag --version`
- Smoke-test CLI locally: `sag --help`, `sag voices --limit 3`, `sag -v Roger "hello"`.
- Add the next patch `Unreleased` section to `CHANGELOG.md`, commit, and push the closeout.
- Announce: optional note with `brew update && brew upgrade steipete/tap/sag`.
