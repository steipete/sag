---
title: Architecture
description: "How sag is laid out: command package, ElevenLabs client, audio backends, and the streaming pipeline."
---

# Architecture

`sag` is a small Go program that wraps a single HTTP API and one audio playback path. It’s intentionally simple — most of the surface area lives in flag parsing and the streaming pipe.

## Layout

```
cmd/sag/main.go              Thin main, calls cmd.Execute().
cmd/                         Cobra commands.
  root.go                    Root command, default-to-speak routing.
  speak.go                   speak subcommand: streaming + playback + file output.
  voices.go                  voices subcommand: search, query, label, --try.
  prompting.go               In-binary prompting cheat sheet.
  api_key.go                 API key resolution (flag, env, file).
internal/elevenlabs/         ElevenLabs HTTP client (TTS + voices).
internal/audio/              Player backends (afplay + oto).
docs/                        This documentation site.
```

There’s no plugin system, no ABI, no internal RPC. Everything is one binary, one process, one HTTP keep-alive.

## Default-to-speak routing

`sag` reads like macOS `say`. The trick is in `cmd/root.go`:

1. If no args and stdin is piped → inject `speak`.
2. If `argv[1] == "--"` (npm/pnpm pass-through) → drop it and re-dispatch.
3. If `argv[1]` is a known subcommand or Cobra builtin (`help`, `completion`) → run it directly.
4. Otherwise → inject `speak` so flag parsing stays consistent.

That’s why `sag "Hello"`, `sag speak "Hello"`, and `echo Hello | sag` all behave the same.

## ElevenLabs client

`internal/elevenlabs/client.go` exposes:

- `NewClient(apiKey, baseURL)` — `&http.Client{Timeout: 0}` so long generations don’t get cut off.
- `ListVoices(ctx)` / `SearchVoices(ctx, query, limit)` / `GetVoice(ctx, voiceID)`.
- `ConvertTTS(ctx, voiceID, req)` — non-streaming `POST /v1/text-to-speech/{voiceID}`. Returns the full body.
- `StreamTTS(ctx, voiceID, req, latencyTier)` — streaming `POST /v1/text-to-speech/{voiceID}/stream`. Returns an `io.ReadCloser` over the response body.

The request type is a strict struct (`TTSRequest`) that omits fields with `omitempty` so server defaults stay intact when the user doesn’t set a slider.

## 60db client

`internal/sixtydb/client.go` merges the live `/default-voices` and `/myvoices` catalogs and uses `/tts-synthesize` for speech. The live service returns NDJSON base64 chunks containing 48 kHz mono PCM. The adapter bounds and validates every frame, rejects incomplete output, supports the documented JSON envelope as a compatibility path, and wraps raw PCM in a WAV container before returning it.

## Streaming pipeline

Streaming combines a file write and a player feed via `io.MultiWriter`:

```text
HTTP body ──► io.Reader ──► io.Copy ──► MultiWriter ──► *os.File   (when -o)
                                                  ╲
                                                   ─► io.Pipe ──► player.Stream(ctx, pr)
```

On the streaming path:

1. The HTTP body is wrapped in a `defer Close`.
2. If `-o` is set, sag opens the destination file and adds it to the writers.
3. If `--play` is on, sag creates an `io.Pipe`, adds the writer to `MultiWriter`, and hands the reader to the player backend in a goroutine.
4. `io.Copy` runs on the goroutine; the main goroutine waits on the player.
5. Errors propagate through unbuffered channels so the partial write/play state is observable.

The non-streaming path is simpler: read the full body, write the file, then play the buffer.

## Audio backends

`internal/audio` owns playback. Two backends:

- `afplay` — shells out to macOS `afplay`. Buffers the entire stream to a temp file (afplay needs the full WAV header for PCM). Uses `os.CommandContext` so cancellation works.
- `oto` — pure-Go MP3 or PCM16 WAV decoder pipeline into `oto.Player`. MP3 is decoded chunk-by-chunk; WAV playback uses the validated complete file.

The `auto` mode uses Oto for ElevenLabs streamed MP3 and OpenAI/ChatGPT streamed PCM. Other playback uses `afplay` on macOS and Oto elsewhere. Override with `--player` or `SAG_PLAYER`.

## Voice resolution

`resolveVoice` is the most policy-heavy function. Order:

1. Empty input → fetch voices, pick the first, log to stderr.
2. `?` → fetch voices, print the table, return early (don’t call TTS).
3. `--voice-id` set → trust the value verbatim.
4. Looks like an ID (≥15 chars, no spaces, has digits) → trust it.
5. Looks like an ID but no digits → fetch voices, prefer exact name match, fall back to the input.
6. Plain name → fetch voices, exact match first, then case-insensitive substring.

The flow is deterministic and always logs the chosen voice to stderr when it had to look anything up. That makes the output diff-friendly between runs and easy to verify in CI.

## Voice metadata cache

`voices --query` and `--label` need full voice descriptors. `cmd/voices_cache.go` keeps a JSON cache in the OS config dir, with a 24-hour TTL. The cache is per-(API key, base URL) and corrupt files are recreated rather than crashing the run.

## Format inference

`inferFormatFromExt` maps `.mp3` → `mp3_44100_128`, `.wav`/`.wave` → `pcm_44100`, `.ogg`/`.opus` → `opus_48000_64`. Anything else falls through to the configured default. `--format` always wins. See [Output & formats](formats.md).

## Compatibility flags

`speak.go` registers a handful of flags that exist purely for `say` parity:

`--progress`, `--audio-device`, `--network-send`, `--interactive`, `--file-format`, `--data-format`, `--channels`, `--bit-rate`, `--quality`.

They’re bound but never read. The CLI message advertises them as accepted for compatibility.

## Testing

- `cmd/*_test.go` covers flag parsing, voice resolution, format inference, and default-to-speak routing.
- `internal/elevenlabs/client_test.go` uses `httptest.Server` to validate request shapes.
- `internal/audio/player_test.go` exercises the player wiring without touching real hardware.

`go test ./...` runs the full suite. Lint is golangci-lint with the project’s `.golangci.yml`.

## Related pages

- [Speaking text](speak.md) — every flag the speak command exposes.
- [Streaming & playback](streaming.md) — how the pipeline behaves at runtime.
- [Spec](spec.md) — the original (smaller) reference.
