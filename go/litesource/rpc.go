package litesource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
)

// RPCError is an interface process's fault (its code kept), or lite-rpc's own refusal: -32601
// for init and a refused tier, -32000 for a process that is down.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("rpc %d: %s", e.Code, e.Message) }

var rpcID atomic.Int64

// Call sends one JSON-RPC 2.0 call to an interface process through POST /api/rpc/v1/json/{iface}
// and answers the result. Every method needs the tier the system's table says (reads rpc:read).
// A FLOAT parameter is sent as Double so the process gets a double whatever it looks like.
func (c *Client) Call(ctx context.Context, iface, method string, params ...any) (json.RawMessage, error) {
	if params == nil {
		params = []any{}
	}
	req := map[string]any{"jsonrpc": "2.0", "method": method, "params": params, "id": rpcID.Add(1)}
	var res struct {
		Result json.RawMessage `json:"result"`
		Error  *RPCError       `json:"error"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/rpc/v1/json/"+url.PathEscape(iface), req, &res); err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return res.Result, nil
}

// Double marks a number as an XML-RPC double in a JSON-RPC call ({"double": n}).
type Double float64

// MarshalJSON writes {"double": n}.
func (d Double) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]float64{"double": float64(d)})
}

// ListDevices is listDevices of one interface: every device and channel description.
func (c *Client) ListDevices(ctx context.Context, iface string) ([]DeviceDescription, error) {
	raw, err := c.Call(ctx, iface, "listDevices")
	if err != nil {
		return nil, err
	}
	var out []DeviceDescription
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Interface == "" {
			out[i].Interface = iface
		}
	}
	return out, nil
}

// GetParamset reads one paramset (VALUES, MASTER) of a channel. What it costs on air: HmIP
// answers VALUES from hmipserver's cache; BidCos asks an actuator for its status on every VALUES
// read of a value channel and a battery device does not answer. Channel 0 and MASTER are free
// on both. So an addon reads HmIP :0 VALUES freely, never a BidCos value channel.
func (c *Client) GetParamset(ctx context.Context, iface, address, paramset string) (map[string]json.RawMessage, error) {
	raw, err := c.Call(ctx, iface, "getParamset", address, paramset)
	if err != nil {
		return nil, err
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ParamDesc is one parameter's description, the fields an addon reads; Raw keeps the whole.
type ParamDesc struct {
	Type       string          `json:"TYPE"`
	Operations int             `json:"OPERATIONS"`
	Flags      int             `json:"FLAGS"`
	Min        json.RawMessage `json:"MIN"`
	Max        json.RawMessage `json:"MAX"`
	Default    json.RawMessage `json:"DEFAULT"`
	Unit       string          `json:"UNIT"`
	ValueList  []string        `json:"VALUE_LIST"`
	ID         string          `json:"ID"`
	Raw        json.RawMessage `json:"-"`
}

// UnmarshalJSON keeps the raw description beside the typed fields.
func (d *ParamDesc) UnmarshalJSON(b []byte) error {
	type plain ParamDesc
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*d = ParamDesc(p)
	d.Raw = append(json.RawMessage(nil), b...)
	return nil
}

// Paramset is one paramset description: parameter id → description.
type Paramset map[string]ParamDesc

// ParamsetCache is GET /api/rpc/v1/paramsets: the whole cache in node-red-contrib-ccu's
// paramsets.json schema, keyed "<interface>/<device type>/<firmware>/<version>/<channel
// type>/<paramset>" (the channel type empty for the device's own).
type ParamsetCache map[string]Paramset

// Paramsets reads the whole paramset description cache.
func (c *Client) Paramsets(ctx context.Context) (ParamsetCache, error) {
	var out ParamsetCache
	err := c.do(ctx, http.MethodGet, "/api/rpc/v1/paramsets", nil, &out)
	return out, err
}

// ParamsetsFor is POST /api/rpc/v1/paramsets/{iface}: the VALUES descriptions of the given
// channels or devices, each once by its key; keys maps an address to its key, errors holds the
// addresses the process did not know.
func (c *Client) ParamsetsFor(ctx context.Context, iface string, addresses []string, paramset string) (keys map[string]string, sets ParamsetCache, errs map[string]string, err error) {
	body := map[string]any{"addresses": addresses}
	if paramset != "" {
		body["paramset"] = paramset
	}
	var out struct {
		Keys      map[string]string `json:"keys"`
		Paramsets ParamsetCache     `json:"paramsets"`
		Errors    map[string]string `json:"errors"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/rpc/v1/paramsets/"+url.PathEscape(iface), body, &out); err != nil {
		return nil, nil, nil, err
	}
	return out.Keys, out.Paramsets, out.Errors, nil
}
