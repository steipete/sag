package chatgpt

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steipete/sag/internal/tts"
)

const maxWarmIdle = 5 * time.Minute
const maxFrameBytes = (64 << 20) + 44

// ValidateKeepAlive bounds the lifetime of idle account voice connections.
func ValidateKeepAlive(idle time.Duration) error {
	if !supportsWarm || idle <= 0 || idle > maxWarmIdle {
		return errors.New("ChatGPT --keep-alive requires Unix and a duration greater than zero and at most 5m")
	}
	return nil
}

type workerRequest struct {
	Kind, Text, Voice, Binary string
	Idle                      time.Duration
	Deadline                  time.Time
}

func (c *Client) socketPath() string {
	return filepath.Join(c.Home, "voice", "worker-v2.sock")
}

func (c *Client) warmCall(ctx context.Context, req workerRequest, onPCM func([]byte) error) (Result, []tts.Voice, error) {
	if err := ValidateKeepAlive(c.KeepAlive); err != nil {
		return Result{}, nil, err
	}
	if err := c.validateHome(); err != nil {
		return Result{}, nil, err
	}
	if len(c.socketPath()) > 100 {
		return Result{}, nil, errors.New("ChatGPT home is too long for a local voice socket")
	}
	req.Idle, req.Binary = c.KeepAlive, c.Binary
	req.Deadline, _ = ctx.Deadline()
	dial := func() (net.Conn, error) { return (&net.Dialer{}).DialContext(ctx, "unix", c.socketPath()) }
	conn, err := dial()
	if err != nil {
		if err := c.startWorker(); err != nil {
			return Result{}, nil, err
		}
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for conn == nil {
			select {
			case <-ctx.Done():
				return Result{}, nil, ctx.Err()
			case <-timer.C:
				return Result{}, nil, errors.New("ChatGPT voice worker did not start within 5s")
			case <-tick.C:
				conn, _ = dial()
			}
		}
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Result{}, nil, err
	}
	var result Result
	for {
		kind, data, err := readFrame(conn)
		if err != nil {
			if ctx.Err() != nil {
				err = ctx.Err()
			}
			return Result{}, nil, fmt.Errorf("ChatGPT voice worker: %w", err)
		}
		switch kind {
		case 'p':
			if len(data)%2 != 0 {
				return Result{}, nil, errors.New("invalid worker PCM frame")
			}
			if onPCM != nil {
				if err := onPCM(data); err != nil {
					return Result{}, nil, err
				}
			}
		case 'w':
			result.Audio = data
		case 't':
			if len(result.Audio) < 44 {
				return Result{}, nil, errors.New("voice worker returned no complete audio")
			}
			result.Transcript = string(data)
			return result, nil, nil
		case 'v':
			var voices []tts.Voice
			if err := json.Unmarshal(data, &voices); err != nil {
				return Result{}, nil, err
			}
			return Result{}, voices, nil
		case 'e':
			return Result{}, nil, fmt.Errorf("ChatGPT voice worker: %s", data)
		default:
			return Result{}, nil, errors.New("unknown voice worker frame")
		}
	}
}

