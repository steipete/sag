---
title: Configuration
description: "Provider keys, default voices, timeouts, base URL, and player selection — everything sag reads from flags or the environment."
---

# Configuration

`sag` reads configuration from CLI flags first, then environment variables. There is no config file: the binary stays single-purpose and friendly to ephemeral CI runners.

## Provider key

Required for any TTS or voice call. `sag --help`, `sag prompting`, and `sag --version` work without one.

| Provider | Flag / variable | Notes |
| --- | --- |
| ElevenLabs | `--api-key` | Inline override. Avoid in shell history; prefer env or `--api-key-file`. |
| ElevenLabs | `ELEVENLABS_API_KEY` | Primary env var. |
| ElevenLabs | `SAG_API_KEY` | Accepted alias. |
| ElevenLabs | `--api-key-file <path>` | Read the key from a file. |
| ElevenLabs | `ELEVENLABS_API_KEY_FILE` | Same as `--api-key-file`. |
| ElevenLabs | `SAG_API_KEY_FILE` | Alias. |
| 60db | `SIXTYDB_API_KEY` | Primary env var. |
| 60db | `SIXTYDB_API_KEY_FILE` | Read the key from a file. |
| OpenAI | `OPENAI_API_KEY` | Primary env var; takes precedence over its key file. |
| OpenAI | `OPENAI_API_KEY_FILE` | Read the key from a file. |

Configure exactly one provider, or choose `--provider elevenlabs|60db|openai`. With explicit selection, only the selected provider's key sources are read. The legacy `--api-key` and `--api-key-file` flags remain ElevenLabs-only.

The file form is handy for agents and containers:

```bash
echo "$ELEVENLABS_API_KEY" > ~/.config/elevenlabs.key
chmod 600 ~/.config/elevenlabs.key
SAG_API_KEY_FILE=~/.config/elevenlabs.key sag voices --limit 3
```

## Default voice

When `--voice` / `--voice-id` is omitted, `sag` resolves in this order:

1. `ELEVENLABS_VOICE_ID`
2. `SAG_VOICE_ID`
3. The first voice returned by the active provider's voice listing (logged on stderr so you notice).

The env defaults apply only to ElevenLabs. With 60db, `sag` falls back to the first merged result from `/default-voices` and `/myvoices`.

```bash
export SAG_VOICE_ID=21m00Tcm4TlvDq8ikWAM
sag "Default voice locked in."
```

Pass `?` to force the voice list and exit:

```bash
sag -v ?
```

## Timeouts

`sag` ships with **no internal timeout** so that long v3 prompts don’t get truncated by a hidden SIGTERM. Decide for yourself:

| Source | Behaviour |
| --- | --- |
| `--timeout 5m` flag | Cancels the TTS request after the given Go duration. `0` keeps the parent context. |
| `SAG_TIMEOUT=5m` env | Same effect, set per shell or per CI job. |
| Outer process timeout | Use `timeout(1)` or your scheduler if you want a hard kill. |

The flag wins over the environment variable; both accept any `time.ParseDuration` string (`30s`, `2m`, `1h30m`).

```bash
SAG_TIMEOUT=10m sag --no-play -o long.mp3 "$(<chapter.txt)"
```

When sag is the bottleneck and the shell aborts the request, you’ll get a partial file. Use `ffprobe` to sanity-check duration before publishing.

## Player backend

| Value | Behaviour |
| --- | --- |
| `auto` (default) | `afplay` on macOS, `oto` everywhere else. |
| `afplay` | macOS only; routes through CoreAudio so AirPlay and Bluetooth zones work. |
| `oto` | Cross-platform MP3/WAV decoder + `oto`. |

Pick a backend explicitly via `--player oto` or `SAG_PLAYER=oto`. See [Streaming & playback](streaming.md) for trade-offs.

## API base URL

Override the active provider endpoint when you’re routing through a proxy or talking to a regional/staging deployment:

```bash
sag --base-url https://api.elevenlabs.io "Default."
sag --base-url https://your-proxy.internal "Routed."
```

The default is `https://api.elevenlabs.io` for ElevenLabs and `https://api.60db.ai` for 60db. There is no env var for this; it’s deliberate so the API target is always visible in the command line.

## Voice metadata cache

`sag voices --query` and `--label` need full voice descriptors. Metadata is cached in your platform-default cache directory for 24 hours. Delete the file if you need an immediate refresh.

## Compatibility flags (no-ops)

These are accepted for `say` parity and silently ignored:

`--progress`, `--audio-device`, `--network-send`, `--interactive`, `--file-format`, `--data-format`, `--channels`, `--bit-rate`, `--quality`.

If you rely on these in a script, sag won’t error. They simply have no effect because the synthesis happens server-side.

## Putting it together

A typical agent profile looks like this:

```bash
export ELEVENLABS_API_KEY_FILE=~/.config/elevenlabs.key
export SAG_VOICE_ID=21m00Tcm4TlvDq8ikWAM
export SAG_TIMEOUT=5m
export SAG_PLAYER=oto

sag --no-play -o "$artifact" "$prompt"
```

## Related pages

- [Quickstart](quickstart.md) — the minimal setup walkthrough.
- [Streaming & playback](streaming.md) — when to use which backend.
- [Output & formats](formats.md) — picking a codec and format string.
- [Models](models.md) — model-specific pricing and latency.
