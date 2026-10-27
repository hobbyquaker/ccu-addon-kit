package litesource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Filter narrows the event stream and the state read: AND across kinds, OR within one. A device
// address matches its channels. Keys apply to event and state messages only.
type Filter struct {
	Interfaces []string
	Addresses  []string
	Keys       []string
	Types      []string
	// Devices asks the stream for one newDevices message per interface with the device
	// descriptions (a listDevices each) before the live events.
	Devices bool
}

func (f Filter) query() url.Values {
	q := url.Values{}
	if len(f.Interfaces) > 0 {
		q.Set("interface", strings.Join(f.Interfaces, ","))
	}
	if len(f.Addresses) > 0 {
		q.Set("address", strings.Join(f.Addresses, ","))
	}
	if len(f.Keys) > 0 {
		q.Set("key", strings.Join(f.Keys, ","))
	}
	if len(f.Types) > 0 {
		q.Set("type", strings.Join(f.Types, ","))
	}
	if f.Devices {
		q.Set("devices", "1")
	}
	return q
}

// Event is a datapoint report: an `event` of the stream, a `state` message (the store's sweep)
// or an entry of GET /state.
type Event struct {
	Interface string          `json:"interface"`
	Address   string          `json:"address"`
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	TS        time.Time       `json:"ts"`
	// LC is since when the value is the current one; zero when the stream did not say.
	LC time.Time `json:"lc,omitempty"`
	// Confirmed is false for an entry restored from occulited's file after a restart until an
	// event or the sweep confirmed it; such a value is a seed, never a change.
	Confirmed bool `json:"confirmed"`
	// Source is event, sweep or restored (the state store's words); "" on a live event.
	Source string `json:"source,omitempty"`
	// Batch is the seq of the first event of the multicall this one came in; 0 when none.
	Batch int64 `json:"batch,omitempty"`
	// PreviousForS is how long the previous value stood, when the store knows it.
	PreviousForS float64 `json:"previous_for_s,omitempty"`
	// ChangedAt is when the store saw the change happen; zero when not known.
	ChangedAt time.Time `json:"changed_at,omitempty"`
	// Previous is the value before LC, from GET /state only.
	Previous json.RawMessage `json:"previous,omitempty"`
}

// Device is the device address of a channel address ("ABC123:1" -> "ABC123").
func (e Event) Device() string { return deviceOf(e.Address) }

