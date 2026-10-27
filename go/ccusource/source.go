package ccusource

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"
)

// DefaultInterfacesList is the CCU's list of interface processes.
const DefaultInterfacesList = "/etc/config/InterfacesList.xml"

// Options configure the source.
type Options struct {
	// ID names the addon in the daemons' callback registration ("battery").
	ID string
	// CallbackPort is the loopback port the daemons send events to.
	CallbackPort int
	// InterfacesList is the CCU's file; DefaultInterfacesList by default. Interfaces adds the
	// well-known radio processes when the file lacks them (a CCU without one simply does not
	// answer on its port).
	InterfacesList string
	// Rega is rega.exe's URL, http://127.0.0.1:8181/rega.exe by default.
	Rega string
	// SweepEvery is how often the HmIP devices' channel 0 is read from the process's cache for
	// the ones that report rarely; 15 min by default, negative never.
	SweepEvery time.Duration
	// ReinitEvery is how often the callback registration is renewed; 10 min by default.
	ReinitEvery time.Duration
	// NamesEvery is how often the names are read from ReGa to notice a change; 10 min.
	NamesEvery time.Duration
	Log        func(string, ...any)
	Client     *http.Client
}

// Source implements litesource.Source over a classic CCU.
type Source struct {
	o         Options
	logf      func(string, ...any)
	mu        sync.Mutex
	ifaces    []iface
	descs     map[string][]litesource.DeviceDescription // by interface
	lastNames string
	// only are the interfaces the subscription wants (Filter.Interfaces); nil: all
	only map[string]bool
}

type iface struct {
	litesource.Interface
	url    string
	client *Client
}

// New makes a source; nothing runs until Subscribe.
func New(o Options) *Source {
	if o.InterfacesList == "" {
		o.InterfacesList = DefaultInterfacesList
	}
	if o.Rega == "" {
		o.Rega = "http://127.0.0.1:8181/rega.exe"
	}
	if o.SweepEvery == 0 {
		o.SweepEvery = 15 * time.Minute
	}
	if o.ReinitEvery == 0 {
		o.ReinitEvery = 10 * time.Minute
	}
	if o.NamesEvery == 0 {
		o.NamesEvery = 10 * time.Minute
	}
	if o.Log == nil {
		o.Log = log.Printf
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 60 * time.Second}
	}
	if o.ID == "" {
		o.ID = "addon"
	}
	return &Source{o: o, logf: o.Log, descs: map[string][]litesource.DeviceDescription{}}
}

var _ litesource.Source = (*Source)(nil)

// --- the interfaces --------------------------------------------------------------------

// wellKnown are the radio processes' public XML-RPC ports when InterfacesList.xml names them
// with a binrpc URL (rfd's xmlrpc_bin://…:32001) or not at all.
var wellKnown = map[string]string{
	"BidCos-RF":    "http://127.0.0.1:2001",
	"BidCos-Wired": "http://127.0.0.1:2000",
	"HmIP-RF":      "http://127.0.0.1:2010",
}

// ParseInterfacesList reads the CCU's InterfacesList.xml: name -> URL.
func ParseInterfacesList(b []byte) (map[string]string, error) {
	var f struct {
		IPC []struct {
			Name string `xml:"name"`
			URL  string `xml:"url"`
		} `xml:"ipc"`
	}
	if err := xml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, i := range f.IPC {
		n, u := strings.TrimSpace(i.Name), strings.TrimSpace(i.URL)
		if n == "" || u == "" {
			continue
		}
		out[n] = u
	}
	return out, nil
}

// httpURL turns the list's URL into the XML-RPC endpoint: xmlrpc:// becomes http://, a binrpc
// URL (xmlrpc_bin://) the process's well-known XML-RPC port, anything else is skipped ("").
func httpURL(name, raw string) string {
	// url.Parse refuses the underscore in xmlrpc_bin://, so the scheme is cut by hand
	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return ""
	}
	switch strings.ToLower(scheme) {
	case "xmlrpc", "http":
		if _, err := url.Parse("http://" + rest); err != nil {
			return ""
		}
		return "http://" + rest
	case "xmlrpc_bin", "binrpc":
		return wellKnown[name]
	}
	return ""
}

