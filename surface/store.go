package surface

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// Component is one entry of the adjacency list: an id, a type name, and the
// whole object kept raw. Properties are decoded when the component is drawn,
// so a component whose properties do not fit its type fails in its own pane
// and nowhere else (E4), instead of failing the message that carried it.
type Component struct {
	ID   string
	Type string
	Raw  json.RawMessage
}

// Surface is one A2UI surface: its catalog, its components by id and its data
// model. A Surface is a value; apply returns a new one and never writes into
// maps an older copy still holds.
type Surface struct {
	ID            string
	CatalogID     string
	Title         string // theme.agentDisplayName, else ID
	SendDataModel bool
	Components    map[string]Component
	Data          any
}

type themeProps struct {
	AgentDisplayName string `json:"agentDisplayName"`
}

// state is every live surface, in creation order. The last one is drawn.
type state struct {
	order    []string
	surfaces map[string]Surface
}

func (s state) active() (Surface, bool) {
	if len(s.order) == 0 {
		return Surface{}, false
	}
	sf, ok := s.surfaces[s.order[len(s.order)-1]]
	return sf, ok
}

// apply runs one message. On error the returned state is s unchanged: a
// message is the unit of atomicity, so a half-applied updateComponents never
// reaches the screen.
func (s state) apply(m message) (state, error) {
	switch {
	case m.CreateSurface != nil:
		c := m.CreateSurface
		if _, ok := s.surfaces[c.SurfaceID]; ok {
			return s, perr(c.SurfaceID, "/createSurface/surfaceId", "surface %q already exists; delete it first", c.SurfaceID)
		}
		if c.CatalogID != CatalogID && c.CatalogID != BasicCatalogID {
			return s, perr(c.SurfaceID, "/createSurface/catalogId", "unsupported catalog %q (supported: %s, %s)", c.CatalogID, CatalogID, BasicCatalogID)
		}
		sf := Surface{ID: c.SurfaceID, CatalogID: c.CatalogID, Title: c.SurfaceID, SendDataModel: c.SendDataModel, Components: map[string]Component{}}
		if len(c.Theme) > 0 {
			var th themeProps
			if err := json.Unmarshal(c.Theme, &th); err != nil {
				return s, perr(c.SurfaceID, "/createSurface/theme", "%v", err)
			}
			if th.AgentDisplayName != "" {
				sf.Title = th.AgentDisplayName
			}
		}
		return s.with(sf, true), nil
	case m.UpdateComponents != nil:
		u := m.UpdateComponents
		sf, ok := s.surfaces[u.SurfaceID]
		if !ok {
			return s, perr(u.SurfaceID, "/updateComponents/surfaceId", "unknown surface %q", u.SurfaceID)
		}
		comps := maps.Clone(sf.Components)
		for i, raw := range u.Components {
			c, err := decodeComponent(raw)
			if err != nil {
				return s, perr(u.SurfaceID, fmt.Sprintf("/updateComponents/components/%d", i), "%v", err)
			}
			comps[c.ID] = c
		}
		sf.Components = comps
		return s.with(sf, false), nil
	case m.UpdateDataModel != nil:
		u := m.UpdateDataModel
		sf, ok := s.surfaces[u.SurfaceID]
		if !ok {
			return s, perr(u.SurfaceID, "/updateDataModel/surfaceId", "unknown surface %q", u.SurfaceID)
		}
		var next any
		var err error
		if u.hasValue {
			var v any
			if err := json.Unmarshal(u.Value, &v); err != nil {
				return s, perr(u.SurfaceID, "/updateDataModel/value", "%v", err)
			}
			next, err = setPath(sf.Data, u.Path, v)
		} else {
			next, err = removePath(sf.Data, u.Path)
		}
		if err != nil {
			return s, perr(u.SurfaceID, "/updateDataModel/path", "%v", err)
		}
		sf.Data = next
		return s.with(sf, false), nil
	case m.DeleteSurface != nil:
		id := m.DeleteSurface.SurfaceID
		if _, ok := s.surfaces[id]; !ok {
			return s, perr(id, "/deleteSurface/surfaceId", "unknown surface %q", id)
		}
		out := state{order: slices.DeleteFunc(slices.Clone(s.order), func(x string) bool { return x == id }), surfaces: maps.Clone(s.surfaces)}
		delete(out.surfaces, id)
		return out, nil
	}
	return s, perr("", "", "empty message")
}

func (s state) with(sf Surface, created bool) state {
	out := state{order: slices.Clone(s.order), surfaces: maps.Clone(s.surfaces)}
	if out.surfaces == nil {
		out.surfaces = map[string]Surface{}
	}
	out.surfaces[sf.ID] = sf
	if created {
		out.order = append(out.order, sf.ID)
	}
	return out
}

// decodeComponent checks only what every catalog shares: an id and a
// component name. A v0.8-style wrapper ({"Text": {...}}) is rejected here
// with a message that says so, since agents still emit it.
func decodeComponent(raw json.RawMessage) (Component, error) {
	var head struct {
		ID        string `json:"id"`
		Component string `json:"component"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return Component{}, err
	}
	if head.ID == "" {
		return Component{}, fmt.Errorf("component has no id")
	}
	if head.Component == "" {
		return Component{}, fmt.Errorf("component %q has no \"component\" type (v0.9 is {\"id\":…,\"component\":\"Text\",…})", head.ID)
	}
	return Component{ID: head.ID, Type: head.Component, Raw: raw}, nil
}

// setData is the renderer's own write to the model: an input's two-way
// binding or a list's selection. It goes through the same copying write as
// updateDataModel.
func (s state) setData(surface, path string, v any) (state, error) {
	sf, ok := s.surfaces[surface]
	if !ok {
		return s, fmt.Errorf("unknown surface %q", surface)
	}
	next, err := setPath(sf.Data, path, v)
	if err != nil {
		return s, err
	}
	sf.Data = next
	return s.with(sf, false), nil
}
