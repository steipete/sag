package chatgpt

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pion/opus"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
	"github.com/steipete/sag/internal/tts"
)

// MaxTextBytes stays below Codex's 1,000-token (4,000-byte) speech-append
// truncation boundary. Reject rather than silently narrating shortened input.
const MaxTextBytes = 3500

const sampleRate = 24000

// Result contains a complete WAV and the narration transcript.
type Result struct {
	Audio      []byte
	Transcript string
}

// ListVoices uses the installed Codex catalog for its v3 ChatGPT transport.
func (c *Client) ListVoices(ctx context.Context) ([]tts.Voice, error) {
	s, err := c.start(ctx)
	if err != nil {
		return nil, err
	}
	defer s.close()
	var catalog struct {
		Voices struct {
			V3 []string `json:"v3"`
		} `json:"voices"`
	}
	if err := s.call(ctx, "thread/realtime/listVoices", map[string]any{}, &catalog); err != nil {
		return nil, err
	}
	var voices []tts.Voice
	for _, name := range catalog.Voices.V3 {
		v := tts.Voice{VoiceID: name, Name: name, Category: "chatgpt", Description: "ChatGPT account voice"}
		if name == "maple" {
			voices = append([]tts.Voice{v}, voices...)
		} else {
			voices = append(voices, v)
		}
	}
	if len(voices) == 0 {
		return nil, errors.New("installed Codex does not expose ChatGPT v3 voices; use Codex 0.169.0 or a compatible release")
	}
	return voices, nil
}

// SearchVoices filters the native voice catalog by name.
func (c *Client) SearchVoices(ctx context.Context, query string, limit int) ([]tts.Voice, error) {
	voices, err := c.ListVoices(ctx)
	if err != nil {
		return nil, err
	}
	var matches []tts.Voice
	for _, voice := range voices {
		if strings.Contains(voice.Name, strings.ToLower(query)) {
			matches = append(matches, voice)
			if limit > 0 && len(matches) >= limit {
				break
			}
		}
	}
	return matches, nil
}

// GetVoice resolves a native ChatGPT voice.
func (c *Client) GetVoice(ctx context.Context, id string) (tts.Voice, error) {
	voices, err := c.ListVoices(ctx)
	if err != nil {
		return tts.Voice{}, err
	}
	for _, voice := range voices {
		if voice.VoiceID == id {
			return voice, nil
		}
	}
	return tts.Voice{}, fmt.Errorf("unknown ChatGPT voice %q", id)
}

type recording struct {
	sync.Mutex
	samples   []int16
	quiet     int
	voiced    int
	sent      int
	streaming bool
}

func (r *recording) append(samples []int16) error {
	r.Lock()
	defer r.Unlock()
	if len(r.samples)+len(samples) > 32<<20 {
		return errors.New("ChatGPT audio exceeded 64 MiB limit")
	}
	r.samples = append(r.samples, samples...)
	for _, sample := range samples {
		if sample > 50 || sample < -50 {
			r.voiced++
			r.quiet = 0
		} else {
			r.quiet++
		}
	}
	return nil
}

// writePCM reads from the bounded recording on the synthesis goroutine, so
// a slow or failed player cannot block RTP decoding or hold the recording lock.
func (r *recording) writePCM(onPCM func([]byte) error) error {
	if onPCM == nil {
		return nil
	}
	r.Lock()
	if r.voiced == 0 || r.sent == len(r.samples) {
		r.Unlock()
		return nil
	}
	if !r.streaming {
		for r.samples[r.sent] <= 50 && r.samples[r.sent] >= -50 {
			r.sent++
		}
		r.sent = max(0, r.sent-sampleRate/10)
		r.streaming = true
	}
	data := make([]byte, (len(r.samples)-r.sent)*2)
	for i, sample := range r.samples[r.sent:] {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(sample))
	}
	r.sent = len(r.samples)
	r.Unlock()
	return onPCM(data)
}

func (r *recording) drained() bool {
	r.Lock()
	defer r.Unlock()
	return r.voiced > 1000 && r.quiet >= sampleRate*3/10
}

