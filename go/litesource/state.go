package litesource

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

// StateEntry is one entry of GET /api/rpc/v1/state; the same fields as an Event, with the
// store's words: Source is event, sweep or restored, Confirmed false for a restored entry.
type StateEntry = Event

// Snapshot is what State answered: the entries, the stream position to resume from, and the
// store's sweeps per interface.
type Snapshot struct {
	Entries     []StateEntry
	EventID     string
	Total       int
	Unconfirmed int
	Datapoints  []string
	Sweeps      map[string]Sweep
}

// Sweep is the state store's last sweep of one interface.
type Sweep struct {
	At       string  `json:"at"`
	Channels int     `json:"channels"`
	Read     int     `json:"read"`
	Failed   int     `json:"failed"`
	Error    string  `json:"error,omitempty"`
	Seconds  float64 `json:"seconds"`
}

// State reads the state store, every page, for the filter (Interfaces, Addresses, Keys; the
// other fields are ignored). EventID is the stream's position before the read: open Events with
// it and nothing in between is lost.
func (c *Client) State(ctx context.Context, f Filter) (*Snapshot, error) {
	q := Filter{Interfaces: f.Interfaces, Addresses: f.Addresses, Keys: f.Keys}.query()
	q.Set("limit", "5000")
	snap := &Snapshot{Sweeps: map[string]Sweep{}}
	after := ""
	for {
		if after != "" {
			q.Set("after", after)
		}
		var page struct {
			Entries []struct {
				StateEntry
				Datapoint string `json:"datapoint"`
			} `json:"entries"`
			Total       int              `json:"total"`
			Unconfirmed int              `json:"unconfirmed"`
			Next        string           `json:"next"`
			EventID     string           `json:"event_id"`
			Sweeps      map[string]Sweep `json:"sweeps"`
			Datapoints  []string         `json:"datapoints"`
		}
		if err := c.do(ctx, http.MethodGet, "/api/rpc/v1/state?"+q.Encode(), nil, &page); err != nil {
			return nil, err
		}
		for _, e := range page.Entries {
			ent := e.StateEntry
			if ent.Key == "" {
				ent.Key = e.Datapoint
			}
			snap.Entries = append(snap.Entries, ent)
		}
		if snap.EventID == "" {
			snap.EventID = page.EventID
			snap.Total = page.Total
			snap.Unconfirmed = page.Unconfirmed
			snap.Datapoints = page.Datapoints
			for k, v := range page.Sweeps {
				snap.Sweeps[k] = v
			}
		}
		if page.Next == "" {
			return snap, nil
		}
		after = page.Next
	}
}

// HistoryRow is one row of GET /history: the time in milliseconds since the epoch and the value.
type HistoryRow struct {
	TS    int64
	Value json.RawMessage
}

// History reads one series of the datapoint history (the last 500 rows of a datapoint on the
// system's list); ErrNotRecorded when the datapoint is not recorded.
func (c *Client) History(ctx context.Context, iface, address, datapoint string) ([]HistoryRow, error) {
	q := Filter{Interfaces: []string{iface}, Addresses: []string{address}, Keys: []string{datapoint}}.query()
	var rows [][]json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/rpc/v1/history?"+q.Encode(), nil, &rows); err != nil {
		var he *HTTPError
		if isHTTP(err, &he) && he.Code == "not-recorded" {
			return nil, ErrNotRecorded
		}
		return nil, err
	}
	out := make([]HistoryRow, 0, len(rows))
	for _, r := range rows {
		if len(r) != 2 {
			continue
		}
		ts, err := strconv.ParseInt(string(r[0]), 10, 64)
		if err != nil {
			continue
		}
		out = append(out, HistoryRow{TS: ts, Value: r[1]})
	}
	return out, nil
}

// ErrNotRecorded is History's answer for a datapoint not on the system's list.
var ErrNotRecorded = &HTTPError{Status: 404, Code: "not-recorded"}
