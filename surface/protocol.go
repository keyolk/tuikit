package surface

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Version is the A2UI protocol version this package speaks. v0.9.1 is
// accepted on input too: its envelope differs from v0.9 only in the version
// string (specification/v0_9_1/json/server_to_client.json).
const Version = "v0.9"

// CatalogID is the terminal catalog. A catalogId is an agreed string, not a
// URL that must resolve (a2ui_protocol.md, "Catalog Identification").
const CatalogID = "tuikit.terminal/v1"

// BasicCatalogID is the A2UI basic catalog. Surfaces that name it are drawn
// with the subset its components share with the terminal catalog; the rest
// show a placeholder (see docs/surface.md).
const BasicCatalogID = "https://a2ui.org/specification/v0_9/catalogs/basic/catalog.json"

// message is one server-to-client envelope after decoding. Exactly one of
// the pointers is set.
type message struct {
	Version          string
	at               string // "/3" for the 4th element of an array line
	CreateSurface    *createSurface
	UpdateComponents *updateComponents
	UpdateDataModel  *updateDataModel
	DeleteSurface    *deleteSurface
}

type createSurface struct {
	SurfaceID     string          `json:"surfaceId"`
	CatalogID     string          `json:"catalogId"`
	Theme         json.RawMessage `json:"theme"`
	SendDataModel bool            `json:"sendDataModel"`
}

type updateComponents struct {
	SurfaceID  string            `json:"surfaceId"`
	Components []json.RawMessage `json:"components"`
}

type updateDataModel struct {
	SurfaceID string          `json:"surfaceId"`
	Path      string          `json:"path"`
	Value     json.RawMessage `json:"value"`
	hasValue  bool
}

type deleteSurface struct {
	SurfaceID string `json:"surfaceId"`
}

// ProtocolError is a message the renderer refused. Path is a JSON pointer
// into the message, the shape the spec's VALIDATION_FAILED error carries.
type ProtocolError struct {
	SurfaceID string
	Path      string
	Msg       string
}

func (e *ProtocolError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s: %s", e.Path, e.Msg)
	}
	return e.Msg
}

func perr(surface, path, format string, a ...any) *ProtocolError {
	return &ProtocolError{SurfaceID: surface, Path: path, Msg: fmt.Sprintf(format, a...)}
}

var envelopeKeys = []string{"createSurface", "updateComponents", "updateDataModel", "deleteSurface"}

// decodeLine turns one JSONL line into messages: an envelope object, or an
// array of them (the server_to_client_list form an A2A DataPart carries).
// An array is not a transaction: each element succeeds or fails alone, as
// the extension spec's "Processing Rules" require.
func decodeLine(line []byte) ([]message, []error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil, nil
	}
	if line[0] == '[' {
		var raws []json.RawMessage
		if err := json.Unmarshal(line, &raws); err != nil {
			return nil, []error{perr("", "", "malformed JSON: %v", err)}
		}
		var msgs []message
		var errs []error
		for i, r := range raws {
			m, err := decodeMessage(r)
			if err != nil {
				var pe *ProtocolError
				if errors.As(err, &pe) {
					pe.Path = fmt.Sprintf("/%d%s", i, pe.Path)
				}
				errs = append(errs, err)
				continue
			}
			m.at = fmt.Sprintf("/%d", i)
			msgs = append(msgs, m)
		}
		return msgs, errs
	}
	m, err := decodeMessage(line)
	if err != nil {
		return nil, []error{err}
	}
	return []message{m}, nil
}

