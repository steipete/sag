# sag 🗣️ — “Mac-style speech with ElevenLabs or 60db”

One-liner TTS that works like `say`: stream to speakers by default, list voices, or save audio files.

## Install
Homebrew (macOS):
```bash
brew install steipete/tap/sag  # auto-taps steipete/tap
```

Prebuilt binaries:
- Download Linux, macOS, and Windows archives from the [latest GitHub release](https://github.com/steipete/sag/releases/latest).
- macOS downloads require macOS 15 or later and are Developer ID signed and notarized by Peter Steinberger, so direct downloads pass Gatekeeper.
- On Linux, unpack the `linux_amd64` archive and place `sag` somewhere on your `PATH`, for example `/usr/local/bin`.

Go toolchain:
```bash
go install github.com/steipete/sag/cmd/sag@latest
```
Requires Go 1.25+.

Debian/Ubuntu source build prerequisites:
```bash
sudo apt install build-essential pkg-config libasound2-dev
```

## Configuration

`sag` supports two TTS providers and auto-selects one from your configured credentials:

- **ElevenLabs** — `ELEVENLABS_API_KEY` (or `--api-key`, or `--api-key-file` / `ELEVENLABS_API_KEY_FILE` / `SAG_API_KEY_FILE`)
- **60db** (`api.60db.ai`) — `SIXTYDB_API_KEY` (or `SIXTYDB_API_KEY_FILE`)

Selection rules:
- Only one key set → that provider is used.
- Both keys set → error; unset one provider key and retry.
- Neither set → error.

Optional ElevenLabs defaults: `ELEVENLABS_VOICE_ID` or `SAG_VOICE_ID`. Override the active provider host with `--base-url`.

60db support is intentionally narrow and follows the live API behavior:
- voice discovery merges `GET /default-voices` and `GET /myvoices`
- synthesis uses `POST /tts-synthesize`, validates every NDJSON frame, and wraps the returned 48 kHz PCM as WAV
- incomplete or malformed provider responses fail instead of producing a truncated file
- 60db currently supports WAV playback/output only; its live synthesis response is buffered rather than streamed

Shared speak flags that work on both providers include `--voice`, `--speed` / `--rate`, `--stability`, `--similarity`, `--play`, `--output`, `--timeout`, and `--metrics`. On 60db, `--format` accepts only `wav`, and an explicit `--stream` is rejected.

ElevenLabs-only speak flags fail fast on 60db: `--model-id`, `--style`, `--speaker-boost` / `--no-speaker-boost`, `--seed`, `--normalize`, `--lang`, and `--latency-tier`. `--stability` / `--similarity` still use the CLI's `0..1` range and are scaled to 60db's documented `0..100` API values. See [docs/providers.md](docs/providers.md) for provider-specific details.

## Usage

Features:
- macOS `say`-style default: `sag "Hello"` routes to `speak` automatically.
- Streaming playback to speakers with optional file output.
- Voice discovery via `sag voices` and `-v ?`.
- Speed/rate controls, latency tiers, and format inference from output extension.
- Model selection via `--model-id` (defaults to `eleven_v3`; use `eleven_multilingual_v2` for a stable baseline).

Speak with ElevenLabs streaming:
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
SIXTYDB_API_KEY=... sag speak -v Aria --output out.wav --no-play "60db WAV output"
```

Key flags (subset):
- `-v, --voice` voice name or ID (`?` to list)
- `--api-key-file` read API key from a file
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
- `--player auto|afplay|oto` choose playback backend (`auto` uses `afplay` on macOS and `oto` elsewhere; also configurable via `SAG_PLAYER`)
- `--timeout` maximum TTS generation time, e.g. `--timeout 5m`; `0` means no internal timeout (default, also configurable via `SAG_TIMEOUT`)
- `--metrics` print basic stats to stderr

Voices:
```bash
sag voices --search english --limit 20
sag voices --search english --limit 5 --try
sag voices --query "crazy scientist" --limit 5 --try
sag voices --label accent=british --label use_case=character --limit 10
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

## Timeout considerations

Longer text can take more than a minute to generate, especially with `eleven_v3`. By default, `sag` does not add its own generation timeout, so callers such as agents, scripts, shells, or CI jobs can choose the right outer timeout without getting a valid-but-truncated audio file from an internal SIGTERM.

If you want `sag` itself to stop a request, set an explicit duration:
```bash
sag --timeout 5m --no-play -o long.mp3 "Long text..."
SAG_TIMEOUT=5m sag --no-play -o long.mp3 "Long text..."
```

For long agent/script runs, give the outer process timeout enough headroom and verify generated audio duration when truncation would matter:
```bash
ffprobe -v quiet -show_entries format=duration -of csv=p=0 long.mp3
```

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
- One provider API key is required.
- Voice defaults to first available if not provided.
- Non-mac platforms: playback still works via MP3/WAV decoding + `oto`, but device selection flags are no-ops.
