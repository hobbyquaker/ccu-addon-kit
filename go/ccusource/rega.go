package ccusource

import (
	"context"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"
)

// namesScript lists every device and channel with its name and the rooms of the channel, one
// line each, tab-separated: D <interface> <address> <name> / C <address> <name> <room ids|>.
// Devices of interface processes that are gone are listed too; the recorder matches by address.
const namesScript = `string sDevId; string sChnId; string sRoomId;
foreach(sDevId, root.Devices().EnumUsedIDs()) {
  object oDev = dom.GetObject(sDevId);
  if (oDev) {
    object oIf = dom.GetObject(oDev.Interface());
    string sIf = "";
    if (oIf) { sIf = oIf.Name(); }
    WriteLine("D\t" # sIf # "\t" # oDev.Address() # "\t" # oDev.Name());
    foreach(sChnId, oDev.Channels().EnumUsedIDs()) {
      object oCh = dom.GetObject(sChnId);
      if (oCh) {
        string sRooms = "";
        foreach(sRoomId, oCh.ChnRoom()) { sRooms = sRooms # sRoomId # "|"; }
        WriteLine("C\t" # oCh.Address() # "\t" # oCh.Name() # "\t" # sRooms);
      }
    }
  }
}
string sId;
foreach(sId, dom.GetObject(ID_ROOMS).EnumUsedIDs()) {
  object oR = dom.GetObject(sId);
  if (oR) { WriteLine("R\t" # sId # "\t" # oR.Name()); }
}
`

// Rega runs an HM-Script and answers what it wrote (without rega.exe's <xml> trailer).
func (s *Source) Rega(ctx context.Context, script string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.o.Rega, strings.NewReader(script))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/plain")
	res, err := s.o.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return "", err
	}
	out := string(b)
	if i := strings.Index(out, "<xml>"); i >= 0 {
		out = out[:i]
	}
	return out, nil
}

// ParseNames turns the script's lines into a metadata snapshot: the objects keyed
// interface.address (a device's rooms are the union of its channels'), the room enum's tree.
func ParseNames(out string) *litesource.MetaSnapshot {
	snap := &litesource.MetaSnapshot{Format: 1, Objects: map[string]litesource.MetaObject{}, Enums: map[string]litesource.MetaEnum{}}
	rooms := map[string]string{}
	type chn struct {
		addr, name string
		rooms      []string
	}
	var devices []struct {
		iface, addr, name string
		channels          []chn
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		f := strings.Split(line, "\t")
		switch {
		case len(f) >= 4 && f[0] == "D":
			devices = append(devices, struct {
				iface, addr, name string
				channels          []chn
			}{iface: f[1], addr: f[2], name: f[3]})
		case len(f) >= 3 && f[0] == "C" && len(devices) > 0:
			c := chn{addr: f[1], name: f[2]}
			if len(f) >= 4 {
				for _, r := range strings.Split(f[3], "|") {
					if r != "" {
						c.rooms = append(c.rooms, r)
					}
				}
			}
			d := &devices[len(devices)-1]
			d.channels = append(d.channels, c)
		case len(f) >= 3 && f[0] == "R":
			rooms[f[1]] = f[2]
		}
	}
	var tree []litesource.MetaNode
	ids := make([]string, 0, len(rooms))
	for id := range rooms {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		tree = append(tree, litesource.MetaNode{ID: id, Name: litesource.MetaText{"": rooms[id]}})
	}
	snap.Enums["room"] = litesource.MetaEnum{Name: litesource.MetaText{"de": "Räume", "en": "Rooms"}, Tree: tree}
	for _, d := range devices {
		if d.iface == "" || d.addr == "" {
			continue
		}
		seen := map[string]bool{}
		var devRooms []string
		for _, c := range d.channels {
			var enums []string
			for _, r := range c.rooms {
				enums = append(enums, "room/"+r)
				if !seen[r] {
					seen[r] = true
					devRooms = append(devRooms, "room/"+r)
				}
			}
			snap.Objects[d.iface+"."+c.addr] = litesource.MetaObject{Name: c.name, Enums: enums}
		}
		snap.Objects[d.iface+"."+d.addr] = litesource.MetaObject{Name: d.name, Enums: devRooms}
	}
	return snap
}

// Names reads the names and rooms from ReGa.
func (s *Source) Names(ctx context.Context) (*litesource.Names, error) {
	out, err := s.Rega(ctx, namesScript)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.lastNames = out
	s.mu.Unlock()
	return litesource.NewNames(ParseNames(out)), nil
}

// NameEvents polls ReGa every NamesEvery and sends a resync event when the names changed.
func (s *Source) NameEvents(ctx context.Context, since int64) <-chan litesource.MetaEvent {
	ch := make(chan litesource.MetaEvent, 4)
	go func() {
		defer close(ch)
		t := time.NewTicker(s.o.NamesEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			out, err := s.Rega(ctx, namesScript)
			if err != nil {
				continue
			}
			s.mu.Lock()
			changed := out != s.lastNames
			s.lastNames = out
			s.mu.Unlock()
			if changed {
				select {
				case ch <- litesource.MetaEvent{Kind: "resync"}:
				default:
				}
			}
		}
	}()
	return ch
}
