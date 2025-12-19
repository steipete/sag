# sag prompting guide

Goal: “more natural” output + controllable delivery.

## Choose model (matters)

### v3 (alpha) (default in `sag`)
- Model ID: `eleven_v3`
- Uses inline audio tags: lowercase `[square brackets]` inside your text.
- SSML `<break>` is *not* supported; use v3 pause tags instead: `[pause]`, `[short pause]`, `[long pause]`.
- Short prompts can be unstable; longer scripts tend to behave better (aim 250+ chars).

### v2 / v2.5 (stable baseline)
- Model ID: `eleven_multilingual_v2`
- Best baseline for “just speak this”.
- Supports SSML `<break time="1.5s" />` for precise pauses (up to ~3s).
- Some English-only models support SSML `<phoneme>` for pronunciation (not yet exposed in `sag`).

## Universal “make it sound good” rules
- Write like a script: short sentences; newlines for beats.
- Punctuation is control: commas + em-dashes slow; ellipses add weight; `!` adds energy.
- Put emphasis in words, not instructions: “quietly” usually beats “whisper please”.
- If you need exact pronunciation: respell (e.g. “key-note”); otherwise enable normalization.

## v3 audio tags (examples)

Voice-related:
- `[whispers]`, `[shouts]`
- `[laughs]`, `[starts laughing]`, `[wheezing]`
- `[sighs]`, `[exhales]`, `[clears throat]`
- `[sarcastic]`, `[curious]`, `[excited]`, `[crying]`, `[mischievously]`

Sound effects:
- `[applause]`, `[clapping]`, `[gunshot]`, `[explosion]`
- `[swallows]`, `[gulps]`

Experimental:
- `[strong X accent]` (replace X, e.g. “French”)
- `[sings]`

Notes:
- Tag effectiveness depends on the voice + training samples; not every voice reacts well.
- Combine tags sparingly; more tags ≠ better audio.

## Knobs in `sag` (0.2.0)

Voice sliders:
- `--stability 0..1` (higher = more consistent, less expressive)
- `--similarity 0..1` (higher = closer to reference voice, less flexible)
- `--style 0..1` (higher = more “styled” delivery; voice/model dependent)
- `--speaker-boost` (can add clarity; model dependent)

Request controls:
- `--seed 0..4294967295` (best-effort repeatability; not perfect determinism)
- `--normalize auto|on|off` (numbers/units/URLs normalization)
- `--lang en|de|fr|...` (2-letter ISO 639-1; influences normalization)
- `--metrics` (prints chars/bytes/duration so you can iterate faster)

## Quick recipes

Natural narrator (v2 default):
```
sag speak -v Roger --stability 0.5 --similarity 0.75 --style 0.0 --normalize auto --lang en \
  "We shipped today. It was close… but it worked."
```

Expressive (v3):
```
sag speak -v Roger --model-id eleven_v3 --stability 0.35 --normalize off --lang en \
  "[whispers] Don’t move. [short pause] Something’s in the hallway…"
```