func decodeMessage(raw []byte) (message, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return message{}, perr("", "", "malformed JSON: %v", err)
	}
	var m message
	if v, ok := obj["version"]; ok {
		if err := json.Unmarshal(v, &m.Version); err != nil {
			return message{}, perr("", "/version", "version must be a string")
		}
	}
	if m.Version != "v0.9" && m.Version != "v0.9.1" {
		return message{}, perr("", "/version", "unsupported version %q (want %s)", m.Version, Version)
	}
	var kind string
	for _, k := range envelopeKeys {
		if _, ok := obj[k]; ok {
			if kind != "" {
				return message{}, perr("", "", "message has both %s and %s", kind, k)
			}
			kind = k
		}
	}
	body := obj[kind]
	strict := func(v any) error {
		d := json.NewDecoder(bytes.NewReader(body))
		d.DisallowUnknownFields()
		if err := d.Decode(v); err != nil {
			return perr("", "/"+kind, "%v", err)
		}
		return nil
	}
	var err error
	switch kind {
	case "":
		return message{}, perr("", "", "no createSurface, updateComponents, updateDataModel or deleteSurface")
	case "createSurface":
		m.CreateSurface = &createSurface{}
		err = strict(m.CreateSurface)
		if err == nil && (m.CreateSurface.SurfaceID == "" || m.CreateSurface.CatalogID == "") {
			err = perr(m.CreateSurface.SurfaceID, "/createSurface", "surfaceId and catalogId are required")
		}
	case "updateComponents":
		m.UpdateComponents = &updateComponents{}
		err = strict(m.UpdateComponents)
		if err == nil && (m.UpdateComponents.SurfaceID == "" || len(m.UpdateComponents.Components) == 0) {
			err = perr(m.UpdateComponents.SurfaceID, "/updateComponents", "surfaceId and a non-empty components list are required")
		}
	case "updateDataModel":
		m.UpdateDataModel = &updateDataModel{}
		err = strict(m.UpdateDataModel)
		if err == nil {
			// "value omitted" removes the key; an explicit null is a value.
			var inner map[string]json.RawMessage
			if jerr := json.Unmarshal(body, &inner); jerr != nil {
				return message{}, perr("", "/updateDataModel", "%v", jerr)
			}
			_, m.UpdateDataModel.hasValue = inner["value"]
			if m.UpdateDataModel.SurfaceID == "" {
				err = perr("", "/updateDataModel", "surfaceId is required")
			}
		}
	case "deleteSurface":
		m.DeleteSurface = &deleteSurface{}
		err = strict(m.DeleteSurface)
		if err == nil && m.DeleteSurface.SurfaceID == "" {
			err = perr("", "/deleteSurface", "surfaceId is required")
		}
	}
	return m, err
}

// Action is the client-to-server `action` message (client_to_server.json):
// what the renderer writes when the user triggers an agent action.
type Action struct {
	Name              string         `json:"name"`
	SurfaceID         string         `json:"surfaceId"`
	SourceComponentID string         `json:"sourceComponentId"`
	Timestamp         string         `json:"timestamp"`
	Context           map[string]any `json:"context"`
}

// ClientError is the client-to-server `error` message. Code is
// VALIDATION_FAILED for a message that broke the protocol, which lets an
// agent correct itself (a2ui_protocol.md, "Standard validation error format").
type ClientError struct {
	Code      string `json:"code"`
	SurfaceID string `json:"surfaceId"`
	Path      string `json:"path,omitempty"`
	Message   string `json:"message"`
}

// Event is one line the renderer writes back: an action or an error.
type Event struct {
	Version string       `json:"version"`
	Action  *Action      `json:"action,omitempty"`
	Error   *ClientError `json:"error,omitempty"`
}

func actionEvent(a Action) Event { return Event{Version: Version, Action: &a} }

func errorEvent(err error) Event {
	ce := &ClientError{Code: "VALIDATION_FAILED", Message: err.Error()}
	var pe *ProtocolError
	if errors.As(err, &pe) {
		ce.SurfaceID, ce.Path, ce.Message = pe.SurfaceID, pe.Path, pe.Msg
	}
	if ce.Path == "" {
		ce.Path = "/" // the schema requires a path for VALIDATION_FAILED
	}
	return Event{Version: Version, Error: ce}
}

func timestamp(now time.Time) string { return now.UTC().Format(time.RFC3339Nano) }
