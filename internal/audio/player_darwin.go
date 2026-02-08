//go:build darwin

package audio

import (
	"context"
	"io"
)

// StreamToSpeakers plays MP3 audio via macOS afplay, which correctly handles AirPlay devices.
func StreamToSpeakers(ctx context.Context, r io.Reader) error {
	return StreamViaAfplay(ctx, r)
}
