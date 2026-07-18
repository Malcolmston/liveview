package liveview

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"html"
	"net/http"
	"sync"
)

// Handler is an [http.Handler] that serves a live view over HTTP plus a
// self-implemented RFC 6455 WebSocket. It has these behaviours on its mount
// path:
//
//   - GET (no upgrade)  serves the full initial HTML page: a fresh session is
//     mounted, its rendered HTML is embedded, and the served JS client is
//     inlined.
//   - GET (websocket)   upgrades to a WebSocket, mounts a session, ships the
//     initial full diff, then streams events in and diffs out.
//   - POST {prefix}/event  the legacy JSON event route ({"session","event",
//     "payload"} in, [Diff] out), kept for non-socket drivers and tests.
//
// Sessions are held in memory and share one [PubSub] hub, so views can
// Subscribe/Broadcast across connections. This is a reference transport; a
// production deployment would add authentication and session eviction, but the
// state -> render -> diff core is identical.
type Handler struct {
	newView func() View
	prefix  string
	pubsub  *PubSub

	mu       sync.Mutex
	sessions map[string]*Session
}

// NewHandler returns a Handler that builds a fresh View (via factory) for each
// mounted session. prefix is the URL path the handler is served under and is
// used to construct the client WebSocket URL; "" is treated as "/".
func NewHandler(prefix string, factory func() View) *Handler {
	if prefix == "" {
		prefix = "/"
	}
	return &Handler{
		newView:  factory,
		prefix:   prefix,
		pubsub:   NewPubSub(),
		sessions: make(map[string]*Session),
	}
}

// PubSub returns the hub shared by every session this handler mounts, so
// application code outside a request can broadcast to connected views.
func (h *Handler) PubSub() *PubSub { return h.pubsub }

// Session returns a live session by id, or nil.
func (h *Handler) Session(id string) *Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[id]
}

func (h *Handler) put(s *Session) {
	h.mu.Lock()
	h.sessions[s.id] = s
	h.mu.Unlock()
}

func (h *Handler) drop(id string) {
	h.mu.Lock()
	delete(h.sessions, id)
	h.mu.Unlock()
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case isWebSocketUpgrade(r):
		h.serveWS(w, r)
	case r.Method == http.MethodPost:
		h.serveEvent(w, r)
	default:
		h.servePage(w, r)
	}
}

// newSession builds, mounts, and returns a fresh session for a request, wiring
// in the shared PubSub and recording the request URI.
func (h *Handler) newSession(r *http.Request) (*Session, *Rendered, error) {
	sess := NewSession(h.newView())
	sess.id = newID()
	sess.AttachPubSub(h.pubsub)
	sess.SetURI(r.URL.RequestURI())
	params := map[string]any{}
	for k, vs := range r.URL.Query() {
		if len(vs) > 0 {
			params[k] = vs[0]
		}
	}
	rendered, err := sess.Mount(params)
	if err != nil {
		return nil, nil, err
	}
	return sess, rendered, nil
}

func (h *Handler) servePage(w http.ResponseWriter, r *http.Request) {
	sess, rendered, err := h.newSession(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.put(sess)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page := "<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>liveview</title></head><body>\n" +
		"<div id=\"lv-root\" data-session=\"" + html.EscapeString(sess.id) + "\">" +
		rendered.HTML() +
		"</div>\n<script>" + clientJS(h.prefix) + "</script>\n</body></html>\n"
	_, _ = w.Write([]byte(page))
}

// serveWS upgrades the request to a WebSocket, mounts a session, and runs the
// event/diff loop until the client disconnects.
func (h *Handler) serveWS(w http.ResponseWriter, r *http.Request) {
	sess, _, err := h.newSession(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	conn, err := Upgrade(w, r)
	if err != nil {
		return
	}
	sess.SetConnected(true)
	h.put(sess)
	h.socketLoop(conn, sess)
}

// socketLoop drives one WebSocket session: it ships the initial diff, then
// multiplexes inbound client messages and outbound server-push messages,
// serializing all writes on this goroutine.
func (h *Handler) socketLoop(conn *Conn, sess *Session) {
	defer func() {
		sess.Close()
		h.drop(sess.id)
		_ = conn.Close()
	}()

	_ = writeReply(conn, "mount", sess.InitialDiff())

	type inbound struct {
		data []byte
		err  error
	}
	reads := make(chan inbound, 1)
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			reads <- inbound{data: data, err: err}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case in := <-reads:
			if in.err != nil {
				return
			}
			if diff, ok := h.dispatch(sess, in.data); ok {
				_ = writeReply(conn, "diff", diff)
			}
		case msg := <-sess.Inbox():
			if diff, err := sess.Info(msg); err == nil {
				_ = writeReply(conn, "diff", diff)
			}
		}
	}
}

// clientMessage is the JSON envelope the browser client sends over the socket.
type clientMessage struct {
	Type     string         `json:"type"`
	Event    string         `json:"event"`
	CID      *int           `json:"cid"`
	Payload  map[string]any `json:"payload"`
	URI      string         `json:"uri"`
	Name     string         `json:"name"`
	Ref      string         `json:"ref"`
	FileName string         `json:"file_name"`
	Size     int64          `json:"size"`
	MIME     string         `json:"mime"`
	Data     string         `json:"data"`
	Last     bool           `json:"last"`
}

// dispatch decodes and routes one client message, returning the diff to send
// back and whether a reply should be written.
func (h *Handler) dispatch(sess *Session, data []byte) (Diff, bool) {
	var m clientMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, false
	}
	switch m.Type {
	case "event":
		var diff Diff
		var err error
		if m.CID != nil {
			diff, err = sess.ComponentEvent(*m.CID, m.Event, m.Payload)
		} else {
			diff, err = sess.Event(m.Event, m.Payload)
		}
		if err != nil {
			return nil, false
		}
		return diff, true
	case "patch":
		diff, err := sess.Params(nil, m.URI)
		if err != nil {
			return nil, false
		}
		return diff, true
	case "upload_start":
		if err := sess.RegisterUploadEntry(m.Name, m.Ref, m.FileName, m.Size, m.MIME); err != nil {
			return nil, false
		}
		return Diff{}, true
	case "upload_chunk":
		raw, err := base64.StdEncoding.DecodeString(m.Data)
		if err != nil {
			return nil, false
		}
		diff, err := sess.UploadChunk(m.Name, m.Ref, raw, m.Last)
		if err != nil {
			return nil, false
		}
		return diff, true
	}
	return nil, false
}

// writeReply marshals {"type":typ,"diff":diff} and writes it as a text frame.
func writeReply(conn *Conn, typ string, diff Diff) error {
	b, err := json.Marshal(map[string]any{"type": typ, "diff": diff})
	if err != nil {
		return err
	}
	return conn.WriteText(string(b))
}

// eventRequest is the JSON body accepted by the legacy event route.
type eventRequest struct {
	Session string         `json:"session"`
	Event   string         `json:"event"`
	Payload map[string]any `json:"payload"`
}

func (h *Handler) serveEvent(w http.ResponseWriter, r *http.Request) {
	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	sess := h.Session(req.Session)
	if sess == nil {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	diff, err := sess.Event(req.Event, req.Payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(diff)
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read essentially never fails; fall back to a fixed marker so the
		// handler stays usable rather than panicking.
		return "session-fallback"
	}
	return hex.EncodeToString(b[:])
}
