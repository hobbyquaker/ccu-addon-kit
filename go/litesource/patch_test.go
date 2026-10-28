package litesource_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"
	"github.com/hobbyquaker/ccu-addon-kit/go/litesource/fakelite"
)

func TestPatchObject(t *testing.T) {
	s := fakelite.New()
	defer s.Close()
	c := newClient(t, s)
	ctx := context.Background()
	rev0 := s.SetName("BidCos-RF.BidCoS-RF:48", "BidCoS-RF:48", "room/eg")
	other := map[string]any{"x": 1}

	// the name and two namespaces: a new revision, the enums left alone
	name := "Flur oben"
	rev, changed, err := c.Meta().PatchObject(ctx, "BidCos-RF", "BidCoS-RF:48", litesource.ObjectPatch{
		Name: &name, Meta: map[string]any{"longpressdim": map[string]string{"binding": "b1", "role": "up"}, "other": other},
	})
	if err != nil || !changed || rev != rev0+1 {
		t.Fatalf("first write: rev %d changed %v err %v", rev, changed, err)
	}
	if w := s.MetaWrites[len(s.MetaWrites)-1]; !strings.HasPrefix(w, "BidCos-RF.BidCoS-RF:48 ") || strings.Contains(w, `"enums"`) {
		t.Fatalf("body: %s", w)
	}
	snap, _ := c.Meta().Snapshot(ctx)
	o, _ := snap.Object("BidCos-RF", "BidCoS-RF:48")
	if o.Name != name || len(o.Enums) != 1 || string(o.Meta["longpressdim"]) != `{"binding":"b1","role":"up"}` || o.Meta["other"] == nil {
		t.Fatalf("object: %+v", o)
	}

	// the same again: the store's 304 is "unchanged", not an error, with the revision
	rev2, changed, err := c.Meta().PatchObject(ctx, "BidCos-RF", "BidCoS-RF:48", litesource.ObjectPatch{
		Meta: map[string]any{"longpressdim": map[string]string{"binding": "b1", "role": "up"}},
	})
	if err != nil || changed || rev2 != rev {
		t.Fatalf("unchanged write: rev %d changed %v err %v", rev2, changed, err)
	}

	// nil removes the namespace and only that one; an empty enum list clears the memberships
	none := []string{}
	if _, changed, err := c.Meta().PatchObject(ctx, "BidCos-RF", "BidCoS-RF:48", litesource.ObjectPatch{
		Enums: &none, Meta: map[string]any{"longpressdim": nil},
	}); err != nil || !changed {
		t.Fatalf("removal: %v %v", changed, err)
	}
	if w := s.MetaWrites[len(s.MetaWrites)-1]; !strings.Contains(w, `"enums":[]`) || !strings.Contains(w, `"longpressdim":null`) {
		t.Fatalf("body: %s", w)
	}
	snap, _ = c.Meta().Snapshot(ctx)
	o, _ = snap.Object("BidCos-RF", "BidCoS-RF:48")
	if _, ok := o.Meta["longpressdim"]; ok || o.Meta["other"] == nil || len(o.Enums) != 0 {
		t.Fatalf("after the removal: %+v", o)
	}

	// a refusal is an HTTPError with the store's code
	empty := ""
	_, _, err = c.Meta().PatchObject(ctx, "BidCos-RF", "BidCoS-RF:48", litesource.ObjectPatch{Name: &empty})
	var he *litesource.HTTPError
	if !errors.As(err, &he) || he.Status != 422 || he.Code != "invalid-body" {
		t.Fatalf("empty name: %v", err)
	}

	// a ref with a colon arrives whole; the event stream sees the write
	if got := s.MetaEvents[len(s.MetaEvents)-1]["ref"]; got != "BidCos-RF.BidCoS-RF:48" {
		t.Fatalf("event ref %v", got)
	}
}
