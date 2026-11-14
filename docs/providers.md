# Providers

`sag` supports ElevenLabs, 60db, OpenAI Realtime, and experimental ChatGPT account voices. Use `--provider elevenlabs|60db|openai|chatgpt` for explicit selection. ChatGPT uses a separate account login and is never auto-selected. Automatic selection preserves existing ElevenLabs/60db commands and uses OpenAI only when neither existing provider is configured.

## Selecting a provider

| Keys set | Active provider |
| --- | --- |
| `ELEVENLABS_API_KEY` / `SAG_API_KEY` or `--api-key` / `--api-key-file` only | ElevenLabs |
| `SIXTYDB_API_KEY` or `SIXTYDB_API_KEY_FILE` only | 60db |
| `OPENAI_API_KEY` or `OPENAI_API_KEY_FILE` only | OpenAI |
| ElevenLabs + OpenAI | ElevenLabs (use `--provider openai` to override) |
| 60db + OpenAI | 60db (use `--provider openai` to override) |
| ElevenLabs + 60db, with or without OpenAI | error unless `--provider` selects one |
| none | error |

Use `--base-url` to override an API-key provider's host. It is not supported for ChatGPT account authentication.

## ChatGPT account (experimental)

This provider uses the installed Codex CLI's experimental v3 WebRTC transport,
tested with Codex 0.169.0. It exposes ChatGPT voices including Maple, Juniper,
Spruce, Ember, Vale, Breeze, Arbor, Sol, and Cove. These are separate from the
public OpenAI Realtime API voices. Account access is checked by the service;
a local voice catalog does not establish entitlement.

```sh
sag login --provider chatgpt --device-auth
sag voices --provider chatgpt
sag --provider chatgpt -v maple "Hello from Maple"
sag --provider chatgpt -v maple -o speech.wav "Save this narration"
```

The default credential directory is `~/.config/sag/chatgpt`. Override it with
`SAG_CHATGPT_HOME`; it must be outside the normal Codex home, including symlinked
paths. `SAG_CHATGPT_CODEX` selects the Codex executable (default: `codex` on PATH).
The login uses OpenAI's own browser/device flow. Sag does not copy browser cookies,
read the normal Codex account, change its configuration, or reuse API-key environment
variables for voice requests. Keep this credential directory private.

Only explicit `--provider chatgpt` commands use this sign-in. Existing API-key
selection and configuration remain unchanged. Calls use an ephemeral Codex thread
with startup context disabled. The input track contains generated silence; sag
does not record the microphone.

Playback streams decoded 24 kHz mono PCM through Oto as audio arrives, skipping
startup silence and using a 250 ms player prebuffer to absorb delivery jitter
and cover the audio backend's render blocks. The input audio clock starts
as soon as the WebRTC answer is applied, without waiting for the data channel.
Connection setup still takes time, but playback no longer waits
for the whole narration. `--no-stream` or `--player afplay` buffers before playback;
explicit `--stream --player afplay` is rejected. File-only output is buffered.
`--metrics` reports `first_pcm` when streaming audio first becomes available;
this excludes voice lookup and is not a measurement at the physical speaker.
Sag saves a WAV only after a completed transcript, drained audio tail, and
successful playback (when enabled). Failed synthesis or playback preserves any
existing output file. Input is capped at 3,500 UTF-8 bytes and sent in
Unicode-safe context chunks of at most 500 bytes. Split longer text into separate requests. The default
timeout is 90 seconds; `--timeout` and `SAG_TIMEOUT` override it.

On Unix, `--keep-alive 1m` starts a private local worker that keeps one unused
voice session ready while the current request plays. Requests run one at a time.
It caches the voice catalog
in memory and sends narration directly over the connected WebRTC data channel.
Each session speaks only once; completed narration and conversation history are
never reused for another request. The first request still pays connection setup.

Keep-alive is off by default. Choose a duration greater than zero and at most
5 minutes; `--keep-alive 0` disables it. After the timeout, the worker closes its
voice connections and removes its socket. No text or audio is saved by the
worker. Idle connections can consume account voice usage, so use a short timeout.
Changing voice replaces the prepared session and requires connection setup.

`--model-id`, `--speed`, `--rate`, voice-tuning flags, `--api-key`, `--api-key-file`,
and `--base-url` are unsupported. Voice preview URLs are unavailable; use a short
speak command instead of `voices --try`. Maple is the default voice.

