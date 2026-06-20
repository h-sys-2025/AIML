package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type webSession struct {
	interp   *Interpreter
	registry *ToolRegistry
	mu       sync.Mutex
}

var (
	webSesh *webSession
)

type toolJSON struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Args        []ArgDef `json:"args"`
	Examples    []string `json:"examples"`
}

func StartWebServer(addr string, registry *ToolRegistry, client *OllamaClient, systemPrompt string, noThinkMode bool) error {
	webSesh = &webSession{
		interp:   NewInterpreter(registry, client, false, 20, false),
		registry: registry,
	}
	webSesh.interp.SetShowThink(!noThinkMode)
	if noThinkMode {
		client.SetThink(false)
	}

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		handleChat(w, r, systemPrompt)
	})
	http.HandleFunc("/api/tools", handleTools)

	fmt.Printf("  Web UI: http://%s\n", addr)
	return http.ListenAndServe(addr, nil)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(webHTML))
}

func handleTools(w http.ResponseWriter, r *http.Request) {
	tools := webSesh.registry.List()
	jt := make([]toolJSON, 0, len(tools))
	for _, t := range tools {
		jt = append(jt, toolJSON{
			Name:        t.Name,
			Description: t.Description,
			Args:        t.Args,
			Examples:    t.Examples,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jt)
}

func handleChat(w http.ResponseWriter, r *http.Request, systemPrompt string) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}

	msg := strings.TrimSpace(r.FormValue("message"))
	if msg == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "empty message"})
		return
	}

	if msg == "/clear" {
		webSesh.mu.Lock()
		webSesh.interp.Clear()
		webSesh.mu.Unlock()
		json.NewEncoder(w).Encode([]OutputEvent{{Type: EventAnswer, Content: "Context cleared."}})
		return
	}

	var events []OutputEvent
	cb := func(ev OutputEvent) {
		if ev.Type != EventToken {
			events = append(events, ev)
		}
	}

	webSesh.mu.Lock()
	webSesh.interp.SetEventCb(cb)
	webSesh.interp.Run(systemPrompt, msg)
	webSesh.interp.SetEventCb(nil)
	webSesh.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

var webHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>AIML Agent</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: system-ui, -apple-system, sans-serif;
  background: #f5f5f5;
  color: #222;
  height: 100vh;
  display: flex;
  overflow: hidden;
}

/* ---- Sidebar ---- */
#sidebar {
  width: 260px;
  min-width: 260px;
  background: #2c2c2c;
  color: #ddd;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  transition: width 0.15s, min-width 0.15s;
}
#sidebar.collapsed {
  width: 0;
  min-width: 0;
}
#sidebar-header {
  padding: 14px 16px 8px;
  font-size: 0.85em;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: #999;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
