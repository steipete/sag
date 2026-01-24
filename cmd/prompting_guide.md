# sag prompting guide

Goal: “more natural” output + controllable delivery.

## Choose model (matters)
- Default model: `speech-01`.
- MiniMax model IDs vary by account/region; check the MiniMax docs for the full list.
- If you have multiple tiers, pick higher-quality models for narration and lower-latency models for realtime.

## Universal “make it sound good” rules
- Write like a script: short sentences; newlines for beats.
- Punctuation is control: commas slow; em-dashes add breath; ellipses add weight; `!` adds energy.
- Use words for intent: “quietly” often works better than meta-instructions.
- If pronunciation is off: respell, add hyphens, or split syllables.

## Knobs in `sag`

Voice controls (model dependent):
- `--speed` or `--rate` to control delivery pace.
- `--emotion` to hint the tone (e.g. `neutral`, `happy`, `sad`).
- `--pitch` and `--volume` for finer adjustments.

Request controls:
- `--normalize auto|on|off` (auto uses server defaults; on/off force text normalization).
- `--lang` language boost hint (e.g. `en`, `zh`, `auto`).
- `--format` output audio format (e.g. `mp3`, `mp3_44100_128`, `wav`).
- `--metrics` prints chars/bytes/duration so you can iterate faster.

## Quick recipes

Natural narrator:
```
sag speak -v "Your Voice" --normalize auto --lang en \
  "We shipped today. It was close… but it worked."
```

Expressive delivery:
```
sag speak -v "Your Voice" --emotion happy --speed 1.05 \
  "We did it! I can’t believe it actually worked."
```

Calm + slower:
```
sag speak -v "Your Voice" --emotion neutral --rate 140 \
  "Take a breath. Slow down. Let the point land."
```