// Channel is the channel index, -1 for a device address.
func (e Event) Channel() int {
	_, ch, ok := strings.Cut(e.Address, ":")
	if !ok {
		return -1
	}
	n := 0
	for _, r := range ch {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func deviceOf(address string) string {
	dev, _, _ := strings.Cut(address, ":")
	return dev
}

// Float answers the value as a float64 when it is a number (ints and doubles both), else false.
func (e Event) Float() (float64, bool) {
	var f float64
	if err := json.Unmarshal(e.Value, &f); err != nil {
		return 0, false
	}
	return f, true
}

// Bool answers the value as a bool when it is one.
func (e Event) Bool() (bool, bool) {
	var b bool
	if err := json.Unmarshal(e.Value, &b); err != nil {
		return false, false
	}
	return b, true
}

// String answers a string value, or the JSON text of any other.
func (e Event) String() string {
	var s string
	if err := json.Unmarshal(e.Value, &s); err == nil {
		return s
	}
	return string(e.Value)
}

// Hello is the stream's first message.
type Hello struct {
	BootID     string `json:"boot_id"`
	Seq        int64  `json:"seq"`
	Interfaces []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"interfaces"`
	Buffer struct {
		Seconds int `json:"seconds"`
		Events  int `json:"events"`
	} `json:"buffer"`
}

// DeviceDescription is an interface process's device description, the fields an addon reads;
// Raw keeps the whole.
type DeviceDescription struct {
	Address   string          `json:"ADDRESS"`
	Type      string          `json:"TYPE"`
	Parent    string          `json:"PARENT"`
	Children  []string        `json:"CHILDREN"`
	Firmware  string          `json:"FIRMWARE"`
	Version   int             `json:"VERSION"`
	Paramsets []string        `json:"PARAMSETS"`
	Interface string          `json:"INTERFACE"`
	Raw       json.RawMessage `json:"-"`
}

// StringList is a list that an interface process may send as a string: the VirtualDevices
// process answers CHILDREN as "" for a device without children (and a single name as a plain
// string is conceivable), where rfd and hmipserver send an array. "" is the empty list, a
// string one entry, an array itself.
type StringList []string

// UnmarshalJSON takes an array, a string or null.
func (l *StringList) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*l = []string{}
		} else {
			*l = []string{s}
		}
		return nil
	}
	if string(b) == "null" {
		*l = nil
		return nil
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	*l = arr
	return nil
}

// UnmarshalJSON keeps the raw description beside the typed fields; the list fields take a string
// as well as an array (StringList), and a VERSION that comes as a string or a float is read too.
func (d *DeviceDescription) UnmarshalJSON(b []byte) error {
	var p struct {
		Address   string          `json:"ADDRESS"`
		Type      string          `json:"TYPE"`
		Parent    string          `json:"PARENT"`
		Children  StringList      `json:"CHILDREN"`
		Firmware  string          `json:"FIRMWARE"`
		Version   json.RawMessage `json:"VERSION"`
		Paramsets StringList      `json:"PARAMSETS"`
		Interface string          `json:"INTERFACE"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*d = DeviceDescription{Address: p.Address, Type: p.Type, Parent: p.Parent, Children: p.Children, Firmware: p.Firmware, Paramsets: p.Paramsets, Interface: p.Interface}
	if len(p.Version) > 0 && string(p.Version) != "null" {
		var f float64
		if err := json.Unmarshal(p.Version, &f); err == nil {
			d.Version = int(f)
		} else {
			var s string
			if json.Unmarshal(p.Version, &s) == nil {
				var n int
				for _, c := range s {
					if c < '0' || c > '9' {
						break
					}
					n = n*10 + int(c-'0')
				}
				d.Version = n
			}
		}
	}
	d.Raw = append(json.RawMessage(nil), b...)
	return nil
}

// DeviceChange is newDevices, deleteDevices, readdedDevice, replaceDevice or updateDevice.
type DeviceChange struct {
	Interface string              `json:"interface"`
	Addresses []string            `json:"addresses"`
	Devices   []DeviceDescription `json:"devices"` // the ?devices=1 snapshot only
	Hint      int                 `json:"hint"`
	Old       string              `json:"old"`
	New       string              `json:"new"`
	TS        time.Time           `json:"ts"`
}

// InterfaceChange is an `interface` message: up, down, restarted, added, removed.
type InterfaceChange struct {
	Interface string    `json:"interface"`
	State     string    `json:"state"`
	TS        time.Time `json:"ts"`
}

// Resync tells the consumer the stream lost its place: gap (the ring did not reach back to the
// last id), boot (occulited restarted), overflow (the consumer read too slowly), or
// reconnect (the client reconnected without a last id, so values in between may be missing).
// The consumer reads State() again and goes on.
type Resync struct {
	Reason string `json:"reason"`
}

// Message is one message of the stream. Exactly one of the typed fields is set, by Type.
type Message struct {
	// Type is hello, event, state, interface, newDevices, deleteDevices, readdedDevice,
	// replaceDevice, updateDevice, firmware, resync, or another name occulited adds later (then
	// only Raw is set).
	Type string
	// ID is the message's id (<boot_id>-<seq>); "" for resync and the devices snapshot.
	ID        string
	Event     *Event
	Hello     *Hello
	Devices   *DeviceChange
	Interface *InterfaceChange
	Resync    *Resync
	Raw       json.RawMessage
}

// Events opens GET /api/rpc/v1/events and keeps it open: it reconnects with backoff when the
// stream ends, resumes with Last-Event-ID, and delivers a Resync message whenever values may have
// been missed (the server's own resync, a hello with another boot id, a reconnect without an id to
// resume from). The channel closes when ctx ends. lastEventID resumes from a stored position
// (GET /state's event_id, or the last ID seen); "" starts live.
func (c *Client) Events(ctx context.Context, f Filter, lastEventID string) <-chan Message {
	out := make(chan Message, 256)
	go c.runStream(ctx, f, lastEventID, out)
	return out
}

func (c *Client) runStream(ctx context.Context, f Filter, lastID string, out chan<- Message) {
	defer close(out)
	delay := c.Reconnect.Min
	var bootID string
	first := true
	for ctx.Err() == nil {
		if !first && lastID == "" {
			// nothing to resume from: whatever happened in between is gone
			if !send(ctx, out, Message{Type: "resync", Resync: &Resync{Reason: "reconnect"}}) {
				return
			}
		}
		first = false
		q := f.query()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/rpc/v1/events?"+q.Encode(), nil)
		if err != nil {
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		if lastID != "" {
			req.Header.Set("Last-Event-ID", lastID)
		}
		res, err := c.http.Do(req)
		if err == nil && res.StatusCode != http.StatusOK {
			body, _ := readLimited(res.Body)
			res.Body.Close()
			err = &HTTPError{Status: res.StatusCode, Body: body}
		}
		if err == nil {
			delay = c.Reconnect.Min
			body := &watchdog{ctx: ctx, body: res.Body, silence: c.Silence}
			body.start()
			// one re-read per connection: the hello of another boot and the server's own resync
			// (boot, gap) come together at the start of a stream, and the consumer reads State once
			resynced := false
			streamErr := readSSE(body, func(m SSEMessage) bool {
				body.alive()
				msg, ok := decode(m)
				if !ok {
					return ctx.Err() == nil
				}
				if msg.Type == "hello" && msg.Hello != nil {
					if bootID != "" && msg.Hello.BootID != bootID && !resynced {
						resynced = true
						if !send(ctx, out, Message{Type: "resync", Resync: &Resync{Reason: "boot"}}) {
							return false
						}
					}
					bootID = msg.Hello.BootID
				}
				if msg.Type == "resync" && msg.Resync != nil && msg.Resync.Reason != "overflow" {
					if resynced {
						return true
					}
					resynced = true
				}
				if m.ID != "" {
					lastID = m.ID
					if msg.Type != "hello" {
						resynced = false // a real message: a later resync (overflow) is news again
					}
				}
				return send(ctx, out, msg)
			}, body.alive)
			body.stop()
			res.Body.Close()
			if ctx.Err() != nil {
				return
			}
			if streamErr != nil {
				c.logf("litesource: the event stream ended: %v", streamErr)
			} else {
				c.logf("litesource: the event stream ended")
			}
		} else {
			if ctx.Err() != nil {
				return
			}
			var he *HTTPError
			if isHTTP(err, &he) && (he.Status == http.StatusUnauthorized || he.Status == http.StatusForbidden) {
				c.logf("litesource: the event stream was refused: %v (retrying)", err)
			} else {
				c.logf("litesource: the event stream could not be opened: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay *= 2
		if delay > c.Reconnect.Max {
			delay = c.Reconnect.Max
		}
	}
}

func send(ctx context.Context, out chan<- Message, m Message) bool {
	select {
	case out <- m:
		return true
	case <-ctx.Done():
		return false
	}
}

func decode(m SSEMessage) (Message, bool) {
	msg := Message{Type: m.Event, ID: m.ID, Raw: json.RawMessage(m.Data)}
	var err error
	switch m.Event {
	case "hello":
		msg.Hello = &Hello{}
		err = json.Unmarshal(msg.Raw, msg.Hello)
	case "event":
		msg.Event = &Event{}
		err = json.Unmarshal(msg.Raw, msg.Event)
		if err == nil && msg.Event.Source == "" {
			msg.Event.Source = "event"
			if !msg.Event.Confirmed && msg.Event.LC.IsZero() {
				msg.Event.Confirmed = true // a live event without store fields is what the device said
			}
		}
	case "state":
		var s struct {
			Event
			Datapoint string `json:"datapoint"`
		}
		err = json.Unmarshal(msg.Raw, &s)
		if err == nil {
			if s.Key == "" {
				s.Key = s.Datapoint
			}
			e := s.Event
			msg.Event = &e
		}
	case "interface":
		msg.Interface = &InterfaceChange{}
		err = json.Unmarshal(msg.Raw, msg.Interface)
	case "newDevices", "deleteDevices", "readdedDevice", "replaceDevice", "updateDevice":
		msg.Devices = &DeviceChange{}
		err = json.Unmarshal(msg.Raw, msg.Devices)
	case "resync":
		msg.Resync = &Resync{}
		err = json.Unmarshal(msg.Raw, msg.Resync)
	}
	return msg, err == nil
}
