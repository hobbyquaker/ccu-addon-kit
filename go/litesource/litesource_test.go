package litesource_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"
	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"
	"github.com/hobbyquaker/ccu-addon-kit/go/litesource/fakelite"
)

func newClient(t *testing.T, s *fakelite.Server) *litesource.Client {
	t.Helper()
	s.Token = "olt_test"
	c := litesource.New(s.URL, addonkit.StaticToken("olt_test"))
	c.Reconnect.Min, c.Reconnect.Max = 10*time.Millisecond, 50*time.Millisecond
	c.Log = t.Logf
	return c
}

func next(t *testing.T, ch <-chan litesource.Message, typ string) litesource.Message {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case m, ok := <-ch:
			if !ok {
				t.Fatalf("stream closed while waiting for %s", typ)
			}
			if m.Type == typ {
				return m
			}
		case <-deadline:
			t.Fatalf("no %s message within 3 s", typ)
		}
	}
}

func TestProbeAndInterfaces(t *testing.T) {
	s := fakelite.New()
	defer s.Close()
	c := newClient(t, s)
	v, err := c.Probe(context.Background())
	if err != nil || v.API != "meta" {
		t.Fatalf("probe: %v %+v", err, v)
	}
	ifs, err := c.Interfaces(context.Background())
	if err != nil || len(ifs) != 1 || ifs[0].Name != "HmIP-RF" {
		t.Fatalf("interfaces: %v %+v", err, ifs)
	}
	bad := litesource.New(s.URL, addonkit.StaticToken("wrong"))
	if _, err := bad.Interfaces(context.Background()); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("a wrong token is 401: %v", err)
	}
}

