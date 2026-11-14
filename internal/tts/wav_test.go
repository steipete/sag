package tts

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func TestFinalizePCM16WAV(t *testing.T) {
	want, err := hex.DecodeString("524946462800000057415645666d74201000000001000100c05d000080bb000002001000646174610400000001020304")
	if err != nil {
		t.Fatal(err)
	}
	buffer := append(make([]byte, 44), 1, 2, 3, 4)
	wav, err := FinalizePCM16WAV(buffer, 24000)
	if err != nil || !bytes.Equal(wav, want) {
		t.Fatalf("invalid WAV: %x %v", wav, err)
	}
	if &wav[0] != &buffer[0] {
		t.Fatal("finalization copied the recording")
	}
	wav, err = FinalizePCM16WAV(buffer, 48000)
	if err != nil || binary.LittleEndian.Uint32(wav[24:28]) != 48000 || binary.LittleEndian.Uint32(wav[28:32]) != 96000 {
		t.Fatalf("invalid 48 kHz header: %v", err)
	}
}

func TestFinalizePCM16WAVRejectsInvalidAudio(t *testing.T) {
	for _, tc := range []struct{ size, rate int }{{0, 24000}, {44, 24000}, {45, 24000}, {46, 0}, {46, -1}} {
		if wav, err := FinalizePCM16WAV(make([]byte, tc.size), tc.rate); err == nil || wav != nil {
			t.Fatalf("accepted size=%d rate=%d", tc.size, tc.rate)
		}
	}
}
