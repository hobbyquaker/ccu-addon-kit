package litesource

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Meta is the metadata API: names, rooms and functions.
type Meta struct{ c *Client }

// Meta answers the metadata API's client.
func (c *Client) Meta() Meta { return Meta{c} }

// MetaObject is one object of the store: a device or a channel, keyed <interface>.<address>.
type MetaObject struct {
	Name     string                     `json:"name"`
	Enums    []string                   `json:"enums,omitempty"`
	Meta     map[string]json.RawMessage `json:"meta,omitempty"`
	Orphaned bool                       `json:"orphaned,omitempty"`
}

// Rooms answers the room paths of the object (every path under room/).
func (o MetaObject) Rooms() []string { return o.pathsOf("room/") }

// Functions answers the function paths.
func (o MetaObject) Functions() []string { return o.pathsOf("function/") }

func (o MetaObject) pathsOf(prefix string) []string {
	var out []string
	for _, e := range o.Enums {
		if strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}

// MetaText is a translated text of the metadata store: {"de": …, "en": …}, or one plain string
// (the favorite enum's nodes carry the user's own page name), kept under "".
type MetaText map[string]string

// UnmarshalJSON accepts a string or a map.
func (t *MetaText) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*t = MetaText{"": s}
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*t = m
	return nil
}

// In answers the text in lang, then en, then de, then any.
func (t MetaText) In(lang string) string {
	for _, l := range []string{lang, "en", "de", ""} {
		if s := t[l]; s != "" {
			return s
		}
	}
	for _, s := range t {
		return s
	}
	return ""
}

// MetaNode is a node of an enum tree.
type MetaNode struct {
	ID       string     `json:"id"`
	Name     MetaText   `json:"name"`
	Icon     string     `json:"icon,omitempty"`
	Children []MetaNode `json:"children,omitempty"`
}

// MetaEnum is one taxonomy with its tree.
type MetaEnum struct {
	Name MetaText   `json:"name"`
	Tree []MetaNode `json:"tree"`
}

// MetaSnapshot is GET /api/meta/v1/snapshot: the whole document.
type MetaSnapshot struct {
	Format   int                   `json:"format"`
	Revision int64                 `json:"revision"`
	Objects  map[string]MetaObject `json:"objects"`
	Enums    map[string]MetaEnum   `json:"enums"`
}

// Object answers the object of an interface and address.
func (s *MetaSnapshot) Object(iface, address string) (MetaObject, bool) {
	o, ok := s.Objects[iface+"."+address]
	return o, ok
}

// NodeName answers a node's name in lang (en behind it) by its path (room/eg/bad), the last
// path element when the tree does not have it.
func (s *MetaSnapshot) NodeName(path, lang string) string {
	enum, rest, _ := strings.Cut(path, "/")
	nodes := s.Enums[enum].Tree
	var found *MetaNode
	for _, id := range strings.Split(rest, "/") {
		found = nil
		for i := range nodes {
			if nodes[i].ID == id {
				found = &nodes[i]
				break
			}
		}
		if found == nil {
			return rest[strings.LastIndex(rest, "/")+1:]
		}
		nodes = found.Children
	}
	if found == nil {
		return rest
	}
	if n := found.Name.In(lang); n != "" {
		return n
	}
	return found.ID
}

// Snapshot reads the whole metadata document.
func (m Meta) Snapshot(ctx context.Context) (*MetaSnapshot, error) {
	var s MetaSnapshot
	err := m.c.do(ctx, http.MethodGet, "/api/meta/v1/snapshot", nil, &s)
	return &s, err
}

// ObjectPatch is the body of PATCH /api/meta/v1/objects/{ref} (occulited's meta-api.md): a
// field left nil is left alone.
type ObjectPatch struct {
	// Name sets the object's name (the store refuses an empty one).
	Name *string
	// Enums replaces the object's memberships (an empty slice: none).
	Enums *[]string
	// Meta is merged per namespace: a value replaces that namespace whole, nil removes it. An
	// addon writes its own namespace only (the token's meta:write scope).
	Meta map[string]any
}

