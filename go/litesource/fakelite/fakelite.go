// Package fakelite is a fake openccu-lite for tests: an httptest server that answers the routes
// litesource speaks — the state store, the event stream (SSE with ids, hello, ping, resync and
// resume by Last-Event-ID), JSON-RPC listDevices and getParamset, the paramset cache, the
// metadata snapshot and change stream, and /api/auth/v1/state for addonkit's session check. A
// test fills it and pushes events; the addon under test sees a system.
package fakelite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Session is one session /api/auth/v1/state confirms.
type Session struct {
	User  string
	Level string
}

// Server is the fake.
type Server struct {
	*httptest.Server
	mu sync.Mutex
	// Token is the credential every API call must carry (Bearer); "" accepts anything.
	Token string
	// Sessions maps a session id to what /state answers for it.
	Sessions map[string]Session
	// State is what GET /state answers, in order.
	State []map[string]any
	// EventID is /state's event_id.
	EventID string
	// Devices is listDevices per interface.
	Devices map[string][]map[string]any
	// Paramsets is the cache (GET /paramsets) and the values of getParamset by "<iface>/<address>/<paramset>".
	Paramsets map[string]map[string]any
	Values    map[string]map[string]any
	// Meta is the snapshot; MetaEvents what the change stream replays after `since`.
	Meta       map[string]any
	MetaEvents []map[string]any
	// Interfaces is GET /interfaces and the hello's list.
	Interfaces []string
	BootID     string
	seq        int64
	ring       []sseMsg
	subs       map[chan sseMsg]struct{}
	Calls      []string // every JSON-RPC method called, "<iface> <method> <params>"
	// Ping is the stream's heartbeat interval; 0 means none.
	Ping time.Duration
}

type sseMsg struct {
	id, event, data string
}

// New starts a fake with one interface and no devices.
func New() *Server {
	s := &Server{Sessions: map[string]Session{}, Devices: map[string][]map[string]any{}, Paramsets: map[string]map[string]any{},
		Values: map[string]map[string]any{}, Interfaces: []string{"HmIP-RF"}, BootID: "boot1", subs: map[chan sseMsg]struct{}{},
		Meta: map[string]any{"format": 1, "revision": 1, "objects": map[string]any{}, "enums": map[string]any{}}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/meta/v1/version", s.version)
	mux.HandleFunc("/api/auth/v1/state", s.authState)
	mux.HandleFunc("/api/rpc/v1/interfaces", s.auth(s.interfaces))
	mux.HandleFunc("/api/rpc/v1/state", s.auth(s.state))
	mux.HandleFunc("/api/rpc/v1/events", s.auth(s.events))
	mux.HandleFunc("/api/rpc/v1/paramsets", s.auth(s.paramsets))
	mux.HandleFunc("/api/rpc/v1/paramsets/", s.auth(s.paramsetsFor))
	mux.HandleFunc("/api/rpc/v1/json/", s.auth(s.jsonrpc))
	mux.HandleFunc("/api/meta/v1/snapshot", s.auth(s.snapshot))
	mux.HandleFunc("/api/meta/v1/events/sse", s.auth(s.metaEvents))
	s.Server = httptest.NewServer(mux)
	return s
}

func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		tok := s.Token
		s.mu.Unlock()
		if tok != "" && r.Header.Get("Authorization") != "Bearer "+tok {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":"unauthenticated"}`)
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"api": "meta", "version": 1, "format": 1, "revision": 1, "implementation": "fakelite",
		"capabilities": map[string]any{"state": true, "history": true, "apis": map[string]int{"meta": 1, "rpc": 1, "system": 1, "auth": 1}}})
}

func (s *Server) authState(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	s.mu.Lock()
	sess, ok := s.Sessions[id]
	tok := s.Token
	s.mu.Unlock()
	if ok {
		role := "user"
		if sess.Level == "administer" {
			role = "admin"
		}
		writeJSON(w, map[string]any{"authenticated": true, "sid": id, "user": sess.User, "level": sess.Level, "role": role})
		return
	}
	if tok != "" && id == tok {
		writeJSON(w, map[string]any{"authenticated": true, "user": "token:test", "scopes": []string{"rpc:read"}})
		return
	}
	w.WriteHeader(401)
	writeJSON(w, map[string]any{"authenticated": false})
}

