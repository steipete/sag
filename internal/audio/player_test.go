package audio

import (
	"context"
	"strings"
	"testing"
)

func TestStreamToSpeakersBadMP3(t *testing.T) {
	err := StreamToSpeakers(context.Background(), strings.NewReader("not-mp3"))
	if err == nil {
		t.Fatalf("expected decode error")
	}
}