// PatchObject creates or updates the object <iface>.<address> and answers the store's revision
// after the write and whether it changed anything: the store answers a write that changes
// nothing with 304, which is not an error here.
func (m Meta) PatchObject(ctx context.Context, iface, address string, p ObjectPatch) (revision int64, changed bool, err error) {
	body := map[string]any{}
	if p.Name != nil {
		body["name"] = *p.Name
	}
	if p.Enums != nil {
		enums := *p.Enums
		if enums == nil {
			enums = []string{}
		}
		body["enums"] = enums
	}
	if p.Meta != nil {
		body["meta"] = p.Meta
	}
	var res struct {
		Revision int64 `json:"revision"`
	}
	hdr, err := m.c.send(ctx, http.MethodPatch, "/api/meta/v1/objects/"+url.PathEscape(iface+"."+address), body, &res)
	var he *HTTPError
	if errors.As(err, &he) && he.Status == http.StatusNotModified {
		rev, _ := strconv.ParseInt(strings.Trim(hdr.Get("ETag"), `"`), 10, 64)
		return rev, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return res.Revision, true, nil
}

// MetaEvent is one event of the change stream. Kind resync means: read the snapshot again.
type MetaEvent struct {
	Revision int64           `json:"revision"`
	Kind     string          `json:"kind"`
	Ref      string          `json:"ref,omitempty"`
	Enum     string          `json:"enum,omitempty"`
	Path     string          `json:"path,omitempty"`
	From     string          `json:"from,omitempty"`
	To       string          `json:"to,omitempty"`
	Value    json.RawMessage `json:"value,omitempty"`
}

// Events follows GET /api/meta/v1/events/sse from a revision, reconnecting with backoff; a gap
// in the revisions, the server's resync and a reconnect each deliver a resync event, after which
// the consumer reads the snapshot again and continues from its revision (the client follows the
// revisions it sees on its own). The channel closes when ctx ends.
func (m Meta) Events(ctx context.Context, since int64) <-chan MetaEvent {
	out := make(chan MetaEvent, 64)
	go m.run(ctx, since, out)
	return out
}

func (m Meta) run(ctx context.Context, since int64, out chan<- MetaEvent) {
	defer close(out)
	c := m.c
	delay := c.Reconnect.Min
	first := true
	for ctx.Err() == nil {
		if !first {
			if !sendMeta(ctx, out, MetaEvent{Kind: "resync", Revision: since}) {
				return
			}
		}
		first = false
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/meta/v1/events/sse?since="+strconv.FormatInt(since, 10), nil)
		if err != nil {
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		res, err := c.http.Do(req)
		if err == nil && res.StatusCode != http.StatusOK {
			body, _ := readLimited(res.Body)
			res.Body.Close()
			err = &HTTPError{Status: res.StatusCode, Body: body}
		}
		if err == nil {
			delay = c.Reconnect.Min
			body := &watchdog{ctx: ctx, body: res.Body, silence: 90 * time.Second}
			body.start()
			_ = readSSE(body, func(sm SSEMessage) bool {
				body.alive()
				var ev MetaEvent
				if json.Unmarshal([]byte(sm.Data), &ev) != nil {
					return true
				}
				if ev.Kind != "resync" && ev.Revision > 0 {
					if since > 0 && ev.Revision > since+1 {
						if !sendMeta(ctx, out, MetaEvent{Kind: "resync", Revision: ev.Revision}) {
							return false
						}
					}
					since = ev.Revision
				}
				return sendMeta(ctx, out, ev)
			}, body.alive)
			body.stop()
			res.Body.Close()
			if ctx.Err() != nil {
				return
			}
			c.logf("litesource: the meta stream ended")
		} else {
			if ctx.Err() != nil {
				return
			}
			c.logf("litesource: the meta stream could not be opened: %v", err)
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

func sendMeta(ctx context.Context, out chan<- MetaEvent, e MetaEvent) bool {
	select {
	case out <- e:
		return true
	case <-ctx.Done():
		return false
	}
}