func (r *recording) wav() ([]byte, error) {
	r.Lock()
	defer r.Unlock()
	if r.voiced < 1000 {
		return nil, errors.New("ChatGPT returned a transcript without usable speech audio")
	}
	start, end := 0, len(r.samples)
	for start < end && r.samples[start] <= 50 && r.samples[start] >= -50 {
		start++
	}
	for end > start && r.samples[end-1] <= 50 && r.samples[end-1] >= -50 {
		end--
	}
	start = max(0, start-sampleRate/10)
	end = min(len(r.samples), end+sampleRate/10)
	samples := r.samples[start:end]
	wav := make([]byte, 44+len(samples)*2)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], sampleRate)
	binary.LittleEndian.PutUint32(wav[28:], sampleRate*2)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(samples)*2))
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(wav[44+i*2:], uint16(sample))
	}
	return wav, nil
}

func realtimeError(m rpcMessage) error {
	if m.Method == "thread/realtime/error" {
		var p struct{ Message string }
		_ = json.Unmarshal(m.Params, &p)
		return fmt.Errorf("ChatGPT voice: %s", p.Message)
	}
	if m.Method == "thread/realtime/closed" {
		var p struct{ Reason string }
		_ = json.Unmarshal(m.Params, &p)
		return fmt.Errorf("ChatGPT voice closed before completion: %s", p.Reason)
	}
	return nil
}

