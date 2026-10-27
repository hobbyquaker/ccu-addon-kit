package litesource

// NewNames builds Names from a metadata snapshot - for another Source (a classic CCU's ReGa
// names in ccusource) that assembles the snapshot itself: the objects keyed interface.address
// with their Name and room paths, the room enum's tree for the room names.
func NewNames(snap *MetaSnapshot) *Names {
	if snap == nil {
		return nil
	}
	return &Names{Revision: snap.Revision, Objects: snap.Objects, snap: snap}
}