#toggle-sidebar {
  background: none;
  border: none;
  color: #999;
  font-size: 1.1em;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 4px;
}
#toggle-sidebar:hover { color: #fff; background: #444; }
#tools-list {
  flex: 1;
  overflow-y: auto;
  padding: 4px 8px;
}
.tool-item {
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.85em;
  line-height: 1.3;
  transition: background 0.1s;
}
.tool-item:hover { background: #3a3a3a; }
.tool-item .tool-name { font-weight: 600; color: #5bf; }
.tool-item .tool-desc { color: #999; font-size: 0.9em; margin-top: 2px; }
.tool-item:active { background: #444; }
#sidebar-footer {
  padding: 8px 16px 12px;
  font-size: 0.75em;
  color: #666;
  text-align: center;
}

/* ---- Tool detail popup ---- */
.tool-popup-overlay {
  display: none;
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.4);
  z-index: 100;
  justify-content: center;
  align-items: center;
}
.tool-popup-overlay.open { display: flex; }
.tool-popup {
  background: #fff;
  border-radius: 10px;
  max-width: 480px;
  width: 90%;
  max-height: 80vh;
  overflow-y: auto;
  padding: 20px;
  box-shadow: 0 8px 30px rgba(0,0,0,0.2);
}
.tool-popup h2 { font-size: 1.1em; margin-bottom: 6px; }
.tool-popup .desc { color: #555; font-size: 0.9em; margin-bottom: 12px; }
.tool-popup .args-table { width: 100%; border-collapse: collapse; font-size: 0.85em; margin-bottom: 12px; }
.tool-popup .args-table th, .tool-popup .args-table td { text-align: left; padding: 4px 8px; border-bottom: 1px solid #eee; }
.tool-popup .args-table th { color: #888; font-weight: 600; }
.tool-popup .req { color: #e55; }
.tool-popup .examples { background: #f5f5f5; border-radius: 6px; padding: 10px; font-family: monospace; font-size: 0.82em; white-space: pre-wrap; word-break: break-all; margin-bottom: 12px; }
.tool-popup .btn-row { display: flex; gap: 8px; justify-content: flex-end; }
.tool-popup .btn-row button {
  padding: 8px 16px;
  border: none;
  border-radius: 6px;
  font-size: 0.9em;
  cursor: pointer;
}
.tool-popup .btn-use {
  background: #007aff;
  color: #fff;
}
.tool-popup .btn-use:active { background: #005bbf; }
.tool-popup .btn-close { background: #e8e8e8; color: #333; }
.tool-popup .btn-close:active { background: #ccc; }

/* ---- Main panel ---- */
#main {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 16px;
  min-width: 0;
}
h1 {
  font-size: 1.2em;
  padding: 0 0 8px;
  border-bottom: 2px solid #333;
  margin-bottom: 8px;
  display: flex;
  align-items: center;
  gap: 8px;
}
h1 #sidebar-toggle {
  background: none;
  border: none;
  font-size: 1em;
  cursor: pointer;
  color: #888;
  padding: 4px 6px;
  border-radius: 4px;
}
h1 #sidebar-toggle:hover { background: #eee; }
#chat {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
  background: #fff;
  border: 1px solid #ddd;
  border-radius: 8px;
  margin-bottom: 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.user-msg {
  align-self: flex-end;
  background: #007aff;
  color: #fff;
  padding: 8px 14px;
  border-radius: 18px 18px 4px 18px;
  max-width: 75%;
  font-size: 0.95em;
}
.event { padding: 6px 10px; border-radius: 6px; }
.event.think {
  color: #888;
  font-style: italic;
  font-size: 0.9em;
  background: #fafafa;
  border-left: 3px solid #ccc;
}
.event.tool-call {
  border: 1px solid #ddd;
  background: #f8f8f8;
  font-family: monospace;
  font-size: 0.85em;
}
.event.tool-call .tool-header {
  font-weight: 600;
  color: #2a7;
  margin-bottom: 4px;
}
.event.tool-call .tool-attrs {
  color: #666;
  font-size: 0.9em;
  margin-bottom: 4px;
}
.event.tool-call .tool-attrs span { color: #07a; }
.event.tool-output {
  border: 1px solid #ddd;
  background: #f0f8f0;
  font-family: monospace;
  font-size: 0.85em;
  white-space: pre-wrap;
  word-break: break-word;
}
.event.tool-error {
  border: 1px solid #e88;
  background: #fff0f0;
  color: #c33;
  font-family: monospace;
  font-size: 0.85em;
}
.event.answer {
  font-weight: 700;
  color: #000;
  font-size: 1em;
  line-height: 1.5;
  white-space: pre-wrap;
}
.event.response {
  color: #000;
  font-size: 1em;
  line-height: 1.5;
  white-space: pre-wrap;
}
.event.stats {
  font-size: 0.78em;
  color: #999;
  text-align: center;
}
.event.error {
  color: #c33;
  font-weight: 600;
}
.event.turn {
  font-size: 0.78em;
  color: #999;
  text-align: center;
  border-top: 1px solid #eee;
  padding-top: 12px;
  margin-top: 4px;
}
.event.feedback { display: none; }
#input-row {
  display: flex;
  gap: 8px;
}
#input {
  flex: 1;
  padding: 10px 14px;
  border: 2px solid #ddd;
  border-radius: 20px;
  font-size: 0.95em;
  outline: none;
}
#input:focus { border-color: #007aff; }
#send {
  padding: 10px 20px;
  background: #007aff;
  color: #fff;
  border: none;
  border-radius: 20px;
  font-size: 0.95em;
  cursor: pointer;
}
#send:active { background: #005bbf; }
#send:disabled { background: #aaa; cursor: default; }
.loading { text-align: center; color: #999; font-size: 0.85em; padding: 8px; }

/* ---- Scrollbar ---- */
#tools-list::-webkit-scrollbar, #chat::-webkit-scrollbar, .tool-popup::-webkit-scrollbar { width: 6px; }
#tools-list::-webkit-scrollbar-track, #chat::-webkit-scrollbar-track, .tool-popup::-webkit-scrollbar-track { background: transparent; }
#tools-list::-webkit-scrollbar-thumb { background: #555; border-radius: 3px; }
#chat::-webkit-scrollbar-thumb { background: #ccc; border-radius: 3px; }
</style>
</head>
<body>

<!-- Sidebar -->
<div id="sidebar">
  <div id="sidebar-header">
    <span>Tools</span>
    <button id="toggle-sidebar" title="Close sidebar">&times;</button>
  </div>
  <div id="tools-list"></div>
  <div id="sidebar-footer">click a tool to use it</div>
</div>

<!-- Tool popup -->
<div id="tool-popup-overlay" class="tool-popup-overlay">
  <div class="tool-popup">
    <h2 id="popup-name"></h2>
    <div class="desc" id="popup-desc"></div>
    <table class="args-table">
      <thead><tr><th>Arg</th><th>Type</th><th></th><th>Description</th></tr></thead>
      <tbody id="popup-args"></tbody>
    </table>
    <div class="examples" id="popup-examples"></div>
    <div class="btn-row">
      <button class="btn-use" id="popup-use">Use in context</button>
      <button class="btn-close" id="popup-close">Close</button>
    </div>
  </div>
</div>

<!-- Main panel -->
<div id="main">
  <h1>
    <button id="sidebar-toggle" title="Toggle sidebar">&#9776;</button>
    AIML Agent
  </h1>
  <div id="chat"></div>
  <div id="input-row">
    <input id="input" type="text" placeholder="Type your message..." autofocus>
    <button id="send">Send</button>
  </div>
</div>

<script>
const sidebar = document.getElementById('sidebar');
const toggleSidebar = document.getElementById('toggle-sidebar');
const sidebarToggleBtn = document.getElementById('sidebar-toggle');
const toolsList = document.getElementById('tools-list');
const popupOverlay = document.getElementById('tool-popup-overlay');
const popupName = document.getElementById('popup-name');
const popupDesc = document.getElementById('popup-desc');
const popupArgs = document.getElementById('popup-args');
const popupExamples = document.getElementById('popup-examples');
const popupUse = document.getElementById('popup-use');
const popupClose = document.getElementById('popup-close');
const chat = document.getElementById('chat');
const input = document.getElementById('input');
const send = document.getElementById('send');

let tools = [];

// ---- Sidebar toggle ----
function closeSidebar() { sidebar.classList.add('collapsed'); }
function openSidebar() { sidebar.classList.remove('collapsed'); }
toggleSidebar.addEventListener('click', closeSidebar);
sidebarToggleBtn.addEventListener('click', openSidebar);

// ---- Fetch tools ----
async function loadTools() {
  let res = await fetch('/api/tools');
  tools = await res.json();
  toolsList.innerHTML = '';
  for (let t of tools) {
    let item = document.createElement('div');
    item.className = 'tool-item';
    item.innerHTML = '<div class="tool-name">' + esc(t.name) + '</div><div class="tool-desc">' + esc(t.description) + '</div>';
    item.addEventListener('click', () => showToolPopup(t));
    toolsList.appendChild(item);
  }
}
loadTools();

// ---- Tool popup ----
function showToolPopup(t) {
  popupName.textContent = t.name;
  popupDesc.textContent = t.description;

  popupArgs.innerHTML = '';
  if (t.args && t.args.length) {
    for (let a of t.args) {
      let tr = document.createElement('tr');
      let req = a.required ? ' <span class="req">*</span>' : '';
      tr.innerHTML = '<td>' + esc(a.name) + req + '</td><td>' + esc(a.type) + '</td><td></td><td>' + esc(a.description) + '</td>';
      popupArgs.appendChild(tr);
    }
  } else {
    popupArgs.innerHTML = '<tr><td colspan="4" style="color:#999;font-style:italic;text-align:center;padding:8px;">No arguments</td></tr>';
  }

  let exHtml = '';
  if (t.examples && t.examples.length) {
    for (let ex of t.examples) {
      exHtml += esc(ex) + '\n';
    }
  } else if (t.args && t.args.length) {
    let tag = 'tool:' + t.name;
    let attrs = '';
    for (let a of t.args) {
      if (a.required) attrs += ' ' + a.name + '="<' + a.name + '>"';
    }
    exHtml = '<' + tag + attrs + '></' + tag + '>';
  }
  popupExamples.textContent = exHtml.trim();

  popupUse.onclick = () => {
    insertTool(t);
    closePopup();
  };
  popupOverlay.classList.add('open');
}

function closePopup() { popupOverlay.classList.remove('open'); }
popupClose.addEventListener('click', closePopup);
popupOverlay.addEventListener('click', e => { if (e.target === popupOverlay) closePopup(); });

function insertTool(t) {
  let tag = 'tool:' + t.name;
  let snippet = '<' + tag;
  if (t.args && t.args.length) {
    let hasBody = t.args.some(a => a.name === 'content' || a.name === 'command' || a.name === 'query' || a.name === 'location' || a.name === 'url' || a.name === 'prompt');
    for (let a of t.args) {
      if (a.required && (a.name === 'content' || a.name === 'command' || a.name === 'query' || a.name === 'location' || a.name === 'url' || a.name === 'prompt')) {
        snippet += ' ' + a.name + '="';
        if (hasBody && a.name === 'content') {
          snippet += '"';
        } else {
          snippet += '"';
        }
        hasBody = true;
      }
    }
    snippet += '>';
    if (hasBody) {
      // For tools with content body, put cursor between tags
      let cursorPos = input.selectionStart;
      snippet += '</' + tag + '>';
      input.setRangeText(snippet, cursorPos, cursorPos, 'end');
      // Place cursor at content position for body-style tools
      let bodyPos = snippet.indexOf('>') + 1;
      if (bodyPos < snippet.indexOf('</')) {
        input.selectionStart = cursorPos + bodyPos;
        input.selectionEnd = cursorPos + bodyPos;
      }
    } else {
      snippet += '';
    }
  } else {
    snippet += '></' + tag + '>';
  }

  let cursorPos = input.selectionStart;
  input.setRangeText(snippet, cursorPos, cursorPos, 'end');

  // For simple tools, try to set cursor at right position
  let closePos = snippet.indexOf('</');
  if (closePos > 0) {
    let bodyStart = snippet.indexOf('>') + 1;
    if (bodyStart < closePos) {
      input.selectionStart = cursorPos + bodyStart;
      input.selectionEnd = cursorPos + bodyStart;
    }
  }

  input.focus();
}

// ---- Chat events ----
function addEvent(ev) {
  let el;
  switch (ev.type) {
    case 'think':
      el = div('event think');
      el.textContent = ev.content;
      break;
    case 'tool_call':
      el = div('event tool-call');
      let h = span('tool-header', '\u25B6 ' + ev.toolName);
      el.appendChild(h);
      if (ev.attrs && Object.keys(ev.attrs).length) {
        let a = span('tool-attrs', '');
        for (let [k, v] of Object.entries(ev.attrs)) {
          a.innerHTML += '<span>' + esc(k) + '</span>=' + esc(v) + ' ';
        }
        el.appendChild(a);
      }
      if (ev.content) {
        el.appendChild(document.createTextNode(ev.content));
      }
      break;
    case 'tool_output':
      el = div('event tool-output');
      el.textContent = ev.content || '';
      break;
    case 'tool_error':
      el = div('event tool-error');
      el.textContent = '\u2716 ' + (ev.content || '');
      break;
    case 'answer':
      el = div('event answer');
      el.textContent = ev.content || '';
      break;
    case 'response':
      el = div('event response');
      el.textContent = ev.content || '';
      break;
    case 'stats':
      el = div('event stats');
      let s = '';
      if (ev.tokPerSec) s += ev.tokPerSec.toFixed(1) + ' tok/s';
      if (ev.evalCount) s += '  \u2022  ' + ev.evalCount + ' tokens';
      if (ev.duration) s += '  \u2022  ' + ev.duration;
      el.textContent = s || '';
      break;
    case 'turn':
      el = div('event turn');
      el.textContent = ev.content === 'end' ? '' : '\u25B6 Turn ' + ev.turn + '/' + ev.maxTurns;
      break;
    case 'error':
      el = div('event error');
      el.textContent = '\u274C ' + (ev.content || '');
      break;
    default:
      return;
  }
  chat.appendChild(el);
  chat.scrollTop = chat.scrollHeight;
}

function div(cls) { let d = document.createElement('div'); d.className = cls; return d; }
function span(cls, text) { let s = document.createElement('span'); s.className = cls; s.textContent = text; return s; }
function esc(s) { return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'); }

async function sendMsg() {
  let msg = input.value.trim();
  if (!msg) return;
  input.value = '';
  send.disabled = true;

  let userEl = document.createElement('div');
  userEl.className = 'user-msg';
  userEl.textContent = msg;
  chat.appendChild(userEl);
  chat.scrollTop = chat.scrollHeight;

  let loading = document.createElement('div');
  loading.className = 'loading';
  loading.textContent = 'Thinking...';
  chat.appendChild(loading);

  try {
    let form = new FormData();
    form.append('message', msg);
    let res = await fetch('/api/chat', { method: 'POST', body: form });
    let events = await res.json();
    chat.removeChild(loading);
    for (let ev of events) addEvent(ev);
  } catch (e) {
    chat.removeChild(loading);
    let errEl = document.createElement('div');
    errEl.className = 'event error';
    errEl.textContent = '\u274C Request failed: ' + e.message;
    chat.appendChild(errEl);
  }
  send.disabled = false;
  input.focus();
}

send.addEventListener('click', sendMsg);
input.addEventListener('keydown', e => { if (e.key === 'Enter') sendMsg(); });
</script>
</body>
</html>`