// Synthesize renders one narration with the account-authenticated native voice
// transport. onPCM receives PCM16 audio as it arrives; the final WAV is returned
// only after the completed transcript and audio tail. A callback error stops synthesis.
func (c *Client) Synthesize(ctx context.Context, text, voice string, onPCM func([]byte) error) (Result, error) {
	if strings.TrimSpace(text) == "" {
		return Result{}, errors.New("ChatGPT input text is empty")
	}
	if len(text) > MaxTextBytes {
		return Result{}, fmt.Errorf("ChatGPT input exceeds %d UTF-8 bytes; split the narration into smaller requests", MaxTextBytes)
	}
	if voice == "" {
		voice = "maple"
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s, err := c.start(ctx)
	if err != nil {
		return Result{}, err
	}
	defer s.close()
	var thread struct{ Thread struct{ ID string } }
	if err := s.call(ctx, "thread/start", map[string]any{"ephemeral": true, "approvalPolicy": "never", "sandbox": "read-only", "modelProvider": "openai", "cwd": c.Home, "baseInstructions": "You provide speech only. Do not use tools.", "developerInstructions": "Do not use tools."}, &thread); err != nil {
		return Result{}, err
	}
	id := thread.Thread.ID
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = pc.Close() }()
	var audio recording
	failed := make(chan error, 1)
	fail := func(err error) {
		select {
		case failed <- err:
		default:
		}
	}
	readerStarted := make(chan struct{})
	readerDone := make(chan struct{})
	var once sync.Once
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		accepted := false
		once.Do(func() { accepted = true; close(readerStarted) })
		if !accepted {
			fail(errors.New("unexpected additional ChatGPT audio track"))
			return
		}
		defer close(readerDone)
		decoder, err := opus.NewDecoderWithOutput(sampleRate, 1)
		if err != nil {
			fail(err)
			return
		}
		builder := samplebuilder.New(50, &codecs.OpusPacket{}, 48000)
		pcm := make([]int16, 5760)
		decode := func() bool {
			for sample := builder.Pop(); sample != nil; sample = builder.Pop() {
				if sample.PrevDroppedPackets > 0 {
					fail(errors.New("ChatGPT audio lost packets; no incomplete file was saved"))
					return false
				}
				n, err := decoder.DecodeToInt16(sample.Data, pcm)
				if err != nil {
					fail(fmt.Errorf("decode ChatGPT audio: %w", err))
					return false
				}
				if err := audio.append(pcm[:n]); err != nil {
					fail(err)
					return false
				}
			}
			return true
		}
		for {
			packet, _, err := track.ReadRTP()
			if err != nil {
				builder.Flush()
				_ = decode()
				return
			}
			builder.Push(packet)
			if !decode() {
				return
			}
		}
	})
	silence, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "sag")
	if err != nil {
		return Result{}, err
	}
	sender, err := pc.AddTrack(silence)
	if err != nil {
		return Result{}, err
	}
	go func() {
		b := make([]byte, 1500)
		for {
			if _, _, err := sender.Read(b); err != nil {
				return
			}
		}
	}()
	// Use DCEP negotiation: a manually negotiated channel can open locally while
	// the service never receives its events.
	dc, err := pc.CreateDataChannel("oai-events", nil)
	if err != nil {
		return Result{}, err
	}
	ready := make(chan struct{})
	started := make(chan struct{})
	var startOnce sync.Once
	dc.OnOpen(func() { close(ready) })
	dc.OnMessage(func(message webrtc.DataChannelMessage) {
		var event struct {
			Type  string `json:"type"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if len(message.Data) > 4<<20 {
			fail(errors.New("ChatGPT voice event exceeded size limit"))
			return
		}
		if err := json.Unmarshal(message.Data, &event); err != nil {
			fail(errors.New("invalid ChatGPT voice event"))
			return
		}
		if event.Type == "session.started" {
			startOnce.Do(func() { close(started) })
		}
		if event.Type == "error" {
			fail(fmt.Errorf("ChatGPT voice: %s", event.Error.Message))
		}
	})
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return Result{}, err
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		return Result{}, err
	}
	select {
	case <-gathered:
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	params := map[string]any{"threadId": id, "version": "v3", "voice": voice, "outputModality": "audio", "includeStartupContext": false, "clientManagedHandoffs": true, "prompt": "You are a text-to-speech narrator. Immediately read supplied speech word for word, without commentary. Stop after the supplied text. Do not use tools.", "initialItems": []any{map[string]any{"role": "developer", "text": "Begin immediately. Say exactly: " + text}}, "transport": map[string]any{"type": "webrtc", "sdp": pc.LocalDescription().SDP}}
	if err := s.call(ctx, "thread/realtime/start", params, nil); err != nil {
		return Result{}, err
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer stopCancel()
		_ = s.call(stopCtx, "thread/realtime/stop", map[string]any{"threadId": id}, nil)
	}()
	for {
		m, err := s.next(ctx)
		if err != nil {
			return Result{}, err
		}
		if err := realtimeError(m); err != nil {
			return Result{}, err
		}
		if m.Method == "thread/realtime/sdp" {
			var answer struct{ SDP string }
			if err := json.Unmarshal(m.Params, &answer); err != nil {
				return Result{}, err
			}
			if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer.SDP}); err != nil {
				return Result{}, err
			}
			break
		}
	}
	select {
	case <-ready:
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	// GPT-Live advances on an active input stream. Pion's valid 20 ms Opus
	// silence frame supplies the clock without recording a microphone.
	go func() {
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if err := silence.WriteSample(media.Sample{Data: []byte{0xf8, 0xff, 0xfe}, Duration: 20 * time.Millisecond}); err != nil {
					fail(err)
					return
				}
			}
		}
	}()
	// An open SCTP channel does not imply that the voice model is initialized.
	// Wait for its application-level acknowledgement before appending speech.
	select {
	case <-started:
	case err := <-failed:
		return Result{}, err
	case <-ctx.Done():
		return Result{}, fmt.Errorf("ChatGPT voice session did not initialize: %w", ctx.Err())
	}
	if err := s.call(ctx, "thread/realtime/appendSpeech", map[string]any{"threadId": id, "text": text}, nil); err != nil {
		return Result{}, err
	}
	var transcript string
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		var m rpcMessage
		if len(s.pending) > 0 {
			m = s.pending[0]
			s.pending = s.pending[1:]
		} else {
			select {
			case m = <-s.messages:
			case err := <-s.readError:
				return Result{}, err
			case err := <-failed:
				return Result{}, err
			case <-ctx.Done():
				phase := "no completed transcript"
				if transcript != "" {
					phase = "audio tail did not complete"
				}
				return Result{}, fmt.Errorf("ChatGPT narration did not complete (%s): %w", phase, ctx.Err())
			case <-tick.C:
				if err := audio.writePCM(onPCM); err != nil {
					return Result{}, err
				}
				// Sideband transcript completion can precede the last RTP packets.
				// Drain a decoded 300 ms quiet tail before closing the media stream.
				if transcript != "" && audio.drained() {
					_ = pc.Close()
					select {
					case <-readerStarted:
						<-readerDone
					default:
					}
					select {
					case err := <-failed:
						return Result{}, err
					default:
					}
					if err := audio.writePCM(onPCM); err != nil {
						return Result{}, err
					}
					wav, err := audio.wav()
					return Result{Audio: wav, Transcript: transcript}, err
				}
				continue
			}
		}
		if err := realtimeError(m); err != nil {
			return Result{}, err
		}
		if m.Method == "thread/realtime/transcript/done" {
			var p struct{ Role, Text string }
			if err := json.Unmarshal(m.Params, &p); err != nil {
				return Result{}, err
			}
			if p.Role == "assistant" {
				transcript = strings.TrimSpace(p.Text)
			}
		}
	}
}