func (s *Source) loadInterfaces(ctx context.Context) []iface {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ifaces != nil {
		return s.ifaces
	}
	urls := map[string]string{}
	if b, err := os.ReadFile(s.o.InterfacesList); err == nil {
		if m, err := ParseInterfacesList(b); err == nil {
			for n, u := range m {
				if h := httpURL(n, u); h != "" {
					urls[n] = h
				}
			}
		} else {
			s.logf("ccusource: %s: %v", s.o.InterfacesList, err)
		}
	}
	for n, u := range wellKnown {
		if _, ok := urls[n]; !ok {
			urls[n] = u
		}
	}
	names := make([]string, 0, len(urls))
	for n := range urls {
		names = append(names, n)
	}
	sort.Strings(names)
	// every process is probed at once, two seconds each: a CCU without one radio does not
	// hold the start up
	running := make([]bool, len(names))
	clients := make([]*Client, len(names))
	var wg sync.WaitGroup
	for idx, n := range names {
		clients[idx] = &Client{URL: urls[n], Client: s.o.Client}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if _, err := clients[idx].Call(cctx, "system.listMethods"); err == nil {
				running[idx] = true
			}
		}(idx)
	}
	wg.Wait()
	var out []iface
	for idx, n := range names {
		if !running[idx] {
			if _, listed := wellKnown[n]; listed {
				continue // a radio process that is not there: not an interface of this CCU
			}
		}
		out = append(out, iface{Interface: litesource.Interface{Name: n, Protocol: "xmlrpc", URLPath: urls[n], Running: running[idx]}, url: urls[n], client: clients[idx]})
	}
	s.ifaces = out
	return out
}

// Interfaces lists the interface processes that answer.
func (s *Source) Interfaces(ctx context.Context) ([]litesource.Interface, error) {
	var out []litesource.Interface
	for _, i := range s.loadInterfaces(ctx) {
		out = append(out, i.Interface)
	}
	return out, nil
}

func (s *Source) iface(ctx context.Context, name string) (*iface, error) {
	for _, i := range s.loadInterfaces(ctx) {
		if i.Name == name {
			ii := i
			return &ii, nil
		}
	}
	return nil, fmt.Errorf("ccusource: unknown interface %q", name)
}

// --- devices ---------------------------------------------------------------------------

// toJSON re-encodes a decoded XML-RPC value as JSON (the Raw of a description, a RawValue).
func toJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

// Devices lists an interface's devices and channels (listDevices).
func (s *Source) Devices(ctx context.Context, name string) ([]litesource.DeviceDescription, error) {
	i, err := s.iface(ctx, name)
	if err != nil {
		return nil, err
	}
	v, err := i.client.Call(ctx, "listDevices")
	if err != nil {
		return nil, err
	}
	list, _ := v.([]any)
	out := make([]litesource.DeviceDescription, 0, len(list))
	for _, e := range list {
		var d litesource.DeviceDescription
		if err := json.Unmarshal(toJSON(e), &d); err != nil {
			continue
		}
		d.Interface = name
		out = append(out, d)
	}
	s.mu.Lock()
	s.descs[name] = out
	s.mu.Unlock()
	return out, nil
}

