package litesource

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// watchdog closes a stream body that said nothing for too long (a connection gone half-open),
// so the reader returns and the client reconnects.
type watchdog struct {
	ctx     context.Context
	body    io.ReadCloser
	silence time.Duration
	mu      sync.Mutex
	last    time.Time
	stopCh  chan struct{}
	timeout bool
}

var errSilent = errors.New("the stream was silent too long")

func (w *watchdog) start() {
	w.last = time.Now()
	w.stopCh = make(chan struct{})
	if w.silence <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(w.silence / 3)
		defer t.Stop()
		for {
			select {
			case <-w.stopCh:
				return
			case <-w.ctx.Done():
				w.body.Close()
				return
			case <-t.C:
				w.mu.Lock()
				quiet := time.Since(w.last) > w.silence
				if quiet {
					w.timeout = true
				}
				w.mu.Unlock()
				if quiet {
					w.body.Close()
					return
				}
			}
		}
	}()
}

func (w *watchdog) alive() {
	w.mu.Lock()
	w.last = time.Now()
	w.mu.Unlock()
}

func (w *watchdog) stop() {
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
}

func (w *watchdog) Read(p []byte) (int, error) {
	n, err := w.body.Read(p)
	if err != nil && err != io.EOF {
		w.mu.Lock()
		to := w.timeout
		w.mu.Unlock()
		if to {
			return n, errSilent
		}
	}
	return n, err
}

func readLimited(r io.Reader) (string, error) {
	b, err := io.ReadAll(io.LimitReader(r, 4096))
	return string(b), err
}

func isHTTP(err error, he **HTTPError) bool { return errors.As(err, he) }
