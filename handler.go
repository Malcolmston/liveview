package liveview

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html"
	"net/http"
	"sync"
)

// Handler is an [http.Handler] that serves a live view over plain HTTP + JSON.
// It has two routes relative to its mount path:
//
//   - GET  {prefix}/         serves the full initial HTML page (a new session is
//     mounted per request and its id is embedded in the page).
//   - POST {prefix}/event    accepts {"session","event","payload"} JSON, runs
//     the event on the named session, and replies with the [Diff] as JSON.
//
// Sessions are held in memory. This is a reference transport; a production
// deployment would layer authentication, session eviction, and a websocket on
// top, but the state -> render -> diff core is identical.
type Handler struct {
	newView func() View
	prefix  string

	mu       sync.Mutex
	sessions map[string]*Session
}

// NewHandler returns a Handler that builds a fresh View (via factory) for each
// mounted session. prefix is the URL path the handler is served under and is
// used to construct the client event URL; "" is treated as "/".
func NewHandler(prefix string, factory func() View) *Handler {
	if prefix == "" {
		prefix = "/"
	}
	return &Handler{
		newView:  factory,
		prefix:   prefix,
		sessions: make(map[string]*Session),
	}
}

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

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.serveEvent(w, r)
	default:
		h.servePage(w, r)
	}
}

func (h *Handler) servePage(w http.ResponseWriter, r *http.Request) {
	sess := NewSession(h.newView())
	sess.id = newID()
	params := map[string]any{}
	for k, vs := range r.URL.Query() {
		if len(vs) > 0 {
			params[k] = vs[0]
		}
	}
	rendered, err := sess.Mount(params)
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

// eventRequest is the JSON body accepted by the event route.
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

// clientJS is a tiny illustrative browser stub: elements carrying a
// data-lv-click attribute send that event name to the server and log the diff.
// It is intentionally minimal — enough to demonstrate the wire protocol without
// pulling in a build step.
func clientJS(prefix string) string {
	url := prefix
	if url == "/" {
		url = ""
	}
	return `
(function(){
  var root=document.getElementById('lv-root');
  var session=root.getAttribute('data-session');
  function send(event,payload){
    fetch('` + url + `/event',{method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({session:session,event:event,payload:payload||{}})})
      .then(function(r){return r.json();}).then(function(diff){console.log('diff',diff);});
  }
  document.addEventListener('click',function(e){
    var t=e.target.closest('[data-lv-click]');
    if(t){send(t.getAttribute('data-lv-click'),{});}
  });
})();`
}
