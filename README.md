# sag 🗣️ — “Mac-style speech with ElevenLabs”

One-liner TTS that works like `say`: stream to speakers by default, list voices, or save audio files.

## Install
Homebrew (macOS):
```bash
brew install steipete/tap/sag  # auto-taps steipete/tap
```

Go toolchain:
```bash
go install ./cmd/sag
```
Requires Go 1.22+.

## Configuration
- `ELEVENLABS_API_KEY` (required)
- Optional defaults: `ELEVENLABS_VOICE_ID` or `SAG_VOICE_ID`

## Usage

Features:
- macOS `say`-style default: `sag "Hello"` routes to `speak` automatically.
- Streaming playback to speakers with optional file output.
- Voice discovery via `sag voices` and `-v ?`.
- Speed/rate controls, latency tiers, and format inference from output extension.
- Model selection via `--model-id` (defaults to `eleven_v3`; use `eleven_multilingual_v2` for a stable baseline).

Speak (streams audio):
```bash
sag speak -v Roger "Hello world"
```

Call it like macOS `say`: omitting the subcommand pipes text to `speak` by default.
```bash
sag "Hello world"
```

macOS `say` compatibility shortcuts (subcommand optional):
```bash
sag -v Roger -r 200 "Faster speech"
sag -o out.mp3 "Save to file"
sag -v ?      # list voices
```

More examples:
```bash
echo "piped input" | sag speak -v Roger
sag speak -v Roger --stream --latency-tier 3 "Faster start"
sag speak -v Roger --speed 1.2 "Talk a bit faster"
sag speak -v Roger --model-id eleven_multilingual_v2 "Use stable v2 baseline"
sag speak -v Roger --output out.wav --format pcm_44100 "Wave output"
```

Key flags (subset):
- `-v, --voice` voice name or ID (`?` to list)
- `-r, --rate` words per minute (maps to ElevenLabs speed; default 175)
- `-f, --input-file` read text from file (`-` for stdin)
- `-o, --output` write audio file; format inferred by extension (`.wav` -> PCM, `.mp3` -> MP3)
- `--speed` explicit speed multiplier (0.5–2.0)
- `--stability` v3: `0|0.5|1` (Creative/Natural/Robust); v2/v2.5: 0..1 (higher = more consistent, less expressive)
- `--similarity` / `--similarity-boost` 0..1 (higher = closer to the reference voice)
- `--style` 0..1 (higher = more stylized delivery; model/voice dependent)
- `--speaker-boost` / `--no-speaker-boost` toggle clarity boost (model dependent)
- `--seed` 0..4294967295 best-effort repeatability across runs
- `--normalize` `auto|on|off` numbers/units/URLs normalization (when set)
- `--lang` `en|de|fr|...` 2-letter ISO 639-1 language code (when set)
- `--stream/--no-stream` stream while generating (default on)
- `--latency-tier` 0–4 lower latency tiers
- `--play/--no-play` control speaker playback
- `--metrics` print basic stats to stderr

Voices:
```bash
sag voices --search english --limit 20
```

## Prompting (make it sound better)
Run:
```bash
sag prompting
```

Highlights:
- v2/v2.5: SSML pauses via `<break time="1.5s" />` (v3 does not support SSML breaks).
- v3: use audio tags like `[whispers]` and pause tags like `[short pause]`.
- Use the voice knobs: `--stability`, `--similarity`, `--style`, `--speaker-boost`, plus request controls `--seed`, `--normalize`, `--lang`.

## Models / engines

`sag` supports any ElevenLabs `model_id` via `--model-id` (we pass it through). Practical defaults + common IDs:

| Engine | `--model-id` | Prompting style | Best for |
|---|---|---|---|
| v3 (alpha) | `eleven_v3` (default) | Audio tags like `[whispers]`, `[short pause]` (no SSML `<break>`) | Most expressive / “acting” |
| v2 (stable) | `eleven_multilingual_v2` | SSML `<break>` supported | Reliable baseline, simple prompts |
| v2.5 Flash | `eleven_flash_v2_5` | SSML `<break>` supported | Ultra-low latency (~75ms) + 50% lower price per character |
| v2.5 Turbo | `eleven_turbo_v2_5` | SSML `<break>` supported | Low latency (~250–300ms) + 50% lower price per character |

Notes:
- SSML `<break>` works on v2/v2.5, not v3. Use pause tags on v3 instead.
- Input limits differ by engine (v3: 5,000 chars; v2: 10,000 chars; v2.5 Turbo/Flash: 40,000 chars). If you hit limits, chunk text and stitch audio.
- `--normalize on` may not be available for v2.5 Turbo/Flash (higher latency); prefer `auto`/`off` if it errors.
- Source of truth: ElevenLabs “Models” docs.

## Development
- With pnpm:
  - `pnpm format`
  - `pnpm lint`
  - `pnpm test`
  - `pnpm build`
  - `pnpm sag -- --help` (passes args to the Go binary)
- Direct Go:
  - Format: `go fmt ./...`
  - Lint: `golangci-lint run`
  - Tests: `go test ./...`
  - Build: `go build ./cmd/sag`

## Limitations
- ElevenLabs account and API key required.
- Voice defaults to first available if not provided.
- Non-mac platforms: playback still works via `go-mp3` + `oto`, but device selection flags are no-ops.