// Descriptions answers the VALUES descriptions of the addresses, one getParamsetDescription per
// distinct model and channel (keyed TYPE/VERSION/channel type) - the description of a channel
// depends on the device's type and firmware version, not on the address.
func (s *Source) Descriptions(ctx context.Context, name string, addresses []string) (map[string]string, litesource.ParamsetCache, error) {
	i, err := s.iface(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	descs := s.descs[name]
	s.mu.Unlock()
	if descs == nil {
		if descs, err = s.Devices(ctx, name); err != nil {
			return nil, nil, err
		}
	}
	byAddr := map[string]litesource.DeviceDescription{}
	for _, d := range descs {
		byAddr[d.Address] = d
	}
	keys := map[string]string{}
	sets := litesource.ParamsetCache{}
	for _, a := range addresses {
		d, ok := byAddr[a]
		if !ok {
			continue
		}
		parent := byAddr[d.Parent]
		key := parent.Type + "/" + strconv.Itoa(parent.Version) + "/" + d.Type
		if d.Parent == "" {
			key = d.Type + "/" + strconv.Itoa(d.Version) + "/"
		}
		keys[a] = key
		if _, done := sets[key]; done {
			continue
		}
		v, err := i.client.Call(ctx, "getParamsetDescription", a, "VALUES")
		if err != nil {
			s.logf("ccusource: %s %s: getParamsetDescription: %v", name, a, err)
			sets[key] = litesource.Paramset{}
			continue
		}
		var ps litesource.Paramset
		if err := json.Unmarshal(toJSON(v), &ps); err != nil {
			sets[key] = litesource.Paramset{}
			continue
		}
		sets[key] = ps
	}
	return keys, sets, nil
}

// Read reads a paramset of a channel (getParamset).
func (s *Source) Read(ctx context.Context, name, address, paramset string) (map[string]litesource.RawValue, error) {
	i, err := s.iface(ctx, name)
	if err != nil {
		return nil, err
	}
	v, err := i.client.Call(ctx, "getParamset", address, paramset)
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	out := make(map[string]litesource.RawValue, len(m))
	for k, e := range m {
		out[k] = toJSON(e)
	}
	return out, nil
}

// Seed: a classic CCU keeps no state store, so the seed is empty and the subscription starts
// from now; the addon reads what it needs (the HmIP channel 0 from the process's cache).
func (s *Source) Seed(ctx context.Context, f litesource.Filter) (*litesource.Snapshot, error) {
	return &litesource.Snapshot{Entries: []litesource.StateEntry{}, EventID: ""}, nil
}

// --- the subscription: the callback server, init, the sweep --------------------------------

// Subscribe starts the callback server on the loopback, registers with every interface process
// (init) and delivers the events; a resync message follows a re-registration that found a
// process restarted. A filter that names interfaces registers with those processes only - an
// interface the addon does not want is never asked for its events (VirtualDevices answers an
// init only after it pushed its whole device list) - and a call naming another one is dropped.
func (s *Source) Subscribe(ctx context.Context, f litesource.Filter, since string) <-chan litesource.Message {
	ch := make(chan litesource.Message, 256)
	want := map[string]bool{}
	for _, k := range f.Keys {
		want[k] = true
	}
	if len(f.Interfaces) > 0 {
		s.mu.Lock()
		s.only = map[string]bool{}
		for _, i := range f.Interfaces {
			s.only[i] = true
		}
		s.mu.Unlock()
	}
	go func() {
		defer close(ch)
		ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(s.o.CallbackPort))
		if err != nil {
			s.logf("ccusource: callback port: %v", err)
			return
		}
		cbURL := "http://127.0.0.1:" + strconv.Itoa(s.o.CallbackPort)
		srv := &http.Server{Handler: s.callbackHandler(ch, want), ReadHeaderTimeout: 10 * time.Second}
		go srv.Serve(ln)
		defer func() {
			sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			s.unregister(sctx, cbURL)
			cancel()
			srv.Close()
		}()
		s.register(ctx, cbURL)
		ch <- litesource.Message{Type: "hello"}
		reinit := time.NewTicker(s.o.ReinitEvery)
		defer reinit.Stop()
		var sweep <-chan time.Time
		if s.o.SweepEvery > 0 {
			t := time.NewTicker(s.o.SweepEvery)
			defer t.Stop()
			sweep = t.C
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-reinit.C:
				s.register(ctx, cbURL)
			case <-sweep:
				s.sweep(ctx, ch, want)
			}
		}
	}()
	return ch
}

// register tells every running interface process the callback, all at once - an init takes
// its time (VirtualDevices answers after about ten seconds, it pushes its devices first) and
// one must not hold the others up; a process that was down is probed again first (it may have
// come up since), so a CCU whose radio process starts late still delivers once it is there.
func (s *Source) register(ctx context.Context, cbURL string) {
	ifaces := s.loadInterfaces(ctx)
	var wg sync.WaitGroup
	for idx := range ifaces {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			i := ifaces[idx]
			if !s.wanted(i.Name) {
				return
			}
			if !i.Running {
				cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				_, err := i.client.Call(cctx, "system.listMethods")
				cancel()
				if err != nil {
					return
				}
				s.mu.Lock()
				if idx < len(s.ifaces) {
					s.ifaces[idx].Running = true
				}
				s.mu.Unlock()
			}
			cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			start := time.Now()
			_, err := i.client.Call(cctx, "init", cbURL, s.o.ID+"-"+i.Name)
			cancel()
			if err != nil {
				s.logf("ccusource: %s: init: %v", i.Name, err)
			} else {
				s.logf("ccusource: %s: registered for events (%.1f s)", i.Name, time.Since(start).Seconds())
			}
		}(idx)
	}
	wg.Wait()
}

// wanted says whether the subscription takes the interface's events.
func (s *Source) wanted(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.only == nil || s.only[name]
}

func (s *Source) unregister(ctx context.Context, cbURL string) {
	s.mu.Lock()
	ifaces := s.ifaces
	s.mu.Unlock()
	for _, i := range ifaces {
		if i.Running && s.wanted(i.Name) {
			_, _ = i.client.Call(ctx, "init", cbURL, "")
		}
	}
}

