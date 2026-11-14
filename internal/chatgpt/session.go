// Package chatgpt provides experimental account-authenticated voice synthesis
// through the installed Codex app-server.
package chatgpt

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client uses a dedicated Codex home for ChatGPT authentication. It never reads
// the normal Codex login, and it never selects an API-key provider as a fallback.
type Client struct {
	Home   string
	Binary string
}

// NewClient resolves the optional sag-specific home and Codex executable.
func NewClient() (*Client, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	configHome := os.Getenv("SAG_CHATGPT_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config", "sag", "chatgpt")
	}
	binary := os.Getenv("SAG_CHATGPT_CODEX")
	if binary == "" {
		binary = "codex"
	}
	c := &Client{Home: configHome, Binary: binary}
	return c, c.validateHome()
}

func (c *Client) validateHome() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	for _, protected := range []string{filepath.Join(home, ".codex"), os.Getenv("CODEX_HOME")} {
		if protected == "" {
			continue
		}
		actual, err := canonicalPath(c.Home)
		if err != nil {
			return err
		}
		other, err := canonicalPath(protected)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(other, actual)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("SAG_CHATGPT_HOME must be outside your normal Codex home")
		}
	}
	return nil
}

// Resolve existing ancestors too, so a symlinked parent cannot alias Codex state.
func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return real, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if filepath.Dir(abs) == abs {
		return abs, nil
	}
	parent, err := canonicalPath(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

func isolatedEnv(home string) []string {
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "CODEX_HOME", "OPENAI_API_KEY", "OPENAI_API_KEY_FILE", "CODEX_API_KEY", "CODEX_ACCESS_TOKEN":
			continue
		}
		env = append(env, entry)
	}
	return append(env, "CODEX_HOME="+home)
}

func (c *Client) command(ctx context.Context, args ...string) (*exec.Cmd, error) {
	if err := c.validateHome(); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, c.Binary, append([]string{"-c", `cli_auth_credentials_store="file"`, "-c", `forced_login_method="chatgpt"`}, args...)...)
	cmd.Env = isolatedEnv(c.Home)
	cmd.Dir = c.Home
	return cmd, nil
}

// Login runs OpenAI's own sign-in flow in the sag-only credential directory.
func (c *Client) Login(ctx context.Context, device bool, in io.Reader, out, stderr io.Writer) error {
	if err := c.validateHome(); err != nil {
		return err
	}
	if err := os.MkdirAll(c.Home, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(c.Home, 0o700); err != nil {
		return err
	}
	args := []string{"login"}
	if device {
		args = append(args, "--device-auth")
	}
	cmd, err := c.command(ctx, args...)
	if err != nil {
		return err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, stderr
	return cmd.Run()
}

type rpcMessage struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type session struct {
	in        io.WriteCloser
	messages  chan rpcMessage
	readError chan error
	pending   []rpcMessage
	id        int
	cancel    context.CancelFunc
	cmd       *exec.Cmd
}

func (c *Client) start(ctx context.Context) (*session, error) {
	if _, err := os.Stat(c.Home); err != nil {
		return nil, errors.New("ChatGPT sign-in missing; run sag login --provider chatgpt --device-auth")
	}
	ctx, cancel := context.WithCancel(ctx)
	cmd, err := c.command(ctx, "--enable", "realtime_conversation", "app-server", "--stdio")
	if err != nil {
		cancel()
		return nil, err
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = in.Close()
		return nil, err
	}
	// Protocol errors are surfaced over stdout. Raw diagnostic logs may contain
	// account metadata and must not become a second error-output channel.
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		cancel()
		_ = in.Close()
		return nil, fmt.Errorf("start Codex voice bridge (requires Codex 0.169.0): %w", err)
	}
	s := &session{in: in, messages: make(chan rpcMessage, 128), readError: make(chan error, 1), cancel: cancel, cmd: cmd}
	go func() {
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 65536), 4<<20)
		for scanner.Scan() {
			var m rpcMessage
			if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
				s.readError <- errors.New("invalid Codex voice bridge response")
				return
			}
			select {
			case s.messages <- m:
			case <-ctx.Done():
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		s.readError <- fmt.Errorf("voice bridge stopped: %w", err)
	}()
	if err := s.call(ctx, "initialize", map[string]any{"clientInfo": map[string]any{"name": "sag", "version": "0.4.6"}, "capabilities": map[string]any{"experimentalApi": true}}, nil); err != nil {
		s.close()
		return nil, err
	}
	if err := json.NewEncoder(in).Encode(map[string]any{"method": "initialized"}); err != nil {
		s.close()
		return nil, err
	}
	var account struct {
		Account struct {
			Type string `json:"type"`
		} `json:"account"`
	}
	if err := s.call(ctx, "account/read", map[string]any{}, &account); err != nil {
		s.close()
		return nil, err
	}
	if account.Account.Type != "chatgpt" {
		s.close()
		return nil, errors.New("ChatGPT sign-in required; run sag login --provider chatgpt --device-auth (API keys cannot use this provider)")
	}
	return s, nil
}

func (s *session) close() { _ = s.in.Close(); s.cancel(); _ = s.cmd.Wait() }

func (s *session) receive(ctx context.Context) (rpcMessage, error) {
	select {
	case m := <-s.messages:
		return m, nil
	case err := <-s.readError:
		return rpcMessage{}, err
	case <-ctx.Done():
		return rpcMessage{}, ctx.Err()
	}
}

func (s *session) next(ctx context.Context) (rpcMessage, error) {
	if len(s.pending) > 0 {
		m := s.pending[0]
		s.pending = s.pending[1:]
		return m, nil
	}
	return s.receive(ctx)
}

func (s *session) call(ctx context.Context, method string, params, result any) error {
	s.id++
	id := s.id
	if err := json.NewEncoder(s.in).Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		m, err := s.receive(ctx)
		if err != nil {
			return err
		}
		if m.ID != id {
			if len(s.pending) >= 128 {
				return errors.New("voice bridge event queue exceeded limit")
			}
			s.pending = append(s.pending, m)
			continue
		}
		if m.Error != nil {
			return fmt.Errorf("%s: %s", method, m.Error.Message)
		}
		if result != nil {
			return json.Unmarshal(m.Result, result)
		}
		return nil
	}
}
