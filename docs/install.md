---
title: Install
description: "Install sag via Homebrew, prebuilt release binaries, or `go install`."
---

# Install

`sag` ships as a single executable. Pick the path that matches your platform.

## Homebrew (macOS, Linux)

```bash
brew install steipete/tap/sag   # auto-taps steipete/tap
sag --version
```

Upgrades:

```bash
brew update && brew upgrade steipete/tap/sag
```

## Prebuilt release binaries

Each release publishes archives for macOS (amd64/arm64/universal), Linux (amd64/arm64), and Windows (amd64). Browse <https://github.com/steipete/sag/releases/latest>, download the matching archive, extract, and put `sag` on your `PATH` (e.g. `/usr/local/bin`).

macOS archives require macOS 15 or later. Their executables are Developer ID signed by Peter Steinberger (team `Y5PE65HELJ`) and notarized by Apple, so direct downloads pass Gatekeeper. Choose `darwin_arm64` for Apple Silicon, `darwin_amd64` for Intel, or `universal_darwin_all` for both.

Archive names include the version: `sag_<version>_<os>_<arch>.tar.gz` (Windows uses `.zip`). Download the matching archive and `SHA256SUMS` from the same release, then verify before extracting. Substitute the release version below:

```bash
shasum -a 256 --check --ignore-missing SHA256SUMS
tar -xzf sag_<version>_darwin_arm64.tar.gz
./sag --version
```

Starting with 0.4.3, `SHA256SUMS` replaces the versioned checksum manifest and per-archive `.sha256` files; the universal archive is named `sag_<version>_universal_darwin_all.tar.gz`. Architecture-specific archive names are unchanged.

## Go toolchain

```bash
go install github.com/steipete/sag/cmd/sag@latest
```

Requires the Go version declared in `go.mod` (1.25+). Source builds use the version recorded in the source.

## From source

```bash
git clone https://github.com/steipete/sag.git
cd sag
go build ./cmd/sag
./sag --version
```

## Linux build prerequisites

The cross-platform `oto` audio backend needs ALSA development headers when building from source on Debian/Ubuntu:

```bash
sudo apt install build-essential pkg-config libasound2-dev
```

Released Linux binaries need the ALSA runtime library (`libasound2t64` on Ubuntu 24.04, or `libasound2` on older Debian/Ubuntu). Development headers are only required when you compile yourself.

## Verify the install

```bash
sag --version
sag --help
sag prompting   # works without an API key
```

A live API call (any TTS or `sag voices`) needs `ELEVENLABS_API_KEY` set; see [Configuration](configuration.md).

## Updating

- **Homebrew:** `brew upgrade steipete/tap/sag`.
- **Prebuilt archives:** download the new tarball/ZIP and replace the binary.
- **`go install`:** rerun `go install github.com/steipete/sag/cmd/sag@latest`.
- **Source builds:** `git pull && go build ./cmd/sag`.

## Related pages

- [Quickstart](quickstart.md) — first speech in under a minute.
- [Configuration](configuration.md) — API key, env vars, default voice, timeouts.
- [Releasing](RELEASING.md) — the maintainer flow for cutting a new version.