func (c *Client) startWorker() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, "voice-worker", "--idle-timeout", c.KeepAlive.String())
	for _, entry := range isolatedEnv(c.Home) {
		if !strings.HasPrefix(entry, "CODEX_HOME=") && !strings.HasPrefix(entry, "SAG_CHATGPT_HOME=") && !strings.HasPrefix(entry, "SAG_CHATGPT_CODEX=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "SAG_CHATGPT_HOME="+c.Home, "SAG_CHATGPT_CODEX="+c.Binary)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

type narrator interface {
	synthesize(context.Context, string, func([]byte) error) (Result, error)
	close()
}

type preparation struct {
	cancel context.CancelFunc
	done   chan struct{}
	voice  narrator
	err    error
}

func beginPreparation(ctx context.Context, voice string, prepare func(context.Context, string) (narrator, error)) *preparation {
	ctx, cancel := context.WithCancel(ctx)
	p := &preparation{cancel: cancel, done: make(chan struct{})}
	go func() {
		p.voice, p.err = prepare(ctx, voice)
		close(p.done)
	}()
	return p
}

func (p *preparation) close() {
	select {
	case <-p.done:
	default:
		p.cancel()
		<-p.done
	}
	if p.voice != nil {
		p.voice.close()
	}
	p.cancel()
}

// ServeWarm owns a private local worker. It keeps at most one unused session
// ready while a different, one-use session narrates the current request.
func (c *Client) ServeWarm(ctx context.Context, idle time.Duration) error {
	if err := ValidateKeepAlive(idle); err != nil {
		return err
	}
	if err := c.validateHome(); err != nil {
		return err
	}
	path := c.socketPath()
	if len(path) > 100 {
		return errors.New("ChatGPT home is too long for a local voice socket")
	}
	if err := (&Client{Home: filepath.Dir(path)}).validateHome(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil || info.Mode().Perm()&0077 != 0 {
		return errors.New("ChatGPT voice directory must be private (mode 0700)")
	}
	lock, err := lockWorker(path + ".lock")
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("voice worker path is not a socket")
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	return c.serveWarm(ctx, listener, idle, func(ctx context.Context, voice string) (narrator, error) { return c.prepare(ctx, voice) }, c.listVoices)
}

func (c *Client) serveWarm(ctx context.Context, listener *net.UnixListener, idle time.Duration, prepare func(context.Context, string) (narrator, error), catalog func(context.Context) ([]tts.Voice, error)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()
	var warm *preparation
	var warmVoice string
	var voices []tts.Voice
	defer func() {
		if warm != nil {
			warm.close()
		}
	}()
	for {
		if err := listener.SetDeadline(time.Now().Add(idle)); err != nil {
			return err
		}
		conn, err := listener.AcceptUnix()
		if err != nil {
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() || ctx.Err() != nil {
				return nil
			}
			return err
		}
		func() {
			defer func() { _ = conn.Close() }()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			var req workerRequest
			if err := json.NewDecoder(io.LimitReader(conn, 32<<10)).Decode(&req); err != nil {
				return
			}
			if err := ValidateKeepAlive(req.Idle); err != nil || req.Binary != c.Binary {
				_ = writeFrame(conn, 'e', []byte("invalid voice worker lifetime or Codex executable"))
				return
			}
			idle = req.Idle
			requestCtx, requestCancel := context.WithCancel(ctx)
			if !req.Deadline.IsZero() {
				requestCancel()
				requestCtx, requestCancel = context.WithDeadline(ctx, req.Deadline)
			}
			defer requestCancel()
			_ = conn.SetDeadline(req.Deadline)
			closed := context.AfterFunc(requestCtx, func() { _ = conn.Close() })
			defer closed()
			go func() { var b [1]byte; _, _ = conn.Read(b[:]); requestCancel() }()
			if req.Kind == "voices" {
				if voices == nil {
					voices, err = catalog(requestCtx)
					if err != nil {
						_ = writeFrame(conn, 'e', []byte(err.Error()))
						return
					}
				}
				if warm == nil {
					warmVoice = "maple"
					warm = beginPreparation(ctx, warmVoice, prepare)
				}
				data, err := json.Marshal(voices)
				if err == nil {
					_ = writeFrame(conn, 'v', data)
				}
				return
			}
			if req.Kind != "speak" || validateText(req.Text) != nil {
				_ = writeFrame(conn, 'e', []byte("invalid voice worker narration"))
				return
			}
			if req.Voice == "" {
				req.Voice = "maple"
			}
			if warm != nil && warmVoice != req.Voice {
				warm.close()
				warm = nil
			}
			if warm == nil {
				warm = beginPreparation(ctx, req.Voice, prepare)
			}
			current := warm
			warm = nil
			defer current.close()
			select {
			case <-requestCtx.Done():
				return
			case <-current.done:
			}
			if current.err != nil {
				_ = writeFrame(conn, 'e', []byte(current.err.Error()))
				return
			}
			warmVoice = req.Voice
			warm = beginPreparation(ctx, warmVoice, prepare)
			result, err := current.voice.synthesize(requestCtx, req.Text, func(data []byte) error { return writeFrame(conn, 'p', data) })
			if err != nil {
				_ = writeFrame(conn, 'e', []byte(err.Error()))
				return
			}
			if err := writeFrame(conn, 'w', result.Audio); err == nil {
				_ = writeFrame(conn, 't', []byte(result.Transcript))
			}
		}()
	}
}

func writeFrame(w io.Writer, kind byte, data []byte) error {
	if len(data) > maxFrameBytes {
		return errors.New("voice worker frame exceeded size limit")
	}
	var header [5]byte
	header[0] = kind
	binary.BigEndian.PutUint32(header[1:], uint32(len(data)))
	for _, part := range [][]byte{header[:], data} {
		if n, err := w.Write(part); err != nil {
			return err
		} else if n != len(part) {
			return io.ErrShortWrite
		}
	}
	return nil
}

func readFrame(r io.Reader) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	size := binary.BigEndian.Uint32(header[1:])
	if size > maxFrameBytes {
		return 0, nil, errors.New("voice worker frame exceeded size limit")
	}
	data := make([]byte, size)
	_, err := io.ReadFull(r, data)
	return header[0], data, err
}