func (s *Server) interfaces(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	for _, i := range s.Interfaces {
		out = append(out, map[string]any{"name": i, "protocol": "xmlrpc", "url_path": "/api/rpc/v1/xmlrpc/" + i, "running": true})
	}
	writeJSON(w, out)
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := splitQ(r.URL.Query()["datapoint"], r.URL.Query()["key"])
	ifaces := splitQ(r.URL.Query()["interface"])
	var entries []map[string]any
	for _, e := range s.State {
		if len(keys) > 0 && !contains(keys, str(e["datapoint"])) {
			continue
		}
		if len(ifaces) > 0 && !contains(ifaces, str(e["interface"])) {
			continue
		}
		entries = append(entries, e)
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	page := map[string]any{"total": len(entries), "unconfirmed": 0, "event_id": s.EventID, "sweeps": map[string]any{}}
	end := after + limit
	if end > len(entries) {
		end = len(entries)
	}
	if entries == nil {
		entries = []map[string]any{}
	}
	page["entries"] = entries[after:end]
	if end < len(entries) {
		page["next"] = strconv.Itoa(end)
	}
	writeJSON(w, page)
}

func splitQ(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		for _, v := range l {
			for _, x := range strings.Split(v, ",") {
				if x != "" {
					out = append(out, x)
				}
			}
		}
	}
	return out
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// Push sends one message on the stream with the next id; data is marshalled. It is kept in the
// ring for resume.
func (s *Server) Push(event string, data any) string {
	b, _ := json.Marshal(data)
	s.mu.Lock()
	s.seq++
	id := s.BootID + "-" + strconv.FormatInt(s.seq, 10)
	m := sseMsg{id: id, event: event, data: string(b)}
	s.ring = append(s.ring, m)
	if len(s.ring) > 5000 {
		s.ring = s.ring[len(s.ring)-5000:]
	}
	s.EventID = id
	for ch := range s.subs {
		select {
		case ch <- m:
		default:
		}
	}
	s.mu.Unlock()
	return id
}

// PushEvent is Push of an `event` message for a datapoint.
func (s *Server) PushEvent(iface, address, key string, value any) string {
	return s.Push("event", map[string]any{"interface": iface, "address": address, "key": key, "value": value, "ts": time.Now().UTC().Format(time.RFC3339Nano)})
}

// Resync sends a resync message (no id) to every stream.
func (s *Server) Resync(reason string) {
	s.mu.Lock()
	m := sseMsg{event: "resync", data: `{"reason":"` + reason + `"}`}
	for ch := range s.subs {
		select {
		case ch <- m:
		default:
		}
	}
	s.mu.Unlock()
}

// Reboot changes the boot id and drops the ring and every open stream, as a restart does.
func (s *Server) Reboot(bootID string) {
	s.mu.Lock()
	s.BootID = bootID
	s.seq = 0
	s.ring = nil
	for ch := range s.subs {
		close(ch)
		delete(s.subs, ch)
	}
	s.mu.Unlock()
}

// Drop ends every open stream without changing anything, as a network hiccup does.
func (s *Server) Drop() {
	s.mu.Lock()
	for ch := range s.subs {
		close(ch)
		delete(s.subs, ch)
	}
	s.mu.Unlock()
}

// Streams counts the open event streams.
func (s *Server) Streams() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(500)
		return
	}
	keys := splitQ(r.URL.Query()["key"], r.URL.Query()["datapoint"])
	last := r.Header.Get("Last-Event-ID")
	if last == "" {
		last = r.URL.Query().Get("last_event_id")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	fmt.Fprint(w, ": connected\n\n")
	ch := make(chan sseMsg, 1024)
	s.mu.Lock()
	var ifaces []map[string]any
	for _, i := range s.Interfaces {
		ifaces = append(ifaces, map[string]any{"name": i, "state": "up"})
	}
	hello, _ := json.Marshal(map[string]any{"boot_id": s.BootID, "seq": s.seq, "interfaces": ifaces, "buffer": map[string]int{"seconds": 300, "events": 5000}})
	s.seq++
	helloID := s.BootID + "-" + strconv.FormatInt(s.seq, 10)
	var replay []sseMsg
	var resync string
	if last != "" {
		// ids are <boot>-<seq>; a position is a seq, which need not be a message's own (GET
		// /state's event_id is the stream's position, taken between messages), so the replay is
		// everything with a higher seq, and a gap is a ring that does not reach back that far
		boot, seqStr, _ := strings.Cut(last, "-")
		lastSeq, perr := strconv.ParseInt(seqStr, 10, 64)
		switch {
		case boot != s.BootID:
			resync = "boot"
		case perr != nil:
			resync = "gap"
		default:
			if len(s.ring) > 0 && seqOf(s.ring[0].id) > lastSeq+1 {
				resync = "gap"
			}
			for _, m := range s.ring {
				if seqOf(m.id) > lastSeq {
					replay = append(replay, m)
				}
			}
		}
	}
	if r.URL.Query().Get("devices") == "1" {
		for iface, devs := range s.Devices {
			b, _ := json.Marshal(map[string]any{"interface": iface, "devices": devs})
			replay = append([]sseMsg{{event: "newDevices", data: string(b)}}, replay...)
		}
	}
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
		s.mu.Unlock()
	}()
	write := func(m sseMsg) bool {
		if (m.event == "event" || m.event == "state") && len(keys) > 0 {
			var d map[string]any
			json.Unmarshal([]byte(m.data), &d)
			k := str(d["key"])
			if k == "" {
				k = str(d["datapoint"])
			}
			if !contains(keys, k) {
				return true
			}
		}
		if m.id != "" {
			fmt.Fprintf(w, "id: %s\n", m.id)
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", m.event, m.data)
		fl.Flush()
		return true
	}
	write(sseMsg{id: helloID, event: "hello", data: string(hello)})
	if resync != "" {
		write(sseMsg{event: "resync", data: `{"reason":"` + resync + `"}`})
	}
	for _, m := range replay {
		write(m)
	}
	var ping <-chan time.Time
	if s.Ping > 0 {
		t := time.NewTicker(s.Ping)
		defer t.Stop()
		ping = t.C
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case m, ok := <-ch:
			if !ok {
				return
			}
			write(m)
		}
	}
}

