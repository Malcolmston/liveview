package liveview

// clientJS returns the browser client as a self-contained JavaScript string.
// The client opens a WebSocket to the handler, applies the static/dynamic diffs
// it receives into the DOM, binds phx-* event bindings (with phx-debounce /
// phx-throttle), executes JS client commands, and streams file uploads back over
// the socket. It has no build step and no third-party dependencies, matching the
// package's standard-library-only ethos.
//
// wsPath is the path the WebSocket connects to (the handler's prefix).
func clientJS(wsPath string) string {
	if wsPath == "" {
		wsPath = "/"
	}
	return `(function(){
"use strict";
var WS_PATH = ` + jsString(wsPath) + `;
var root = document.getElementById("lv-root");
var tree = null;          // current static/dynamic tree
var comps = {};           // cid -> component tree
var socket = null;
var throttled = {};       // debounce/throttle timers keyed by element+event

// ---- diff application ------------------------------------------------------
function isObject(v){ return v && typeof v === "object" && !Array.isArray(v); }

// mergeTree applies a diff object onto a tree node in place, recursing into
// nested trees. The reserved "s" (statics), "c" (components), "stream", "e"
// (events) and "nav" keys are handled by the caller.
function mergeTree(node, diff){
  if (node == null) node = {};
  for (var k in diff){
    if (k === "s"){ node.s = diff.s; continue; }
    var val = diff[k];
    if (isObject(val) && !("__leaf" in val)){
      if (isObject(node[k])){ node[k] = mergeTree(node[k], val); }
      else { node[k] = val; }
    } else {
      node[k] = val;
    }
  }
  return node;
}

// renderTree turns a tree node into an HTML string by interleaving statics and
// dynamics. A numeric dynamic references a component by cid.
function renderTree(node){
  if (node == null) return "";
  var s = node.s || [];
  var out = "";
  for (var i=0;i<s.length;i++){
    out += s[i];
    if (i < s.length-1){
      var d = node[i];
      if (typeof d === "number"){ out += renderTree(comps[d]); }
      else if (isObject(d)){ out += renderTree(d); }
      else if (d != null){ out += d; }
    }
  }
  return out;
}

function applyComponents(cdiff){
  for (var cid in cdiff){ comps[cid] = mergeTree(comps[cid], cdiff[cid]); }
}

// applyDiff merges a diff into the current tree and re-renders the root,
// preserving stream containers and then applying any stream/nav/event effects.
function applyDiff(diff){
  if (diff.c){ applyComponents(diff.c); }
  var streams = diff.stream, events = diff.e, nav = diff.nav;
  delete diff.c; delete diff.stream; delete diff.e; delete diff.nav;
  tree = mergeTree(tree || {}, diff);
  var preserved = snapshotStreams();
  root.innerHTML = renderTree(tree);
  restoreStreams(preserved);
  bindUploads();
  if (streams){ for (var name in streams){ applyStream(name, streams[name]); } }
  if (events){ events.forEach(dispatchPush); }
  if (nav){ applyNav(nav); }
}

// ---- streams ---------------------------------------------------------------
function streamContainers(){ return document.querySelectorAll("[phx-update=stream]"); }
function snapshotStreams(){
  var snap = {};
  streamContainers().forEach(function(c){ if (c.id) snap[c.id] = c.innerHTML; });
  return snap;
}
function restoreStreams(snap){
  streamContainers().forEach(function(c){ if (c.id && snap[c.id] != null) c.innerHTML = snap[c.id]; });
}
function applyStream(name, ops){
  var c = document.getElementById(name);
  if (!c) return;
  ops.forEach(function(op){
    if (op.action === "reset"){ c.innerHTML = ""; return; }
    if (op.action === "delete"){ var el = document.getElementById(op.id); if (el) el.remove(); return; }
    if (op.action === "insert"){
      var tmp = document.createElement("template");
      tmp.innerHTML = op.html.trim();
      var el = tmp.content.firstChild;
      if (op.at === 0){ c.insertBefore(el, c.firstChild); } else { c.appendChild(el); }
    }
  });
}

// ---- navigation & push events ----------------------------------------------
function applyNav(nav){
  if (nav.kind === "patch"){
    if (nav.replace) history.replaceState({}, "", nav.to); else history.pushState({}, "", nav.to);
    send({type:"patch", uri: location.href});
  } else if (nav.kind === "navigate"){
    location.href = nav.to;
  }
}
function dispatchPush(ev){
  window.dispatchEvent(new CustomEvent("phx:"+ev.event, {detail: ev.payload || {}}));
}

// ---- JS client commands ----------------------------------------------------
function runCommands(cmds, el){
  cmds.forEach(function(c){
    var op = c[0], a = c[1] || {};
    var targets = a.to ? document.querySelectorAll(a.to) : [el];
    switch(op){
      case "push": send({type:"event", event:a.event, cid:a.target?cidOf(a.target):null, payload: gather(el)}); break;
      case "add_class": targets.forEach(function(t){ t.classList.add.apply(t.classList, a.names.split(/\s+/)); }); break;
      case "remove_class": targets.forEach(function(t){ t.classList.remove.apply(t.classList, a.names.split(/\s+/)); }); break;
      case "toggle_class": targets.forEach(function(t){ a.names.split(/\s+/).forEach(function(n){ t.classList.toggle(n); }); }); break;
      case "toggle": targets.forEach(function(t){ t.style.display = (t.style.display==="none"?"":"none"); }); break;
      case "show": targets.forEach(function(t){ t.style.display=""; }); break;
      case "hide": targets.forEach(function(t){ t.style.display="none"; }); break;
      case "set_attr": targets.forEach(function(t){ t.setAttribute(a.attr, a.value); }); break;
      case "dispatch": targets.forEach(function(t){ t.dispatchEvent(new Event(a.event, {bubbles:true})); }); break;
    }
  });
}
function cidOf(sel){ var el = document.querySelector(sel); return el ? parseInt(el.getAttribute("data-phx-cid")||"0",10)||null : null; }

// ---- event binding ---------------------------------------------------------
// gather collects phx-value-* attributes plus form field values into a payload.
function gather(el){
  var payload = {};
  if (!el) return payload;
  var attrs = el.attributes || [];
  for (var i=0;i<attrs.length;i++){
    var n = attrs[i].name;
    if (n.indexOf("phx-value-") === 0){ payload[n.slice(10)] = attrs[i].value; }
  }
  var form = el.form || (el.tagName === "FORM" ? el : null);
  if (form){ new FormData(form).forEach(function(v,k){ payload[k] = v; }); }
  return payload;
}

function eventFor(el, attr){
  var raw = el.getAttribute(attr);
  if (!raw) return null;
  var t = raw.trim();
  if (t.charAt(0) === "["){ try { return {commands: JSON.parse(t)}; } catch(e){} }
  return {event: t};
}

// rateLimit applies phx-debounce / phx-throttle to fn, keyed by el+kind.
function rateLimit(el, kind, fn){
  var deb = el.getAttribute("phx-debounce"), thr = el.getAttribute("phx-throttle");
  var key = (el.id || el.getAttribute("phx-click") || "") + ":" + kind;
  if (deb){ clearTimeout(throttled[key]); throttled[key] = setTimeout(fn, parseInt(deb,10)||0); return; }
  if (thr){ if (throttled[key]) return; fn(); throttled[key] = setTimeout(function(){ throttled[key]=null; }, parseInt(thr,10)||0); return; }
  fn();
}

function fire(el, attr, extra){
  var spec = eventFor(el, attr);
  if (!spec) return;
  rateLimit(el, attr, function(){
    if (spec.commands){ runCommands(spec.commands, el); return; }
    var payload = gather(el);
    if (extra){ for (var k in extra) payload[k] = extra[k]; }
    var cid = el.getAttribute("data-phx-cid");
    send({type:"event", event: spec.event, cid: cid?parseInt(cid,10):null, payload: payload});
  });
}

function bindEvents(){
  document.addEventListener("click", function(e){
    var t = e.target.closest("[phx-click]"); if (t){ e.preventDefault(); fire(t, "phx-click"); }
  });
  document.addEventListener("input", function(e){
    var t = e.target.closest("[phx-change]"); if (t){ fire(t, "phx-change"); }
  });
  document.addEventListener("submit", function(e){
    var t = e.target.closest("[phx-submit]"); if (t){ e.preventDefault(); fire(t, "phx-submit"); }
  });
  document.addEventListener("keydown", function(e){
    var t = e.target.closest("[phx-keydown]"); if (t){ fire(t, "phx-keydown", {key: e.key}); }
  });
}

// ---- uploads ---------------------------------------------------------------
var CHUNK = 64 * 1024;
function bindUploads(){
  document.querySelectorAll("input[type=file][phx-upload]").forEach(function(inp){
    if (inp.__bound) return; inp.__bound = true;
    inp.addEventListener("change", function(){
      var name = inp.getAttribute("phx-upload");
      Array.prototype.forEach.call(inp.files, function(file, i){
        var ref = String(i);
        send({type:"upload_start", name:name, ref:ref, file_name:file.name, size:file.size, mime:file.type});
        sliceAndSend(name, ref, file, 0);
      });
    });
  });
}
function sliceAndSend(name, ref, file, offset){
  var end = Math.min(offset + CHUNK, file.size);
  var last = end >= file.size;
  var reader = new FileReader();
  reader.onload = function(){
    var b64 = btoa(String.fromCharCode.apply(null, new Uint8Array(reader.result)));
    send({type:"upload_chunk", name:name, ref:ref, data:b64, last:last});
    if (!last){ sliceAndSend(name, ref, file, end); }
  };
  reader.readAsArrayBuffer(file.slice(offset, end));
}

// ---- socket ----------------------------------------------------------------
function send(msg){ if (socket && socket.readyState === 1){ socket.send(JSON.stringify(msg)); } }
function connect(){
  var scheme = location.protocol === "https:" ? "wss:" : "ws:";
  socket = new WebSocket(scheme + "//" + location.host + WS_PATH + location.search);
  socket.onmessage = function(e){
    var msg = JSON.parse(e.data);
    if (msg.type === "mount" || msg.type === "diff"){ applyDiff(msg.diff || {}); }
  };
  socket.onclose = function(){ setTimeout(connect, 1000); };
}

window.addEventListener("popstate", function(){ send({type:"patch", uri: location.href}); });
bindEvents();
connect();
})();`
}

// jsString encodes s as a JavaScript string literal (a double-quoted, escaped
// token) for safe interpolation into the client source.
func jsString(s string) string {
	var b []byte
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '\\':
			b = append(b, '\\', c)
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '<':
			b = append(b, '\\', 'x', '3', 'c')
		default:
			b = append(b, c)
		}
	}
	b = append(b, '"')
	return string(b)
}
