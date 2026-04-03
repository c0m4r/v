"use strict";

const API = "/api";
let consoleWS = null;
let consoleTerm = null;
let consoleFitAddon = null;
let consoleVMId = null;
let consoleVMName = null;
let consoleReconnectTimer = null;
let consoleManualClose = false;
let consoleResizeObserver = null;
let refreshTimer = null;

// --- API helpers ---

async function api(method, path, body) {
  const opts = { method, headers: {} };
  if (body) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const resp = await fetch(API + path, opts);
  const data = await resp.json();
  if (!resp.ok) throw new Error(data.error || resp.statusText);
  return data;
}

// --- VM List ---

async function loadVMs() {
  try {
    const vms = await api("GET", "/vms");
    const tbody = document.getElementById("vm-list");
    const noVMs = document.getElementById("no-vms");
    const table = document.getElementById("vm-table");

    if (!vms || vms.length === 0) {
      table.hidden = true;
      noVMs.hidden = false;
      return;
    }

    table.hidden = false;
    noVMs.hidden = true;
    tbody.innerHTML = vms.map(vm => `
      <tr>
        <td><strong>${esc(vm.name)}</strong></td>
        <td><code>${esc(vm.id)}</code></td>
        <td>${vm.cpus}</td>
        <td>${vm.memory_mb} MB</td>
        <td>${esc(vm.disk_size)}</td>
        <td>${esc(vm.net_mode)}</td>
        <td>${vm.ip ? esc(vm.ip) : (vm.ssh_port ? 'localhost:' + vm.ssh_port : '-')}</td>
        <td class="state-${vm.state}">${vm.state}</td>
        <td class="actions">${vmActions(vm)}</td>
      </tr>
    `).join("");
  } catch (err) {
    console.error("Failed to load VMs:", err);
  }
}

function vmActions(vm) {
  const btns = [];
  if (vm.state === "stopped") {
    btns.push(actionBtn(vm.id, "start", "Start", "btn-primary"));
    btns.push(actionBtn(vm.id, "delete", "Delete", "btn-danger"));
  } else {
    btns.push(actionBtn(vm.id, "stop", "Stop", ""));
    btns.push(actionBtn(vm.id, "force-stop", "Force Stop", ""));
    btns.push(actionBtn(vm.id, "restart", "Restart", ""));
    btns.push(`<button class="btn btn-small" onclick="openConsole('${vm.id}','${esc(vm.name)}')">Console</button>`);
  }
  return btns.join("");
}

function actionBtn(id, action, label, extraClass) {
  return `<button class="btn btn-small ${extraClass}" onclick="vmAction(this,'${id}','${action}')">${label}</button>`;
}

async function vmAction(btn, id, action) {
  if (action === "delete") {
    if (!confirm("Delete this VM? This cannot be undone.")) return;
  }

  // Set loading state on the clicked button
  btn.classList.add("btn-loading");
  const origText = btn.textContent;
  btn.textContent = origText;

  try {
    if (action === "delete") {
      await api("DELETE", `/vms/${id}`);
    } else {
      await api("POST", `/vms/${id}/${action}`);
    }
    await loadVMs();
  } catch (err) {
    alert("Error: " + err.message);
    btn.classList.remove("btn-loading");
  }
}

// --- Settings ---

document.getElementById("btn-settings").addEventListener("click", async () => {
  try {
    const cfg = await api("GET", "/config");
    document.getElementById("settings-ssh-key").value = cfg.default_ssh_key || "";
  } catch (err) {
    console.error("Failed to load config:", err);
  }
  document.getElementById("settings-dialog").showModal();
});

document.getElementById("settings-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = new FormData(e.target);
  try {
    await api("PUT", "/config", {
      default_ssh_key: form.get("default_ssh_key") || "",
    });
    document.getElementById("settings-dialog").close();
  } catch (err) {
    alert("Error: " + err.message);
  }
});

