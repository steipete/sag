# Changelog

## 0.2.1 - 2025-12-19
### Changed
- Default model is now `eleven_v3` (override with `--model-id eleven_multilingual_v2` for a stable baseline).

## 0.2.0 - 2025-12-19
### Added
- Voice control flags: `--stability`, `--similarity`/`--similarity-boost`, `--style`, `--speaker-boost`/`--no-speaker-boost`.
- Request controls: `--seed`, `--normalize (auto|on|off)`, `--lang` (ISO 639-1).
- `--metrics` to print basic request stats (chars/bytes/duration) to stderr.
- `sag prompting` command and README section with prompting tips.

## 0.1.1 - 2025-12-19
### Changed
- Release metadata only (patch bump).

## 0.1.0 - 2025-12-08
### Added
- Initial release of `sag` ElevenLabs TTS CLI with macOS `say`-style flags.
- Streaming default playback to speakers with optional file output; cross-platform audio via go-mp3 + oto.
- Voice listing (`sag voices`) and voice resolution by name/ID, including `?` to list.
- macOS `say` compatibility: `-v/--voice`, `-r/--rate`, `-f/--input-file`, `-o/--output`, plus accepted no-op flags.
- Auto-routing bare `sag ...` text/flags to `speak` subcommand, including npm/pnpm `--` pass-through support.
- Speed control (`--speed` or `--rate`), latency tier selection, model/format overrides with extension inference.
- Default voice fallback to first available when none provided; env support `ELEVENLABS_VOICE_ID` or `SAG_VOICE_ID`.
- Config via `ELEVENLABS_API_KEY`/`SAG_API_KEY`; `--api-key` and `--base-url` overrides.
- Tests for format inference, text sourcing, voice resolution helpers, and default `speak` routing behavior.
- Help/README improvements: feature overview, examples for subcommand-less usage, and voice discovery guidance.
- Homebrew tap formula (`brew install steipete/tap/sag`) and release playbook.
- CI workflow (lint + tests) and golangci-lint config.
- Documentation: README, docs/spec.md, and usage examples.
- Version flag (`--version` / `-V`) reporting 0.1.0; help available without API key.
