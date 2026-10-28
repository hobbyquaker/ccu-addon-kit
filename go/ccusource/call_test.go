package ccusource

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"
)

func TestCall(t *testing.T) {
	d := newDaemon(t)
	list := filepath.Join(t.TempDir(), "InterfacesList.xml")
	os.WriteFile(list, []byte(`<?xml version="1.0" encoding="utf-8" ?><interfaces v="1.0">
<ipc><name>HmIP-RF</name><url>`+strings.Replace(d.srv.URL, "http://", "xmlrpc://", 1)+`</url></ipc>
<ipc><name>BidCos-RF</name><url>xmlrpc_bin://127.0.0.1:1</url></ipc>
</interfaces>`), 0o644)
	s := New(Options{ID: "t", InterfacesList: list, Log: t.Logf})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// the answer comes back as JSON, as litesource's Call answers it
	raw, err := s.Call(ctx, "HmIP-RF", "getParamset", "DEVA:0", "VALUES")
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil || values["OPERATING_VOLTAGE"] != 2.95 || values["LOW_BAT"] != false {
		t.Fatalf("getParamset: %s %v", raw, err)
	}

	// a Double at any depth goes as a double, an int stays an int, the rest as it is
	if _, err := s.Call(ctx, "HmIP-RF", "putParamset", "DEVA:1", "DEVA:1", map[string]any{
		"LEVEL": litesource.Double(1), "RAMP": []any{litesource.Double(0.5), 2}, "ON": true, "NAME": "x",
	}); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	ps, _ := d.last[2].(map[string]any)
	d.mu.Unlock()
	ramp, _ := ps["RAMP"].([]any)
	if ps["LEVEL"] != 1.0 || len(ramp) != 2 || ramp[0] != 0.5 || ramp[1] != 2 || ps["ON"] != true || ps["NAME"] != "x" {
		t.Fatalf("putParamset params: %#v", ps)
	}

	// a fault keeps its code; an interface the CCU does not have is an error before any call
	_, err = s.Call(ctx, "HmIP-RF", "nope")
	var f *Fault
	if !errors.As(err, &f) || f.Code != -32601 {
		t.Fatalf("fault: %v", err)
	}
	if _, err := s.Call(ctx, "BidCos-RF", "listDevices"); err == nil || !strings.Contains(err.Error(), "unknown interface") {
		t.Fatalf("unknown interface: %v", err)
	}
}
