# sag 🗣️ — “Mac-style speech with MiniMax”

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
Requires Go 1.24+.

## Configuration
- `MINIMAX_API_KEY` (required; fallback `SAG_API_KEY`)
- `--api-key-file` or `MINIMAX_API_KEY_FILE`/`SAG_API_KEY_FILE` to load the key from a file
- Optional defaults: `MINIMAX_VOICE_ID` or `SAG_VOICE_ID`
- `--base-url` to override the API host (default `https://api.minimax.io`; some regions use `https://api-uw.minimax.io`)

## Usage

Features:
- macOS `say`-style default: `sag "Hello"` routes to `speak` automatically.
- Streaming playback to speakers with optional file output.
- Voice discovery via `sag voices` and `-v ?`.
- Speed/rate controls and format inference from output extension.
- Model selection via `--model-id` (defaults to `speech-01`).

Speak (streams audio):
```bash
sag speak -v "Your Voice" "Hello world"
```

Call it like macOS `say`: omitting the subcommand pipes text to `speak` by default.
```bash
sag "Hello world"
```

macOS `say` compatibility shortcuts (subcommand optional):
```bash
sag -v "Your Voice" -r 200 "Faster speech"
sag -o out.mp3 "Save to file"
sag -v ?      # list voices
```

More examples:
```bash
echo "piped input" | sag speak -v "Your Voice"
sag speak -v "Your Voice" --speed 1.2 "Talk a bit faster"
sag speak -v "Your Voice" --emotion happy "Great news, everyone!"
sag speak -v "Your Voice" --output out.wav --format wav "Wave output"
```

Key flags (subset):
- `-v, --voice` voice name or ID (`?` to list)
- `--voice-category` MiniMax voice category (`system|voice_cloning|voice_generation|all`)
- `--api-key-file` read API key from a file
- `-r, --rate` words per minute (maps to speed; default 175)
- `-f, --input-file` read text from file (`-` for stdin)
- `-o, --output` write audio file; format inferred by extension (`.wav` -> WAV, `.mp3` -> MP3)
- `--speed` explicit speed multiplier (0.5–2.0)
- `--emotion` (model dependent)
- `--pitch` (model dependent)
- `--volume` (model dependent)
- `--normalize` `auto|on|off` (auto uses server default text normalization)
- `--lang` language boost hint (e.g. `en`, `zh`, `auto`)
- `--format` output format (e.g. `mp3_44100_128`, `mp3`, `wav`)
- `--stream/--no-stream` stream while generating (default on)
- `--play/--no-play` control speaker playback
- `--metrics` print basic stats to stderr

Voices:
```bash
sag voices --category system --search english --limit 20
```

## Prompting (make it sound better)
Run:
```bash
sag prompting
```

Highlights:
- Keep scripts short and readable; punctuation drives timing.
- Use `--emotion`, `--pitch`, and `--volume` for tone shaping (model dependent).
- `--normalize` and `--lang` help with numbers/units and multilingual output.

## Models / engines

`sag` supports any MiniMax model ID via `--model-id` (we pass it through). Default is `speech-01`.

Notes:
- Model availability varies by account/region; see MiniMax docs for the current list.
- If you hit length limits, chunk text and stitch audio.

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
- MiniMax account and API key required.
- Voice defaults to first available if not provided.
- Non-mac platforms: playback still works via `go-mp3` + `oto`, but device selection flags are no-ops.
- Playback only supports MP3 output; use `--no-play` for WAV/FLAC/PCM.
