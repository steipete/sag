# Providers

`sag` supports two HTTP TTS backends. The CLI auto-selects the provider from your configured credentials; there is no `--provider` flag.

## Selecting a provider

| Keys set | Active provider |
| --- | --- |
| `ELEVENLABS_API_KEY` / `SAG_API_KEY` or `--api-key` / `--api-key-file` only | ElevenLabs |
| `SIXTYDB_API_KEY` or `SIXTYDB_API_KEY_FILE` only | 60db |
| both | error |
| neither | error |

Use `--base-url` to override the active provider host.

## 60db routes sag uses

The 60db integration is deliberately limited to routes verified against the live service:

| Capability | Route | Notes |
| --- | --- | --- |
| Voice listing | `GET /default-voices` | Workspace-default voices; queried first. |
| Voice listing | `GET /myvoices` | User-created voices; appended after defaults and deduped by `voice_id`. |
| Speak | `POST /tts-synthesize` | Live response is NDJSON containing base64 PCM chunks; sag validates the complete response and wraps it as 48 kHz mono WAV. |

For 60db, `sag` validates error and incomplete markers even on HTTP 200 responses, handles single- and double-encoded chunks used by the live service, rejects malformed or empty audio, and enforces per-chunk, per-frame, and total-audio limits. The adapter also retains support for the documented JSON envelope if the service returns it.

## Flag behavior

### Supported on both providers

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

These flags are passed through to ElevenLabs and rejected on 60db:

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
- 60db sends 48 kHz mono PCM; byte prefixes that resemble JSON or incomplete MP3 headers are preserved. Sag wraps it as WAV, so `--format wav` and `.wav` output are supported.

## Streaming, files, and playback

- ElevenLabs can stream in the requested output format, so `--stream` and `--format` work together.
- 60db uses validated full-response synthesis because the live `/tts-stream` route does not provide a working compatible stream.
- Sag disables its default streaming mode automatically for 60db. Explicit `--stream` is an error.
- 60db output is WAV only. It works with file output and both `afplay` and `oto` playback.

## Voice metadata notes

- `sag voices --try` uses `GET /voices/:id` on 60db to fetch `sample_url` when the list responses do not include preview URLs.
- 60db voice `model` and `categories` values are exposed as CLI labels so `--query` and `--label model=...` still work.