func TestStateAndEvents(t *testing.T) {
	s := fakelite.New()
	defer s.Close()
	c := newClient(t, s)
	for i := 0; i < 2500; i++ {
		s.State = append(s.State, map[string]any{"interface": "HmIP-RF", "address": "DEV" + string(rune('A'+i%26)) + ":0", "datapoint": "LOW_BAT",
			"value": false, "ts": "2026-10-27T10:00:00Z", "lc": "2026-10-10T10:00:00Z", "confirmed": i%2 == 0, "source": "restored"})
	}
	s.State = append(s.State, map[string]any{"interface": "HmIP-RF", "address": "DEVA:0", "datapoint": "UNREACH", "value": true, "ts": "2026-10-27T10:00:00Z", "confirmed": true, "source": "event"})
	s.EventID = "boot1-0"
	snap, err := c.State(context.Background(), litesource.Filter{Keys: []string{"LOW_BAT"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Entries) != 2500 || snap.EventID != "boot1-0" || snap.Entries[0].Key != "LOW_BAT" || snap.Entries[1].Confirmed {
		t.Fatalf("state pages: %d entries, event id %q, first %+v", len(snap.Entries), snap.EventID, snap.Entries[0])
	}
	if v, ok := snap.Entries[0].Bool(); !ok || v {
		t.Fatal("bool value")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := c.Events(ctx, litesource.Filter{Keys: []string{"OPERATING_VOLTAGE", "LOW_BAT"}}, snap.EventID)
	h := next(t, ch, "hello")
	if h.Hello.BootID != "boot1" {
		t.Fatalf("hello: %+v", h.Hello)
	}
	s.PushEvent("HmIP-RF", "DEVA:0", "OPERATING_VOLTAGE", 2.9)
	s.PushEvent("HmIP-RF", "DEVA:0", "RSSI_DEVICE", -60) // filtered out
	s.PushEvent("HmIP-RF", "DEVA:0", "LOW_BAT", true)
	e := next(t, ch, "event")
	if f, ok := e.Event.Float(); !ok || f != 2.9 || e.Event.Device() != "DEVA" || e.Event.Channel() != 0 || e.Event.Source != "event" || !e.Event.Confirmed {
		t.Fatalf("event: %+v", e.Event)
	}
	e = next(t, ch, "event")
	if e.Event.Key != "LOW_BAT" {
		t.Fatalf("the filtered key must not arrive: %+v", e.Event)
	}
	lastID := e.ID

	// a dropped connection: the client resumes with the last id and nothing is lost, no resync
	s.Drop()
	s.PushEvent("HmIP-RF", "DEVB:0", "OPERATING_VOLTAGE", 3.0)
	h = next(t, ch, "hello")
	e = next(t, ch, "event")
	if e.Event.Address != "DEVB:0" || e.ID <= lastID {
		t.Fatalf("resume: %+v", e)
	}

	// a reboot: another boot id is a resync
	s.Reboot("boot2")
	m := next(t, ch, "resync")
	if m.Resync.Reason != "boot" {
		t.Fatalf("reboot: %+v", m.Resync)
	}
	// the server's own resync passes through
	next(t, ch, "hello")
	s.Resync("overflow")
	if m := next(t, ch, "resync"); m.Resync.Reason != "overflow" {
		t.Fatalf("overflow: %+v", m.Resync)
	}
	cancel()
	for range ch {
	}
}

func TestSilenceWatchdog(t *testing.T) {
	s := fakelite.New()
	defer s.Close()
	c := newClient(t, s)
	c.Silence = 150 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := c.Events(ctx, litesource.Filter{}, "")
	next(t, ch, "hello")
	// nothing is sent and no ping: the client must reconnect, and say so with a reconnect resync
	// only when it had no id (it has the hello's)
	next(t, ch, "hello")
	cancel()
}

func TestRPCAndParamsets(t *testing.T) {
	s := fakelite.New()
	defer s.Close()
	c := newClient(t, s)
	s.Devices["HmIP-RF"] = []map[string]any{
		{"ADDRESS": "DEVA", "TYPE": "HmIP-WRC2", "FIRMWARE": "1.2.3", "VERSION": 1, "CHILDREN": []string{"DEVA:0", "DEVA:1"}},
		{"ADDRESS": "DEVA:0", "TYPE": "MAINTENANCE", "PARENT": "DEVA", "PARAMSETS": []string{"MASTER", "VALUES"}},
	}
	s.Paramsets["HmIP-RF/HmIP-WRC2/1.2.3/1/MAINTENANCE/VALUES"] = map[string]any{
		"OPERATING_VOLTAGE": map[string]any{"TYPE": "FLOAT", "OPERATIONS": 5, "MIN": 0.0, "MAX": 25.2, "UNIT": "V"},
		"LOW_BAT":           map[string]any{"TYPE": "BOOL", "OPERATIONS": 5, "FLAGS": 9},
	}
	s.Values["HmIP-RF/DEVA:0/VALUES"] = map[string]any{"OPERATING_VOLTAGE": 2.9, "LOW_BAT": false}
	ctx := context.Background()
	devs, err := c.ListDevices(ctx, "HmIP-RF")
	if err != nil || len(devs) != 2 || devs[0].Type != "HmIP-WRC2" || devs[1].Parent != "DEVA" || devs[1].Interface != "HmIP-RF" {
		t.Fatalf("listDevices: %v %+v", err, devs)
	}
	vals, err := c.GetParamset(ctx, "HmIP-RF", "DEVA:0", "VALUES")
	if err != nil || string(vals["OPERATING_VOLTAGE"]) != "2.9" {
		t.Fatalf("getParamset: %v %v", err, vals)
	}
	if _, err := c.GetParamset(ctx, "HmIP-RF", "NOPE:0", "VALUES"); err == nil || !strings.Contains(err.Error(), "rpc -1") {
		t.Fatalf("a fault keeps its code: %v", err)
	}
	if _, err := c.Call(ctx, "BidCos-RF", "listDevices"); err == nil {
		t.Fatal("unknown interface")
	}
	all, err := c.Paramsets(ctx)
	if err != nil || all["HmIP-RF/HmIP-WRC2/1.2.3/1/MAINTENANCE/VALUES"]["OPERATING_VOLTAGE"].Type != "FLOAT" {
		t.Fatalf("paramsets: %v %+v", err, all)
	}
	keys, sets, errs, err := c.ParamsetsFor(ctx, "HmIP-RF", []string{"DEVA:0", "X:1"}, "VALUES")
	if err != nil || keys["DEVA:0"] == "" || sets[keys["DEVA:0"]]["LOW_BAT"].Flags != 9 || errs["X:1"] == "" {
		t.Fatalf("paramsetsFor: %v %v %v %v", err, keys, sets, errs)
	}
	if len(s.Calls) != 4 {
		t.Fatalf("calls: %v", s.Calls)
	}
}

func TestMeta(t *testing.T) {
	s := fakelite.New()
	defer s.Close()
	c := newClient(t, s)
	s.Meta["enums"] = map[string]any{
		"room": map[string]any{"name": map[string]string{"en": "Rooms"}, "tree": []map[string]any{
			{"id": "eg", "name": map[string]string{"de": "Erdgeschoss", "en": "Ground floor"}, "children": []map[string]any{
				{"id": "kueche", "name": map[string]string{"de": "Küche", "en": "Kitchen"}}}}}},
		// the favorite enum's nodes carry a plain string as their name (a real system's snapshot)
		"favorite": map[string]any{"name": "Favoriten", "tree": []map[string]any{{"id": "start", "name": "Startseite"}}},
	}
	s.SetName("HmIP-RF.DEVA", "Fenster Küche", "room/eg/kueche", "favorite/start")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	names, err := c.Names(ctx)
	if err != nil || names.Name("HmIP-RF", "DEVA") != "Fenster Küche" {
		t.Fatalf("names: %v %+v", err, names)
	}
	if r := names.RoomNames("HmIP-RF", "DEVA", "de"); len(r) != 1 || r[0] != "Küche" {
		t.Fatalf("rooms: %v", r)
	}
	if r := names.RoomNames("HmIP-RF", "DEVA", "en"); r[0] != "Kitchen" {
		t.Fatalf("rooms en: %v", r)
	}
	snap, _ := c.Meta().Snapshot(ctx)
	if n := snap.NodeName("favorite/start", "de"); n != "Startseite" {
		t.Fatalf("a string-named node: %q", n)
	}
	ch := c.NameEvents(ctx, names.Revision)
	s.SetName("HmIP-RF.DEVA", "Küche Fenster")
	select {
	case ev := <-ch:
		if ev.Kind != "object.updated" || ev.Ref != "HmIP-RF.DEVA" || ev.Revision != names.Revision+1 {
			t.Fatalf("event: %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no meta event")
	}
}

// The VirtualDevices process answers CHILDREN as a string ("" without children) where rfd and
// hmipserver send an array; seen on a production system 2026-10-27, where the whole interface
// was skipped for it.
func TestDeviceDescriptionTolerantLists(t *testing.T) {
	cases := []struct {
		in       string
		children []string
		sets     []string
		version  int
	}{
		{`{"ADDRESS":"CUX2801001","TYPE":"HM-LC-Sw1-Pl","CHILDREN":"","PARAMSETS":"MASTER","VERSION":"7"}`, []string{}, []string{"MASTER"}, 7},
		{`{"ADDRESS":"CUX2801001:1","TYPE":"SWITCH","PARENT":"CUX2801001","CHILDREN":"CUX2801001:2","PARAMSETS":["MASTER","VALUES"],"VERSION":1.0}`, []string{"CUX2801001:2"}, []string{"MASTER", "VALUES"}, 1},
		{`{"ADDRESS":"DEVA","TYPE":"HmIP-WRC2","CHILDREN":["DEVA:0","DEVA:1"],"PARAMSETS":null}`, []string{"DEVA:0", "DEVA:1"}, nil, 0},
	}
	for _, c := range cases {
		var d litesource.DeviceDescription
		if err := json.Unmarshal([]byte(c.in), &d); err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if strings.Join(d.Children, ",") != strings.Join(c.children, ",") || (d.Children == nil) != (c.children == nil) || strings.Join(d.Paramsets, ",") != strings.Join(c.sets, ",") || d.Version != c.version || string(d.Raw) != c.in {
			t.Errorf("%s: %+v", c.in, d)
		}
	}
	var d litesource.DeviceDescription
	if err := json.Unmarshal([]byte(`{"ADDRESS":"X","CHILDREN":7}`), &d); err == nil {
		t.Error("a number is not a list")
	}
}

// A filter naming interfaces goes to the stream as interface=, and (as occulited) nothing of
// another interface comes back.
func TestEventsInterfaceFilter(t *testing.T) {
	s := fakelite.New()
	defer s.Close()
	s.Interfaces = []string{"HmIP-RF", "VirtualDevices"}
	c := newClient(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := c.Events(ctx, litesource.Filter{Interfaces: []string{"HmIP-RF", "BidCos-RF"}, Keys: []string{"LOW_BAT"}}, "")
	next(t, ch, "hello")
	if q := s.Queries(); len(q) != 1 || !strings.Contains(q[0], "interface=HmIP-RF%2CBidCos-RF") {
		t.Fatalf("stream query: %v", q)
	}
	s.PushEvent("VirtualDevices", "INT0001:0", "LOW_BAT", true)
	s.PushEvent("HmIP-RF", "DEVA:0", "LOW_BAT", true)
	m := next(t, ch, "event")
	if m.Event.Interface != "HmIP-RF" {
		t.Fatalf("an event of another interface came through: %+v", m.Event)
	}
	cancel()
}
