package ccusource

import (
	"reflect"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	call, err := MarshalCall("init", "http://127.0.0.1:8292", "battery-HmIP-RF")
	if err != nil {
		t.Fatal(err)
	}
	m, p, err := UnmarshalCall(call)
	if err != nil || m != "init" || len(p) != 2 || p[0] != "http://127.0.0.1:8292" || p[1] != "battery-HmIP-RF" {
		t.Fatalf("%s %v %v", m, p, err)
	}
	v := map[string]any{"ADDRESS": "DEVA:0", "VERSION": 1, "CHILDREN": []any{"a", "b"}, "VOLTAGE": 2.95, "LOW_BAT": false, "NOTE": "ä<>&"}
	res, err := MarshalResponse(v)
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalResponse(res)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, v) {
		t.Fatalf("got %#v", got)
	}
	// a fault
	if _, err := UnmarshalResponse(MarshalFault(-1, "unknown method")); err == nil || !strings.Contains(err.Error(), "fault -1: unknown method") {
		t.Fatalf("fault: %v", err)
	}
	// what rfd sends: ISO-8859-1 declared, a bare value, <int>, <boolean>true
	raw := `<?xml version="1.0" encoding="ISO-8859-1"?><methodResponse><params><param><value><struct>` +
		`<member><name>LOWBAT</name><value><boolean>0</boolean></value></member>` +
		`<member><name>NAME</name><value>K` + "\xfc" + `che</value></member>` +
		`<member><name>RSSI_DEVICE</name><value><int>-67</int></value></member>` +
		`</struct></value></param></params></methodResponse>`
	g, err := UnmarshalResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	mm := g.(map[string]any)
	if mm["LOWBAT"] != false || mm["NAME"] != "Küche" || mm["RSSI_DEVICE"] != -67 {
		t.Fatalf("%#v", mm)
	}
	// an empty response
	if g, err := UnmarshalResponse([]byte(`<?xml version="1.0"?><methodResponse><params></params></methodResponse>`)); err != nil || g != nil {
		t.Fatalf("empty: %v %v", g, err)
	}
	// a multicall as the daemons send it
	mc := `<?xml version="1.0"?><methodCall><methodName>system.multicall</methodName><params><param><value><array><data>` +
		`<value><struct><member><name>methodName</name><value><string>event</string></value></member><member><name>params</name><value><array><data>` +
		`<value><string>battery-HmIP-RF</string></value><value><string>DEVA:0</string></value><value><string>OPERATING_VOLTAGE</string></value><value><double>2.9</double></value>` +
		`</data></array></value></member></struct></value></data></array></value></param></params></methodCall>`
	name, params, err := UnmarshalCall([]byte(mc))
	if err != nil || name != "system.multicall" || len(params) != 1 {
		t.Fatalf("%s %v %v", name, params, err)
	}
	calls := params[0].([]any)
	first := calls[0].(map[string]any)
	if first["methodName"] != "event" || first["params"].([]any)[3] != 2.9 {
		t.Fatalf("%#v", first)
	}
}
