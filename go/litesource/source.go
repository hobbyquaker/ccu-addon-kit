package litesource

import "context"

// Names is what a consumer wants of the metadata: the name and rooms of every device and channel,
// keyed <interface>.<address>, and the revision it was read at.
type Names struct {
	Revision int64
	Objects  map[string]MetaObject
	snap     *MetaSnapshot
}

// Name answers the name of an interface and address, "" when unknown.
func (n *Names) Name(iface, address string) string {
	if n == nil {
		return ""
	}
	return n.Objects[iface+"."+address].Name
}

// RoomNames answers the names of the rooms the object is in, in lang.
func (n *Names) RoomNames(iface, address, lang string) []string {
	return n.EnumNames(iface, address, "room/", lang)
}

// FunctionNames answers the names of the functions (Gewerke) the object is in, in lang.
func (n *Names) FunctionNames(iface, address, lang string) []string {
	return n.EnumNames(iface, address, "function/", lang)
}

// EnumNames answers the names, in lang, of the nodes of one enum the object is in: prefix is
// the enum's path prefix ("room/", "function/", "favorite/").
func (n *Names) EnumNames(iface, address, prefix, lang string) []string {
	if n == nil || n.snap == nil {
		return nil
	}
	var out []string
	for _, p := range n.Objects[iface+"."+address].pathsOf(prefix) {
		out = append(out, n.snap.NodeName(p, lang))
	}
	return out
}

// Snapshot answers the metadata snapshot the names were read from (nil for Names built without
// one), for a consumer that needs the enum trees themselves.
func (n *Names) Snapshot() *MetaSnapshot {
	if n == nil {
		return nil
	}
	return n.snap
}

// Source is the seam between an addon and the system it runs on: openccu-lite here, a classic
// CCU (XML-RPC callbacks, ReGa names) in a later implementation. An addon records from Seed and
// Subscribe and shows names from Names.
type Source interface {
	// Seed answers the last known values of the filter's datapoints and the position to
	// subscribe from so that nothing in between is lost.
	Seed(ctx context.Context, f Filter) (*Snapshot, error)
	// Subscribe delivers the live reports; a Resync message means Seed again.
	Subscribe(ctx context.Context, f Filter, since string) <-chan Message
	// Devices lists the devices and channels of an interface.
	Devices(ctx context.Context, iface string) ([]DeviceDescription, error)
	// Interfaces names the interface processes.
	Interfaces(ctx context.Context) ([]Interface, error)
	// Read reads one paramset of a channel (what it costs on air is the caller's concern).
	Read(ctx context.Context, iface, address, paramset string) (map[string]RawValue, error)
	// Descriptions answers the VALUES descriptions of addresses, each once by its key.
	Descriptions(ctx context.Context, iface string, addresses []string) (keys map[string]string, sets ParamsetCache, err error)
	// Names reads the names and rooms.
	Names(ctx context.Context) (*Names, error)
	// NameEvents follows name changes from a revision; a resync event means Names again.
	NameEvents(ctx context.Context, since int64) <-chan MetaEvent
}

// RawValue is a JSON value as the process gave it.
type RawValue = []byte

// Seed implements Source over the state store.
func (c *Client) Seed(ctx context.Context, f Filter) (*Snapshot, error) { return c.State(ctx, f) }

// Subscribe implements Source over the event stream.
func (c *Client) Subscribe(ctx context.Context, f Filter, since string) <-chan Message {
	return c.Events(ctx, f, since)
}

// Devices implements Source.
func (c *Client) Devices(ctx context.Context, iface string) ([]DeviceDescription, error) {
	return c.ListDevices(ctx, iface)
}

// Read implements Source.
func (c *Client) Read(ctx context.Context, iface, address, paramset string) (map[string]RawValue, error) {
	m, err := c.GetParamset(ctx, iface, address, paramset)
	if err != nil {
		return nil, err
	}
	out := make(map[string]RawValue, len(m))
	for k, v := range m {
		out[k] = RawValue(v)
	}
	return out, nil
}

// Descriptions implements Source.
func (c *Client) Descriptions(ctx context.Context, iface string, addresses []string) (map[string]string, ParamsetCache, error) {
	keys, sets, _, err := c.ParamsetsFor(ctx, iface, addresses, "VALUES")
	return keys, sets, err
}

// Names implements Source over the metadata snapshot.
func (c *Client) Names(ctx context.Context) (*Names, error) {
	s, err := c.Meta().Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return &Names{Revision: s.Revision, Objects: s.Objects, snap: s}, nil
}

// NameEvents implements Source over the metadata change stream.
func (c *Client) NameEvents(ctx context.Context, since int64) <-chan MetaEvent {
	return c.Meta().Events(ctx, since)
}

var _ Source = (*Client)(nil)