func seqOf(id string) int64 {
	_, seqStr, _ := strings.Cut(id, "-")
	n, _ := strconv.ParseInt(seqStr, 10, 64)
	return n
}

func (s *Server) paramsets(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, s.Paramsets)
}

func (s *Server) paramsetsFor(w http.ResponseWriter, r *http.Request) {
	iface := strings.TrimPrefix(r.URL.Path, "/api/rpc/v1/paramsets/")
	var body struct {
		Addresses []string `json:"addresses"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := map[string]string{}
	sets := map[string]map[string]any{}
	errs := map[string]string{}
	for _, a := range body.Addresses {
		k := s.keyOf(iface, a)
		if k == "" {
			errs[a] = "unknown"
			continue
		}
		keys[a] = k
		sets[k] = s.Paramsets[k]
	}
	writeJSON(w, map[string]any{"keys": keys, "paramsets": sets, "errors": errs})
}

// keyOf finds the cache key of an address from the device list: <iface>/<type>/<fw>/<ver>/<chtype>/VALUES.
func (s *Server) keyOf(iface, address string) string {
	for _, d := range s.Devices[iface] {
		if str(d["ADDRESS"]) != address {
			continue
		}
		dev, _, isChannel := strings.Cut(address, ":")
		typ, fw, ver := str(d["TYPE"]), str(d["FIRMWARE"]), fmt.Sprint(d["VERSION"])
		chType := ""
		if isChannel {
			chType = typ
			for _, p := range s.Devices[iface] {
				if str(p["ADDRESS"]) == dev {
					typ, fw, ver = str(p["TYPE"]), str(p["FIRMWARE"]), fmt.Sprint(p["VERSION"])
				}
			}
		}
		k := strings.Join([]string{iface, typ, fw, ver, chType, "VALUES"}, "/")
		if _, ok := s.Paramsets[k]; ok {
			return k
		}
		return ""
	}
	return ""
}

func (s *Server) jsonrpc(w http.ResponseWriter, r *http.Request) {
	iface := strings.TrimPrefix(r.URL.Path, "/api/rpc/v1/json/")
	var req struct {
		Method string `json:"method"`
		Params []any  `json:"params"`
		ID     any    `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls = append(s.Calls, fmt.Sprintf("%s %s %v", iface, req.Method, req.Params))
	answer := func(result any, rpcErr map[string]any) {
		out := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if rpcErr != nil {
			out["error"] = rpcErr
		} else {
			out["result"] = result
		}
		writeJSON(w, out)
	}
	if !contains(s.Interfaces, iface) {
		w.WriteHeader(404)
		fmt.Fprint(w, `{"error":"unknown-interface"}`)
		return
	}
	switch req.Method {
	case "listDevices":
		devs := s.Devices[iface]
		if devs == nil {
			devs = []map[string]any{}
		}
		answer(devs, nil)
	case "getParamset":
		if len(req.Params) != 2 {
			answer(nil, map[string]any{"code": -32602, "message": "params"})
			return
		}
		v, ok := s.Values[iface+"/"+str(req.Params[0])+"/"+str(req.Params[1])]
		if !ok {
			answer(nil, map[string]any{"code": -1, "message": "Failure"})
			return
		}
		answer(v, nil)
	case "init":
		answer(nil, map[string]any{"code": -32601, "message": "init is not available remotely"})
	default:
		answer(nil, map[string]any{"code": -32601, "message": "unknown method " + req.Method})
	}
}

func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, s.Meta)
}

// SetName sets an object's name and rooms in the snapshot and bumps the revision.
func (s *Server) SetName(ref, name string, enums ...string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	objs := s.Meta["objects"].(map[string]any)
	o := map[string]any{"name": name}
	if enums != nil {
		o["enums"] = enums
	}
	objs[ref] = o
	rev := toInt(s.Meta["revision"]) + 1
	s.Meta["revision"] = rev
	ev := map[string]any{"revision": rev, "kind": "object.updated", "ref": ref, "value": o}
	s.MetaEvents = append(s.MetaEvents, ev)
	return rev
}

func toInt(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	}
	return 0
}

func (s *Server) metaEvents(w http.ResponseWriter, r *http.Request) {
	fl := w.(http.Flusher)
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	// replay what came after `since`, then poll for new events (the fake has no subscriber list here)
	for {
		s.mu.Lock()
		var replay []map[string]any
		for _, e := range s.MetaEvents {
			if toInt(e["revision"]) > since {
				replay = append(replay, e)
				since = toInt(e["revision"])
			}
		}
		s.mu.Unlock()
		for _, e := range replay {
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		fl.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-time.After(20 * time.Millisecond):
		}
	}
}
