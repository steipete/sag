package tts

import (
	"encoding/binary"
	"errors"
	"math"
)

// FinalizePCM16WAV fills a reserved 44-byte header in a mono PCM16 buffer.
// Keeping the header with the samples avoids copying the complete recording.
func FinalizePCM16WAV(wav []byte, sampleRate int) ([]byte, error) {
	if len(wav) <= 44 {
		return nil, errors.New("empty PCM audio")
	}
	if (len(wav)-44)%2 != 0 {
		return nil, errors.New("PCM audio has an incomplete 16-bit sample")
	}
	if sampleRate <= 0 || uint64(sampleRate) > math.MaxUint32/2 {
		return nil, errors.New("invalid PCM sample rate")
	}
	if uint64(len(wav)-8) > math.MaxUint32 {
		return nil, errors.New("PCM audio is too large for WAV")
	}
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(wav[28:], uint32(sampleRate)*2)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(wav)-44))
	return wav, nil
}
