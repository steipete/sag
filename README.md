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
sag speak -v Roger --output out.wav --format pcm_44100 "Wave output"
```

Key flags (subset):
- `-v, --voice` voice name or ID (`?` to list)
- `-r, --rate` words per minute (maps to ElevenLabs speed; default 175)
- `-f, --input-file` read text from file (`-` for stdin)
- `-o, --output` write audio file; format inferred by extension (`.wav` -> PCM, `.mp3` -> MP3)
- `--speed` explicit speed multiplier (0.5–2.0)
- `--stream/--no-stream` stream while generating (default on)
- `--latency-tier` 0–4 lower latency tiers
- `--play/--no-play` control speaker playback

Voices:
```bash
sag voices --search english --limit 20
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
- ElevenLabs account and API key required.
- Voice defaults to first available if not provided.
- Non-mac platforms: playback still works via `go-mp3` + `oto`, but device selection flags are no-ops.
