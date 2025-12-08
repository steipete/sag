# sag specification

CLI that mirrors macOS `say` but uses ElevenLabs for synthesis. Defaults to streaming directly to speakers and can also write audio files.

## Runtime & deps
- Go 1.22+
- Playback uses built-in Go audio (go-mp3 + oto) and should work on macOS/Linux/Windows with a default output device.
- Auth via `ELEVENLABS_API_KEY` (or `--api-key` flag).

## Commands

### `sag speak [text]`
- Text input: pass as args, `-f/--input-file` (use `-` for stdin), or pipe stdin.
- macOS `say` compatibility:
  - `-v/--voice` accepts voice **name** or ID; `?` lists voices.
  - `-r/--rate` words-per-minute (default 175) maps to ElevenLabs speed.
  - `-o/--output` same meaning; format inferred by extension when possible.
  - Accepts but ignores `--progress`, `--audio-device`, `--network-send`, `--interactive`, `--file-format`, `--data-format`, `--channels`, `--bit-rate`, `--quality`.
- Required: voice (via `-v/--voice` or `ELEVENLABS_VOICE_ID`/`SAG_VOICE_ID`).
- Flags:
  - `--model-id` (default `eleven_multilingual_v2`)
  - `--format` (default `mp3_44100_128`; `.wav` infers `pcm_44100`)
  - `--stream/--no-stream` (default stream)
  - `--latency-tier` (0-4, default 0)
  - `--play/--no-play` (default play)
  - `--speed` (0.5–2.0, default 1.0; >1.0 speaks faster)
  - `--output <path>` save audio while optionally playing
- Behavior:
  - Streaming path calls `POST /v1/text-to-speech/{voice_id}/stream` with JSON body.
  - Non-streaming path calls `POST /v1/text-to-speech/{voice_id}` and then plays/saves.
  - Errors if neither playback nor output is selected.

Usage examples:
```
sag speak --voice-id VOICE_ID "Hello world"
echo "piped input" | sag speak --voice-id VOICE_ID
sag speak --voice-id VOICE_ID --output out.mp3 --no-play
sag speak --voice-id VOICE_ID --speed 1.15 "Talk a bit faster"
sag speak --voice-id VOICE_ID --stream --latency-tier 3 "Faster start"
sag speak -v "Roger" -r 200 "mac say style flags"
```

### `sag voices`
- Lists voices via `GET /v1/voices` (server-side search when supported).
- Flags:
  - `--search <query>`: filter by name
  - `--limit <n>`: truncate output (default 100)

Sample:
```
sag voices --search "english"
```

## Config sources
- `ELEVENLABS_API_KEY` for auth (required).
- Default voice env: `ELEVENLABS_VOICE_ID` or `SAG_VOICE_ID`.
- `--base-url` flag for alternate API host (defaults to `https://api.elevenlabs.io`).

## Notes & future polish
- Add cross-platform playback backends.
- Persist defaults in a config file (voice/model/format).
- Add tests around flag parsing and error handling.