// sweep reads the HmIP devices' channel 0 from hmipserver's cache (no radio traffic) and
// delivers the values as state messages - a device that reports rarely still has a current
// voltage.
func (s *Source) sweep(ctx context.Context, ch chan<- litesource.Message, want map[string]bool) {
	if !s.wanted("HmIP-RF") {
		return
	}
	s.mu.Lock()
	descs, known := s.descs["HmIP-RF"]
	s.mu.Unlock()
	if !known {
		// after a device change the list is read again (the consumer does the same)
		if _, err := s.iface(ctx, "HmIP-RF"); err != nil {
			return
		}
		descs, _ = s.Devices(ctx, "HmIP-RF")
	}
	for _, d := range descs {
		if d.Parent != "" || d.Address == "" || ctx.Err() != nil {
			continue
		}
		vals, err := s.Read(ctx, "HmIP-RF", d.Address+":0", "VALUES")
		if err != nil {
			continue
		}
		now := time.Now()
		for k, v := range vals {
			if len(want) > 0 && !want[k] {
				continue
			}
			ch <- litesource.Message{Type: "state", Event: &litesource.Event{Interface: "HmIP-RF", Address: d.Address + ":0", Key: k, Value: v, TS: now, Confirmed: true, Source: "sweep"}}
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// callbackHandler answers the daemons' calls: event (one value), system.multicall (many),
// listDevices (an empty list: the addon keeps no device list for the daemon to diff),
// newDevices / deleteDevices (device changes), system.listMethods, ping.
func (s *Source) callbackHandler(ch chan<- litesource.Message, want map[string]bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
		if err != nil {
			http.Error(w, "read", 400)
			return
		}
		method, params, err := UnmarshalCall(body)
		if err != nil {
			w.Header().Set("Content-Type", "text/xml")
			w.Write(MarshalFault(-32700, err.Error()))
			return
		}
		res := s.dispatch(method, params, ch, want)
		w.Header().Set("Content-Type", "text/xml")
		out, err := MarshalResponse(res)
		if err != nil {
			w.Write(MarshalFault(-32603, err.Error()))
			return
		}
		w.Write(out)
	})
}

// ifaceOf reads the interface name out of the registration id ("<addon>-<interface>").
func (s *Source) ifaceOf(id string) string {
	return strings.TrimPrefix(id, s.o.ID+"-")
}

func (s *Source) dispatch(method string, params []any, ch chan<- litesource.Message, want map[string]bool) any {
	switch method {
	case "system.multicall":
		list, _ := first(params).([]any)
		out := make([]any, 0, len(list))
		for _, c := range list {
			m, _ := c.(map[string]any)
			name, _ := m["methodName"].(string)
			p, _ := m["params"].([]any)
			out = append(out, []any{s.dispatch(name, p, ch, want)})
		}
		return out
	case "event":
		if len(params) >= 4 {
			id, _ := params[0].(string)
			addr, _ := params[1].(string)
			key, _ := params[2].(string)
			if (len(want) == 0 || want[key]) && s.wanted(s.ifaceOf(id)) {
				select {
				case ch <- litesource.Message{Type: "event", Event: &litesource.Event{Interface: s.ifaceOf(id), Address: addr, Key: key, Value: toJSON(params[3]), TS: time.Now(), Confirmed: true, Source: "event"}}:
				default:
					s.logf("ccusource: event dropped (the consumer is behind)")
				}
			}
		}
		return ""
	case "listDevices":
		s.logf("ccusource: %s asks for our device list (none kept)", s.ifaceOf(stringAt(params, 0)))
		return []any{}
	case "newDevices", "deleteDevices", "updateDevice", "replaceDevice", "readdedDevice":
		id, _ := first(params).(string)
		name := s.ifaceOf(id)
		if !s.wanted(name) {
			return ""
		}
		s.logf("ccusource: %s: %s", name, method)
		s.mu.Lock()
		delete(s.descs, name)
		s.mu.Unlock()
		msg := litesource.Message{Type: method, Devices: &litesource.DeviceChange{Interface: name, TS: time.Now()}}
		if method == "deleteDevices" && len(params) >= 2 {
			if list, ok := params[1].([]any); ok {
				for _, a := range list {
					if sa, ok := a.(string); ok {
						msg.Devices.Addresses = append(msg.Devices.Addresses, sa)
					}
				}
			}
		}
		select {
		case ch <- msg:
		default:
		}
		return ""
	case "system.listMethods":
		return []any{"system.multicall", "system.listMethods", "event", "listDevices", "newDevices", "deleteDevices", "updateDevice", "replaceDevice", "readdedDevice", "ping"}
	case "ping":
		return ""
	}
	return ""
}

func stringAt(p []any, i int) string {
	if i < len(p) {
		s, _ := p[i].(string)
		return s
	}
	return ""
}

func first(p []any) any {
	if len(p) == 0 {
		return nil
	}
	return p[0]
}

// ErrNotCCU says the system has no interface list and no process answers.
var ErrNotCCU = errors.New("ccusource: no interface process answers")

// Probe says whether this looks like a CCU with at least one interface process.
func (s *Source) Probe(ctx context.Context) error {
	if len(s.loadInterfaces(ctx)) == 0 {
		return ErrNotCCU
	}
	return nil
}
