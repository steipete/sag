package openai

import (
	"strings"
	"unicode"
)

// CheckTranscript reports possible narration drift without exposing the text.
// This is a heuristic: punctuation and case are ignored, but spoken number
// expansions and homophones can differ even when the audio is acceptable.
func CheckTranscript(input, transcript string) string {
	if strings.TrimSpace(transcript) == "" {
		return "OpenAI returned no transcript; narration could not be checked"
	}
	if normalizedWords(input) != normalizedWords(transcript) {
		return "OpenAI transcript differs from the input; narration may contain omissions or rewrites"
	}
	return ""
}

func normalizedWords(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) {
			return unicode.ToLower(r)
		}
		if r == '\'' || r == '’' {
			return -1
		}
		return ' '
	}, text)
	return strings.Join(strings.Fields(text), " ")
}
