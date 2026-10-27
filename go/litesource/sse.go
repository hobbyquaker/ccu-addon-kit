package litesource

import (
	"bufio"
	"io"
	"strings"
)

// SSEMessage is one Server-Sent Events message: id, event and the joined data lines.
type SSEMessage struct {
	ID    string
	Event string
	Data  string
}

// readSSE parses a text/event-stream body into messages; onComment is called for every comment
// line (": ping"), which counts as life. It returns when the stream ends or fails.
func readSSE(r io.Reader, emit func(SSEMessage) bool, onComment func()) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var id, event string
	var data []string
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			if len(data) > 0 {
				ev := event
				if ev == "" {
					ev = "message"
				}
				if !emit(SSEMessage{ID: id, Event: ev, Data: strings.Join(data, "\n")}) {
					return nil
				}
			}
			id, event, data = "", "", nil
			continue
		}
		if strings.HasPrefix(line, ":") {
			if onComment != nil {
				onComment()
			}
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "data":
			data = append(data, value)
		case "event":
			event = value
		case "id":
			id = value
		}
	}
	return sc.Err()
}
