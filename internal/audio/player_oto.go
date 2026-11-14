package audio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/hajimehoshi/go-mp3"
)

const playbackPollInterval = 100 * time.Millisecond

var (
	audioCtxMu      sync.Mutex
	audioCtx        *oto.Context
	audioReady      chan struct{}
	audioSampleRate int
	audioChannels   int
	audioFormat     oto.Format
	audioContextErr error
)

// StreamViaOto decodes MP3 or PCM16 WAV audio and plays it via the oto backend.
func StreamViaOto(ctx context.Context, r io.Reader) error {
	reader := bufio.NewReader(r)
	header, err := reader.Peek(4)
	if err == nil && bytes.Equal(header, []byte("RIFF")) {
		data, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("read wav: %w", err)
		}
		wav, err := parsePCM16WAV(data)
		if err != nil {
			return fmt.Errorf("decode wav: %w", err)
		}
		return playPCM(ctx, bytes.NewReader(wav.data), wav.sampleRate, wav.channels, 0)
	}

	decoder, err := mp3.NewDecoder(reader)
	if err != nil {
		return fmt.Errorf("decode mp3: %w", err)
	}
	return playPCM(ctx, decoder, decoder.SampleRate(), 2, 0)
}

// StreamPCM16 plays signed little-endian PCM without waiting for a WAV file.
// It owns r and closes it on return or cancellation to unblock audio reads.
func StreamPCM16(ctx context.Context, r io.ReadCloser, sampleRate, channelCount int) error {
	defer func() { _ = r.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = r.Close() })
	defer stop()
	return playPCM(ctx, r, sampleRate, channelCount, 100*time.Millisecond)
}

func playPCM(ctx context.Context, pcm io.Reader, sampleRate, channelCount int, prebuffer time.Duration) error {
	audioCtx, ready, err := getAudioContext(sampleRate, channelCount, oto.FormatSignedInt16LE)
	if err != nil {
		return fmt.Errorf("audio context: %w", err)
	}
	if err := waitForAudioReady(ctx, ready); err != nil {
		return err
	}

	player := audioCtx.NewPlayer(pcm)
	if prebuffer > 0 {
		// Oto fills its source buffer before Play returns. Its 500 ms default
		// delays live speech; retain 100 ms of jitter tolerance for streaming PCM.
		player.SetBufferSize(int(int64(sampleRate*channelCount*2) * int64(prebuffer) / int64(time.Second)))
	}
	player.Play()

	return waitForPlayback(ctx, player)
}

type pcm16WAV struct {
	data       []byte
	sampleRate int
	channels   int
}

func parsePCM16WAV(data []byte) (pcm16WAV, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return pcm16WAV{}, errors.New("missing RIFF/WAVE header")
	}

	var (
		formatFound bool
		audioFormat uint16
		channels    uint16
		sampleRate  uint32
		bits        uint16
		pcm         []byte
	)
	for offset := 12; offset+8 <= len(data); {
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		chunkStart := offset + 8
		if chunkSize < 0 || chunkStart > len(data) || chunkSize > len(data)-chunkStart {
			return pcm16WAV{}, errors.New("truncated WAV chunk")
		}
		chunk := data[chunkStart : chunkStart+chunkSize]
		switch string(data[offset : offset+4]) {
		case "fmt ":
			if len(chunk) < 16 {
				return pcm16WAV{}, errors.New("truncated fmt chunk")
			}
			audioFormat = binary.LittleEndian.Uint16(chunk[0:2])
			channels = binary.LittleEndian.Uint16(chunk[2:4])
			sampleRate = binary.LittleEndian.Uint32(chunk[4:8])
			bits = binary.LittleEndian.Uint16(chunk[14:16])
			formatFound = true
		case "data":
			pcm = chunk
		}
		offset = chunkStart + chunkSize
		if chunkSize%2 != 0 {
			offset++
		}
	}

	if !formatFound {
		return pcm16WAV{}, errors.New("missing fmt chunk")
	}
	if audioFormat != 1 || bits != 16 {
		return pcm16WAV{}, fmt.Errorf("unsupported WAV format %d/%d-bit", audioFormat, bits)
	}
	if channels == 0 || sampleRate == 0 {
		return pcm16WAV{}, errors.New("invalid WAV channel count or sample rate")
	}
	if len(pcm) == 0 {
		return pcm16WAV{}, errors.New("missing audio data")
	}
	if len(pcm)%(int(channels)*2) != 0 {
		return pcm16WAV{}, errors.New("incomplete PCM frame")
	}
	return pcm16WAV{data: pcm, sampleRate: int(sampleRate), channels: int(channels)}, nil
}

func getAudioContext(sampleRate, channelCount int, format oto.Format) (*oto.Context, chan struct{}, error) {
	audioCtxMu.Lock()
	defer audioCtxMu.Unlock()

	if audioCtx != nil {
		if audioContextErr != nil {
			return nil, nil, audioContextErr
		}
		if audioSampleRate != sampleRate {
			return nil, nil, fmt.Errorf("context already initialized at %d Hz; got %d Hz", audioSampleRate, sampleRate)
		}
		if audioChannels != channelCount || audioFormat != format {
			return nil, nil, fmt.Errorf("context already initialized with %d channels/format %d; got %d channels/format %d", audioChannels, audioFormat, channelCount, format)
		}
		return audioCtx, audioReady, nil
	}

	if sampleRate <= 0 {
		return nil, nil, errors.New("invalid sample rate")
	}

	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: channelCount,
		Format:       format,
	})
	if err != nil {
		audioContextErr = err
		return nil, nil, err
	}
	audioCtx = ctx
	audioReady = ready
	audioSampleRate = sampleRate
	audioChannels = channelCount
	audioFormat = format
	audioContextErr = nil
	return audioCtx, audioReady, nil
}

type playbackState interface {
	IsPlaying() bool
	Pause()
	Err() error
}

func waitForPlayback(ctx context.Context, player playbackState) error {
	ticker := time.NewTicker(playbackPollInterval)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			player.Pause()
			return err
		}
		if !player.IsPlaying() {
			return player.Err()
		}
		select {
		case <-ctx.Done():
			player.Pause()
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForAudioReady(ctx context.Context, ready <-chan struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ready == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ready:
		return nil
	}
}
