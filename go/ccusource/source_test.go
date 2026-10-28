package ccusource

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"
)

// a stand-in hmipserver: listDevices, getParamsetDescription, getParamset, init (it then
// sends one event and one multicall to the callback)
type daemon struct {
	t       *testing.T
	mu      sync.Mutex
	inits   []string
	client  *http.Client
	calls   []string
	cbURL   string
	cbID    string
	srv     *httptest.Server
	devices []any
	// last are the parameters of the last putParamset
	last []any
}

func newDaemon(t *testing.T) *daemon {
	d := &daemon{t: t, client: &http.Client{Timeout: 5 * time.Second}}
	d.devices = []any{
		map[string]any{"ADDRESS": "DEVA", "TYPE": "HmIP-WRC2", "VERSION": 1, "FIRMWARE": "1.10.0", "CHILDREN": []any{"DEVA:0", "DEVA:1"}, "PARAMSETS": []any{"MASTER"}},
		map[string]any{"ADDRESS": "DEVA:0", "TYPE": "MAINTENANCE", "PARENT": "DEVA", "VERSION": 1, "PARAMSETS": []any{"MASTER", "VALUES"}},
		map[string]any{"ADDRESS": "DEVA:1", "TYPE": "KEY_TRANSCEIVER", "PARENT": "DEVA", "VERSION": 1, "PARAMSETS": []any{"MASTER", "VALUES"}},
		map[string]any{"ADDRESS": "DEVB", "TYPE": "HmIP-WRC2", "VERSION": 1, "CHILDREN": "", "PARAMSETS": "MASTER"},
		map[string]any{"ADDRESS": "DEVB:0", "TYPE": "MAINTENANCE", "PARENT": "DEVB", "VERSION": 1},
	}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		method, params, err := UnmarshalCall(body)
		if err != nil {
			w.Write(MarshalFault(-1, err.Error()))
			return
		}
		d.mu.Lock()
		d.calls = append(d.calls, method)
		d.mu.Unlock()
		var res any
		switch method {
		case "system.listMethods":
			res = []any{"init", "listDevices"}
		case "listDevices":
			res = d.devices
		case "getParamsetDescription":
			if params[0] == "DEVA:0" || params[0] == "DEVB:0" {
				res = map[string]any{"OPERATING_VOLTAGE": map[string]any{"TYPE": "FLOAT", "OPERATIONS": 5}, "LOW_BAT": map[string]any{"TYPE": "BOOL", "OPERATIONS": 5}}
			} else {
				res = map[string]any{"PRESS_SHORT": map[string]any{"TYPE": "ACTION"}}
			}
		case "getParamset":
			res = map[string]any{"OPERATING_VOLTAGE": 2.95, "LOW_BAT": false, "UNREACH": false}
		case "putParamset":
			d.mu.Lock()
			d.last = params
			d.mu.Unlock()
			res = ""
		case "init":
			url, _ := params[0].(string)
			id, _ := params[1].(string)
			d.mu.Lock()
			d.inits = append(d.inits, id)
			d.cbURL, d.cbID = url, id
			d.mu.Unlock()
			res = ""
			if id != "" {
				go d.sendEvents(url, id)
			}
		default:
			w.Write(MarshalFault(-32601, "unknown method "+method))
			return
		}
		out, _ := MarshalResponse(res)
		w.Header().Set("Content-Type", "text/xml")
		w.Write(out)
	}))
	t.Cleanup(d.srv.Close)
	return d
}

