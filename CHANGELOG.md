# Changelog

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