The model can alter narration. Sag uses the same transcript comparison warning
as the OpenAI provider; review audio before relying on exact wording. Disclose
that the voice is AI-generated. This transport may change with Codex releases;
its reference contract is the [Codex v0.169.0 realtime protocol](https://github.com/openai/codex/blob/rust-v0.169.0/codex-rs/app-server-protocol/src/protocol/v2/realtime.rs).

## OpenAI Realtime

OpenAI connects to `wss://api.openai.com/v1/realtime` with Bearer authentication.
`--base-url` accepts an HTTP(S) or WS(S) API root including the version path
(for example `https://api.openai.com/v1`); sag appends `/realtime` and the model query.

- Normal-speed requests configure voice and format directly on `response.create`, avoiding a session-update round trip. Non-default speed waits for the speed configuration acknowledgement. WAV framing reuses the PCM buffer without another full-audio copy.
- Default model: `gpt-realtime-2.1-mini`; use `--model-id` for another compatible Realtime model.
- Default voice: `marin`. Also available: `cedar`, `alloy`, `ash`, `ballad`, `coral`, `echo`, `sage`, `shimmer`, and `verse`.
- `sag voices --provider openai` lists built-in voices locally. There are no preview URLs for `--try`; use `sag --provider openai -v NAME "Sample text"` instead.
- Playback streams 24 kHz mono PCM through Oto by default. The consumer receives complete 16-bit frames even when provider chunks split a sample. Playback failure stops synthesis and closes blocked reads/writes.
- Files remain WAV-only and require a successful `response.done`. With streamed playback, playback must also succeed before the file is written; failed synthesis or playback leaves existing output files untouched.
- `--no-stream` buffers before playback. `--player afplay` (or `SAG_PLAYER=afplay`) selects buffered macOS playback; explicit `--stream --player afplay` is rejected. File-only output is always buffered.
- `--speed` and `--rate` accept the shared CLI minimum of 0.5× and OpenAI's maximum of 1.5×.
- `--stability`, `--similarity`, `--similarity-boost`, `--style`, `--speaker-boost`, `--no-speaker-boost`, `--seed`, `--normalize`, `--lang`, and `--latency-tier` are rejected.
- An HTTP 401 response tells you to replace the selected `OPENAI_API_KEY` or `OPENAI_API_KEY_FILE` credential and retry, without echoing the key or server body.
- `--timeout` / `SAG_TIMEOUT` covers connection and synthesis. Cancellation closes the WebSocket. Per-event and total-audio limits prevent unbounded buffering.

Realtime is a generative model. Sag sends input as text to narrate with instructions
to read it verbatim, but exact wording is not guaranteed. Sag checks the final
`response.output_audio_transcript.done` transcript after successful generation.
Comparison ignores case, punctuation, and whitespace. Missing transcripts or
changed wording produce a warning without exposing either text. This is a heuristic:
number expansions, homophones, and spelling differences can warn even when audio
is acceptable. It is not independent audio transcription or a guarantee of accuracy.
With streaming, changed words may already have played before the warning arrives.
Disclose that the voice is AI-generated to listeners.

The older `/v1/audio/speech` models (`tts-1`, `tts-1-hd`, and `gpt-4o-mini-tts`)
are not used: OpenAI has [announced their shutdown for January 6, 2027](https://developers.openai.com/api/docs/deprecations#2026-10-01-text-to-speech-models).
See the official [Realtime conversation contract](https://developers.openai.com/api/docs/guides/realtime-conversations).

## 60db routes sag uses

The 60db integration is deliberately limited to routes verified against the live service:

| Capability | Route | Notes |
| --- | --- | --- |
| Voice listing | `GET /default-voices` | Workspace-default voices; queried first. |
| Voice listing | `GET /myvoices` | User-created voices; appended after defaults and deduped by `voice_id`. |
| Speak | `POST /tts-synthesize` | Live response is NDJSON containing base64 PCM chunks; sag validates the complete response and wraps it as 48 kHz mono WAV. |

For 60db, `sag` validates error and incomplete markers even on HTTP 200 responses, handles single- and double-encoded chunks used by the live service, rejects malformed or empty audio, and enforces per-chunk, per-frame, and total-audio limits. The adapter also retains support for the documented JSON envelope if the service returns it.

## ElevenLabs and 60db flag behavior

### Supported on ElevenLabs and 60db

- `-v, --voice`
- `-r, --rate`
- `--speed`
- `--stability`
- `--similarity` / `--similarity-boost`
- `--format`
- `--play` / `--no-play`
- `-o, --output`
- `--timeout`
- `--metrics`

### ElevenLabs-only

These flags are passed through to ElevenLabs and rejected on 60db (OpenAI also supports `--model-id`):

- `--model-id`
- `--style`
- `--speaker-boost`
- `--no-speaker-boost`
- `--seed`
- `--normalize`
- `--lang`
- `--latency-tier`

### Parameter translation

- `--stability` and `--similarity` stay in the CLI's `0..1` range and are scaled to 60db's documented `0..100` request values.
- 60db sends 48 kHz mono PCM. Sag wraps it as WAV, so `--format wav` and `.wav` output are supported.

## Streaming, files, and playback

- ElevenLabs can stream in the requested output format, so `--stream` and `--format` work together.
- 60db uses validated full-response synthesis because the live `/tts-stream` route does not provide a working compatible stream.
- Sag disables its default streaming mode automatically for 60db. Explicit `--stream` is an error.
- 60db output is WAV only. It works with file output and both `afplay` and `oto` playback.

## Voice metadata notes

- `sag voices --try` uses `GET /voices/:id` on 60db to fetch `sample_url` when the list responses do not include preview URLs.
- 60db voice `model` and `categories` values are exposed as CLI labels so `--query` and `--label model=...` still work.