func (d *daemon) post(url string, body []byte) {
	res, err := d.client.Post(url, "text/xml", strings.NewReader(string(body)))
	if err != nil {
		d.t.Logf("callback: %v", err)
		return
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
}

func (d *daemon) sendEvents(url, id string) {
	time.Sleep(50 * time.Millisecond)
	ev, _ := MarshalCall("event", id, "DEVA:0", "OPERATING_VOLTAGE", 2.9)
	d.post(url, ev)
	mc, _ := MarshalCall("system.multicall", []any{
		map[string]any{"methodName": "event", "params": []any{id, "DEVA:0", "LOW_BAT", true}},
		map[string]any{"methodName": "event", "params": []any{id, "DEVA:1", "PRESS_SHORT", true}},
	})
	d.post(url, mc)
	del, _ := MarshalCall("deleteDevices", id, []any{"DEVB", "DEVB:0"})
	d.post(url, del)
	ping, _ := MarshalCall("ping", id)
	d.post(url, ping)
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestSource(t *testing.T) {
	d := newDaemon(t)
	rega := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), "root.Devices()") {
			t.Errorf("unexpected script")
		}
		io.WriteString(w, "D\tHmIP-RF\tDEVA\tKüche Taster\nC\tDEVA:0\tKüche Taster:0\t1234|\nC\tDEVA:1\tKüche Taster:1\t1234|5678|\nD\t\tORPHAN\tx\nR\t1234\tKüche\nR\t5678\tFlur\n<xml><exec>/rega.exe</exec></xml>")
	}))
	defer rega.Close()
	dir := t.TempDir()
	list := filepath.Join(dir, "InterfacesList.xml")
	os.WriteFile(list, []byte(`<?xml version="1.0" encoding="utf-8" ?><interfaces v="1.0">
<ipc><name>VirtualDevices</name><url>xmlrpc://127.0.0.1:1/groups</url><info>Virtual Devices</info></ipc>
<ipc><name>HmIP-RF</name><url>`+strings.Replace(d.srv.URL, "http://", "xmlrpc://", 1)+`</url></ipc>
<ipc><name>BidCos-RF</name><url>xmlrpc_bin://127.0.0.1:1</url></ipc>
</interfaces>`), 0o644)
	port := freePort(t)
	s := New(Options{ID: "battery", CallbackPort: port, InterfacesList: list, Rega: rega.URL, SweepEvery: 80 * time.Millisecond, ReinitEvery: time.Hour, NamesEvery: 60 * time.Millisecond, Log: t.Logf})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// the interfaces: the one that answers; the binrpc one maps to its well-known port, which
	// nobody answers here, so it is left out; VirtualDevices is kept but not running
	ifs, err := s.Interfaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, i := range ifs {
		names = append(names, i.Name+":"+map[bool]string{true: "up", false: "down"}[i.Running])
	}
	if strings.Join(names, ",") != "HmIP-RF:up,VirtualDevices:down" {
		t.Fatalf("interfaces: %v", names)
	}
	if err := s.Probe(ctx); err != nil {
		t.Fatal(err)
	}

	devs, err := s.Devices(ctx, "HmIP-RF")
	if err != nil || len(devs) != 5 || devs[0].Address != "DEVA" || devs[0].Interface != "HmIP-RF" || len(devs[0].Children) != 2 || len(devs[3].Children) != 0 || devs[3].Paramsets[0] != "MASTER" {
		t.Fatalf("devices: %+v %v", devs, err)
	}
	keys, sets, err := s.Descriptions(ctx, "HmIP-RF", []string{"DEVA:0", "DEVA:1", "DEVB:0", "NOPE:9"})
	if err != nil {
		t.Fatal(err)
	}
	if keys["DEVA:0"] != "HmIP-WRC2/1/MAINTENANCE" || keys["DEVB:0"] != keys["DEVA:0"] || keys["DEVA:1"] != "HmIP-WRC2/1/KEY_TRANSCEIVER" || keys["NOPE:9"] != "" {
		t.Fatalf("keys: %v", keys)
	}
	if _, ok := sets[keys["DEVA:0"]]["OPERATING_VOLTAGE"]; !ok || len(sets) != 2 {
		t.Fatalf("sets: %v", sets)
	}
	d.mu.Lock()
	descCalls := 0
	for _, c := range d.calls {
		if c == "getParamsetDescription" {
			descCalls++
		}
	}
	d.mu.Unlock()
	if descCalls != 2 {
		t.Fatalf("one description per key, got %d calls", descCalls)
	}
	vals, err := s.Read(ctx, "HmIP-RF", "DEVA:0", "VALUES")
	if err != nil || string(vals["OPERATING_VOLTAGE"]) != "2.95" || string(vals["LOW_BAT"]) != "false" {
		t.Fatalf("read: %v %v", vals, err)
	}
	if snap, err := s.Seed(ctx, litesource.Filter{}); err != nil || len(snap.Entries) != 0 {
		t.Fatalf("seed: %+v %v", snap, err)
	}

	// names from ReGa
	n, err := s.Names(ctx)
	if err != nil || n.Name("HmIP-RF", "DEVA") != "Küche Taster" || n.Name("HmIP-RF", "DEVA:1") != "Küche Taster:1" || n.Name("HmIP-RF", "ORPHAN") != "" {
		t.Fatalf("names: %+v %v", n, err)
	}
	if r := n.RoomNames("HmIP-RF", "DEVA", "de"); strings.Join(r, ",") != "Küche,Flur" {
		t.Fatalf("rooms: %v", r)
	}
	if r := n.RoomNames("HmIP-RF", "DEVA:0", "en"); strings.Join(r, ",") != "Küche" {
		t.Fatalf("channel rooms: %v", r)
	}

	// the subscription: init registers, the daemon's events arrive, the sweep reads channel 0
	ch := s.Subscribe(ctx, litesource.Filter{Keys: []string{"OPERATING_VOLTAGE", "LOW_BAT", "UNREACH"}}, "")
	var got []string
	deadline := time.After(5 * time.Second)
	for len(got) < 7 {
		select {
		case m, ok := <-ch:
			if !ok {
				t.Fatal("closed")
			}
			switch m.Type {
			case "hello":
				got = append(got, "hello")
			case "event", "state":
				got = append(got, m.Type+":"+m.Event.Address+"/"+m.Event.Key+"="+string(m.Event.Value)+"/"+m.Event.Source)
			case "deleteDevices":
				got = append(got, "deleteDevices:"+strings.Join(m.Devices.Addresses, "+"))
			}
		case <-deadline:
			t.Fatalf("got only %v", got)
		}
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"hello", "event:DEVA:0/OPERATING_VOLTAGE=2.9/event", "event:DEVA:0/LOW_BAT=true/event", "deleteDevices:DEVB+DEVB:0", "state:DEVA:0/OPERATING_VOLTAGE=2.95/sweep"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "PRESS_SHORT") {
		t.Error("an unwanted key came through")
	}
	d.mu.Lock()
	inits := strings.Join(d.inits, ",")
	d.mu.Unlock()
	if inits != "battery-HmIP-RF" {
		t.Fatalf("inits: %s", inits)
	}
	// the names poll: unchanged names give no event
	ev := s.NameEvents(ctx, 0)
	select {
	case e := <-ev:
		t.Fatalf("unexpected name event %+v", e)
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	for range ch {
	}
	time.Sleep(100 * time.Millisecond)
	d.mu.Lock()
	last := d.inits[len(d.inits)-1]
	d.mu.Unlock()
	if last != "" {
		t.Fatalf("unregistered at the end: %v", d.inits)
	}
}