// --- Create VM ---

async function loadImages() {
  try {
    const data = await api("GET", "/images");
    const select = document.getElementById("image-select");
    select.innerHTML = "";

    if (data.cached && data.cached.length > 0) {
      const group = document.createElement("optgroup");
      group.label = "Cached";
      for (const img of data.cached) {
        const opt = document.createElement("option");
        opt.value = img.Name;
        opt.textContent = img.Name;
        group.appendChild(opt);
      }
      select.appendChild(group);
    }

    if (data.available) {
      const group = document.createElement("optgroup");
      group.label = "Available (will download)";
      for (const [name] of Object.entries(data.available)) {
        const opt = document.createElement("option");
        opt.value = name;
        opt.textContent = name;
        group.appendChild(opt);
      }
      select.appendChild(group);
    }
  } catch (err) {
    console.error("Failed to load images:", err);
  }
}

document.getElementById("btn-create").addEventListener("click", async () => {
  loadImages();
  // Show default SSH key status
  const ta = document.querySelector('#create-form [name="ssh_key"]');
  const label = ta.closest("label");
  try {
    const cfg = await api("GET", "/config");
    if (cfg.default_ssh_key) {
      const short = cfg.default_ssh_key.length > 50
        ? cfg.default_ssh_key.slice(0, 47) + "..."
        : cfg.default_ssh_key;
      ta.placeholder = short;
      label.childNodes[0].textContent = "SSH Public Key (using default) ";
    } else {
      ta.placeholder = "ssh-ed25519 AAAA... user@host";
      label.childNodes[0].textContent = "SSH Public Key (no default set) ";
    }
  } catch (err) {
    // ignore
  }
  document.getElementById("create-dialog").showModal();
});

document.getElementById("create-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = new FormData(e.target);
  const imageName = form.get("image");
  let image = imageName;

  const btn = e.target.querySelector('[type="submit"]');
  btn.classList.add("btn-loading");

  try {
    const imgData = await api("GET", "/images");
    const isCached = imgData.cached && imgData.cached.some(i => i.Name === imageName);

    if (!isCached && imgData.available && imgData.available[imageName]) {
      btn.textContent = "Downloading image...";
      const result = await api("POST", "/images/pull", { name: imageName });
      btn.textContent = "Create";
      image = result.path.split("/").pop();
    }

    await api("POST", "/vms", {
      Name: form.get("name"),
      CPUs: parseInt(form.get("cpus")),
      MemoryMB: parseInt(form.get("memory_mb")),
      DiskSize: form.get("disk_size"),
      Image: image,
      NetMode: form.get("net_mode"),
      SSHKey: form.get("ssh_key") || "",
    });

    document.getElementById("create-dialog").close();
    e.target.reset();
    await loadVMs();
  } catch (err) {
    alert("Error: " + err.message);
  } finally {
    btn.classList.remove("btn-loading");
    btn.textContent = "Create";
  }
});

// --- Console ---

function setConsoleStatus(text, cls) {
  const el = document.getElementById("console-status");
  el.textContent = text;
  el.className = "console-status" + (cls ? " " + cls : "");
}

function connectConsole() {
  if (consoleReconnectTimer) {
    clearInterval(consoleReconnectTimer);
    consoleReconnectTimer = null;
  }
  if (consoleWS) {
    consoleWS.onclose = null;
    consoleWS.close();
    consoleWS = null;
  }

  setConsoleStatus("Connecting...", "reconnecting");

  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  consoleWS = new WebSocket(`${proto}//${location.host}/api/vms/${consoleVMId}/console`);
  consoleWS.binaryType = "arraybuffer";

  consoleWS.onopen = () => {
    setConsoleStatus("Connected", "connected");
    if (consoleTerm) consoleTerm.writeln("\r\nConnected.");
  };

  consoleWS.onmessage = (e) => {
    if (!consoleTerm) return;
    consoleTerm.write(new Uint8Array(e.data));
  };

  consoleWS.onclose = () => {
    if (consoleManualClose) return;
    if (consoleTerm) consoleTerm.writeln("\r\nDisconnected.");
    scheduleReconnect();
  };

  consoleWS.onerror = () => {};
}

