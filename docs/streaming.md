---
title: Streaming & playback
description: "How sag streams audio while it generates, picks a player backend, and exposes latency tiers."
---

# Streaming & playback

`sag` requests streamed ElevenLabs audio by default; playback latency depends on the selected backend. OpenAI streams PCM through Oto by default. 60db synthesis is buffered because its live response must be fully validated and wrapped as WAV before playback.

## OpenAI streaming

OpenAI audio chunks feed Oto as complete PCM16 frames while the response is still
being generated. `--player auto` selects this path for OpenAI; `--player oto` makes
it explicit. Cancellation and player failures close the audio pipe and stop the
WebSocket request.

Use `--no-stream` to buffer before playback, or select `--player afplay` for
buffered macOS routing. `SAG_PLAYER=afplay` is honored. Explicitly combining
`--stream` with `afplay` is an error because that backend needs a complete file.

`-o voice.wav` saves a validated complete response. Add `--play` to listen while
it generates; the file is written only after synthesis and playback succeed.
Failed generation or playback preserves an existing output file.

The final narration transcript is compared with the input after generation.
Warnings flag possible omissions or rewrites without printing the text. Streaming
can play changed words before the check finishes; use `--no-stream` to receive
warnings before playback. See [OpenAI provider notes](providers.md) for the
comparison's limitations.

## ElevenLabs streaming pipeline

Streaming path (default):

```
ElevenLabs /v1/text-to-speech/{voice}/stream
        ↓ (HTTP chunks)
io.MultiWriter ──→ os.File   (when -o is set)
                ↘
                  io.Pipe  ──→ player backend  (when --play)
```

Non-streaming path (`--no-stream`):

```
ElevenLabs /v1/text-to-speech/{voice}
        ↓ (full body)
[]byte ──→ os.File   (when -o)
        ↘
          bytes.Reader ──→ player backend  (when --play)
```

The streaming path uses a `io.MultiWriter` so writing to disk and playing in parallel is one allocation and one copy loop. The non-streaming path materializes the whole response, then plays it. 60db always uses the latter path; explicit `--stream` is rejected.

## When to disable streaming

Use `--no-stream` when:

- A downstream tool (validator, archiver, transcoder) needs a complete file before any consumer reads it.
- You’re comparing two takes and want bit-identical audio across runs.
- You hit weird boundary issues with a custom proxy that doesn’t handle chunked responses.

For everything else, keep streaming on. Latency wins are real, especially with v2.5 Flash.

## Latency tiers

`--latency-tier 0..4` is forwarded as the `optimize_streaming_latency` parameter. `0` is the model default; higher values trade fidelity for first-byte time.

```bash
sag --latency-tier 3 "Faster start, lossier mid-frequencies."
```

Tiers 3 and 4 are noticeable on long prompts; on short ones the win is mostly cosmetic. v2.5 Flash is already optimized — you’ll rarely need tiers above 1.

## Player backends

ElevenLabs streamed MP3 and OpenAI/ChatGPT streamed PCM use Oto by default with a 250 ms source reserve. Other playback uses `afplay` on macOS and Oto on Linux/Windows. Pick explicitly via `--player` or `SAG_PLAYER`; an explicit `afplay` choice retains its full-file buffering.

| Backend | Platforms | Notes |
| --- | --- | --- |
| `auto` | all | Streamed MP3/PCM → Oto; otherwise macOS → `afplay`, others → Oto. |
| `afplay` | macOS | Routes through CoreAudio so AirPlay, Bluetooth zones, and the menubar volume all work as expected. |
| `oto` | macOS, Linux, Windows | Pure-Go MP3 and PCM16 WAV decode + cross-platform output. Good fallback on macOS when `afplay` misbehaves (rare). |

```bash
sag --player oto "Run on Linux or force pure Go."
SAG_PLAYER=afplay sag "Always afplay."
```

`afplay` requires the audio to land as a complete temp file before playback starts; sag handles the buffering for you. `oto` decodes MP3 chunk-by-chunk and plays validated PCM16 WAV files.

## File-only mode

`-o/--output <path>` saves audio. ElevenLabs streaming writes during generation; OpenAI and 60db wait for a validated response. Speaker playback is disabled by default when `-o` is set; pass `--play` to keep both, or `--no-play` to be explicit:

```bash
sag -o intro.mp3 "Save only."
sag -o intro.mp3 --play "Save and listen."
sag --no-play "Headless: nothing happens — error."
```

The last form errors with `nothing to do: enable --play or provide --output`. `sag` refuses to spend tokens on audio it would throw away.

## Cancellation

`Ctrl-C` cancels both the HTTP request and the player. With `--timeout` (or `SAG_TIMEOUT`), sag uses `context.WithTimeout`; on expiry, in-flight bytes are dropped. ElevenLabs streaming can leave a partial file; OpenAI does not save incomplete audio. Use `ffprobe` to verify duration before publishing.

## ElevenLabs format ↔ player matrix

| `--format` | Streamed playback | File output |
| --- | --- | --- |
| `mp3_*` (default) | ✅ via afplay & oto | ✅ |
| `pcm_*` (WAV) | ⚠️ afplay needs the WAV header at file end; prefer `--no-play` | ✅ |
| `opus_*` | ✅ via afplay (macOS); oto cannot decode Opus today | ✅ |

If you need lossless WAV with playback, do `--no-stream -o file.wav --play`. For Opus on Linux, write the file and play it with `mpv` / `paplay` separately.

## Related pages

- [Speaking text](speak.md) — every flag, including streaming/play options.
- [Output & formats](formats.md) — codecs and format strings.
- [Configuration](configuration.md) — env vars, timeouts, base URL.