func TestInterfacesListAndURLs(t *testing.T) {
	m, err := ParseInterfacesList([]byte(`<interfaces v="1.0"><ipc><name>BidCos-RF</name><url>xmlrpc_bin://127.0.0.1:32001</url></ipc><ipc><name>HmIP-RF</name><url>xmlrpc://127.0.0.1:32010</url></ipc><ipc><name>CUxD</name><url>xmlrpc_bin://127.0.0.1:8701</url></ipc></interfaces>`))
	if err != nil || len(m) != 3 {
		t.Fatal(m, err)
	}
	if httpURL("BidCos-RF", m["BidCos-RF"]) != "http://127.0.0.1:2001" || httpURL("HmIP-RF", m["HmIP-RF"]) != "http://127.0.0.1:32010" || httpURL("CUxD", m["CUxD"]) != "" {
		t.Fatal("urls")
	}
	snap := ParseNames("junk\nD\tBidCos-RF\tJEQ01\tLampe\nC\tJEQ01:1\tLampe:1\t\n")
	if snap.Objects["BidCos-RF.JEQ01"].Name != "Lampe" || len(snap.Objects["BidCos-RF.JEQ01"].Enums) != 0 {
		t.Fatalf("%+v", snap.Objects)
	}
}

// A filter naming interfaces registers with those processes only: VirtualDevices is running
// but never sees an init, and a call that names it anyway is dropped.
func TestSubscribeOnlyTheFilteredInterfaces(t *testing.T) {
	ip := newDaemon(t)
	virt := newDaemon(t)
	dir := t.TempDir()
	list := filepath.Join(dir, "InterfacesList.xml")
	os.WriteFile(list, []byte(`<?xml version="1.0" encoding="utf-8" ?><interfaces v="1.0">
<ipc><name>VirtualDevices</name><url>`+strings.Replace(virt.srv.URL, "http://", "xmlrpc://", 1)+`</url></ipc>
<ipc><name>HmIP-RF</name><url>`+strings.Replace(ip.srv.URL, "http://", "xmlrpc://", 1)+`</url></ipc>
<ipc><name>BidCos-RF</name><url>xmlrpc_bin://127.0.0.1:1</url></ipc>
</interfaces>`), 0o644)
	port := freePort(t)
	s := New(Options{ID: "battery", CallbackPort: port, InterfacesList: list, SweepEvery: -1, ReinitEvery: time.Hour, NamesEvery: time.Hour, Log: t.Logf})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ch := s.Subscribe(ctx, litesource.Filter{Interfaces: []string{"HmIP-RF", "BidCos-RF"}, Keys: []string{"OPERATING_VOLTAGE", "LOW_BAT"}}, "")
	var got []string
	deadline := time.After(5 * time.Second)
	for len(got) < 3 {
		select {
		case m, ok := <-ch:
			if !ok {
				t.Fatal("closed")
			}
			switch m.Type {
			case "hello":
				got = append(got, "hello")
				// a call naming the unwanted interface (as a stale registration would send)
				ev, _ := MarshalCall("system.multicall", []any{
					map[string]any{"methodName": "event", "params": []any{"battery-VirtualDevices", "INT0001:0", "LOW_BAT", true}},
				})
				virt.post("http://127.0.0.1:"+strconv.Itoa(port), ev)
				nd, _ := MarshalCall("newDevices", "battery-VirtualDevices", []any{})
				virt.post("http://127.0.0.1:"+strconv.Itoa(port), nd)
			case "event":
				got = append(got, m.Event.Interface+":"+m.Event.Address+"/"+m.Event.Key)
			default:
				if m.Devices != nil && m.Devices.Interface == "VirtualDevices" {
					t.Fatalf("a device change of an unwanted interface came through: %+v", m)
				}
			}
		case <-deadline:
			t.Fatalf("got only %v", got)
		}
	}
	select {
	case m := <-ch:
		if m.Event != nil && m.Event.Interface == "VirtualDevices" || m.Devices != nil && m.Devices.Interface == "VirtualDevices" {
			t.Fatalf("an unwanted interface came through: %+v", m)
		}
	case <-time.After(200 * time.Millisecond):
	}
	for _, g := range got {
		if strings.HasPrefix(g, "VirtualDevices") {
			t.Fatalf("an event of VirtualDevices came through: %v", got)
		}
	}
	cancel()
	for range ch {
	}
	virt.mu.Lock()
	vinits := len(virt.inits)
	virt.mu.Unlock()
	ip.mu.Lock()
	iinits := strings.Join(ip.inits, ",")
	ip.mu.Unlock()
	if vinits != 0 {
		t.Fatalf("VirtualDevices got %d inits", vinits)
	}
	if !strings.HasPrefix(iinits, "battery-HmIP-RF") {
		t.Fatalf("HmIP-RF inits: %s", iinits)
	}
}
