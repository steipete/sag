# sag specification

CLI that mirrors macOS `say` but uses MiniMax for synthesis. Defaults to streaming directly to speakers and can also write audio files.

## Runtime & deps
- Go 1.24+
- Playback uses built-in Go audio (go-mp3 + oto) and should work on macOS/Linux/Windows with a default output device.
- Auth via `MINIMAX_API_KEY` (or `--api-key` flag).

## Commands

### `sag speak [text]`
- Text input: pass as args, `-f/--input-file` (use `-` for stdin), or pipe stdin.
- macOS `say` compatibility:
  - `-v/--voice` accepts voice **name** or ID; `?` lists voices.
  - `--voice-category` restricts voice lookup to a MiniMax category (`system|voice_cloning|voice_generation|all`).
  - `-r/--rate` words-per-minute (default 175) maps to speed.
  - `-o/--output` same meaning; format inferred by extension when possible.
  - Accepts but ignores `--progress`, `--audio-device`, `--network-send`, `--interactive`, `--file-format`, `--data-format`, `--channels`, `--bit-rate`, `--quality`.
- Required: voice (via `-v/--voice` or `MINIMAX_VOICE_ID`/`SAG_VOICE_ID`).
- Flags:
  - `--model-id` (default `speech-01`)
  - `--format` (default `mp3_44100_128`; `.wav` infers `wav`)
  - `--stream/--no-stream` (default stream)
  - `--play/--no-play` (default play)
  - `--speed` (0.5–2.0, default 1.0; >1.0 speaks faster)
  - `--emotion` (model dependent)
  - `--pitch` (model dependent)
  - `--volume` (model dependent)
  - `--normalize` (`auto|on|off`; auto = server default)
  - `--lang` (language boost hint; when set)
  - `--metrics` print basic stats to stderr
  - `--output <path>` save audio while optionally playing
- Behavior:
  - Streaming path calls `POST /v1/t2a_v2` with `stream=true`.
  - Non-streaming path calls `POST /v1/t2a_v2` with `stream=false` and then plays/saves.
  - Errors if neither playback nor output is selected.
  - Playback supports MP3 output only.

Usage examples:
```
sag speak --voice-id VOICE_ID "Hello world"
echo "piped input" | sag speak --voice-id VOICE_ID
sag speak --voice-id VOICE_ID --output out.mp3 --no-play
sag speak --voice-id VOICE_ID --speed 1.15 "Talk a bit faster"
sag speak -v "Roger" -r 200 "mac say style flags"
```

### `sag voices`
- Lists voices via `POST /v1/get_voice`.
- Flags:
  - `--search <query>`: filter by name/id
  - `--limit <n>`: truncate output (default 100)
  - `--category`: server-side voice category (`system|voice_cloning|voice_generation|all`; default all)

Sample:
```
sag voices --search "english"
```

### `sag prompting`
- Prints a practical prompting guide (model-specific tips, tags, and suggested flags).
- Does not require an API key.

## Config sources
- `MINIMAX_API_KEY` for auth (required; fallback `SAG_API_KEY`).
- Default voice env: `MINIMAX_VOICE_ID` or `SAG_VOICE_ID`.
- `--base-url` flag for alternate API host (defaults to `https://api.minimax.io`).

## Notes & future polish
- Add cross-platform playback backends.
- Persist defaults in a config file (voice/model/format).
- Add tests around flag parsing and error handling.