function scheduleReconnect() {
  if (consoleManualClose) return;
  let countdown = 5;
  setConsoleStatus(`Reconnecting in ${countdown}s...`, "reconnecting");
  consoleReconnectTimer = setInterval(() => {
    countdown--;
    if (countdown <= 0) {
      clearInterval(consoleReconnectTimer);
      consoleReconnectTimer = null;
      connectConsole();
    } else {
      setConsoleStatus(`Reconnecting in ${countdown}s...`, "reconnecting");
    }
  }, 1000);
}

function openConsole(id, name) {
  const overlay = document.getElementById("console-overlay");

  // Clean up previous session
  consoleManualClose = false;
  if (consoleReconnectTimer) { clearInterval(consoleReconnectTimer); consoleReconnectTimer = null; }
  if (consoleResizeObserver) { consoleResizeObserver.disconnect(); consoleResizeObserver = null; }
  if (consoleTerm) { consoleTerm.dispose(); consoleTerm = null; }
  if (consoleWS) { consoleWS.onclose = null; consoleWS.close(); consoleWS = null; }

  consoleVMId = id;
  consoleVMName = name;
  document.getElementById("console-title").textContent = `Console: ${name}`;

  overlay.classList.remove("fullscreen");
  document.getElementById("btn-fullscreen").textContent = "Fullscreen";
  overlay.hidden = false;

  consoleTerm = new Terminal({
    cursorBlink: true,
    theme: { background: "#0d1117", foreground: "#c9d1d9" },
  });
  consoleFitAddon = new FitAddon.FitAddon();
  consoleTerm.loadAddon(consoleFitAddon);
  consoleTerm.open(document.getElementById("terminal"));

  consoleTerm.onData((data) => {
    if (consoleWS && consoleWS.readyState === WebSocket.OPEN) {
      consoleWS.send(new TextEncoder().encode(data));
    }
  });

  // Auto-fit on any resize (drag handle or fullscreen toggle)
  consoleResizeObserver = new ResizeObserver(() => {
    if (consoleFitAddon) consoleFitAddon.fit();
  });
  consoleResizeObserver.observe(document.getElementById("terminal"));

  requestAnimationFrame(() => { consoleFitAddon.fit(); connectConsole(); });
}

function toggleFullscreen() {
  const overlay = document.getElementById("console-overlay");
  const panel = document.getElementById("console-panel");
  const btn = document.getElementById("btn-fullscreen");
  // Clear inline size from resize drag so CSS classes take effect
  panel.style.width = "";
  panel.style.height = "";
  overlay.classList.toggle("fullscreen");
  btn.textContent = overlay.classList.contains("fullscreen") ? "Exit Fullscreen" : "Fullscreen";
}

function closeConsole() {
  consoleManualClose = true;
  if (consoleReconnectTimer) { clearInterval(consoleReconnectTimer); consoleReconnectTimer = null; }
  if (consoleResizeObserver) { consoleResizeObserver.disconnect(); consoleResizeObserver = null; }

  const ws = consoleWS;
  const term = consoleTerm;
  consoleWS = null;
  consoleTerm = null;
  consoleFitAddon = null;
  consoleVMId = null;
  consoleVMName = null;

  if (ws) { ws.onclose = null; ws.close(); }
  if (term) term.dispose();

  const overlay = document.getElementById("console-overlay");
  overlay.classList.remove("fullscreen");
  overlay.hidden = true;
}

// --- Utils ---

function esc(s) {
  const d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

// --- Init ---

loadVMs();
refreshTimer = setInterval(loadVMs, 5000);
