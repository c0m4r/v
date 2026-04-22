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
let serverIsRoot = false;

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

// Passwords are kept out of the DOM to avoid single-quote breakout in onclick attrs.
const vmPasswords = {};

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

    // Refresh password cache
    for (const vm of vms) vmPasswords[vm.id] = vm.root_password || "";

    table.hidden = false;
    noVMs.hidden = true;
    tbody.innerHTML = vms.map(vm => `
      <tr>
        <td><strong>${esc(vm.name)}</strong></td>
        <td><code>${esc(vm.id)}</code></td>
        <td>${esc(vm.cpus)}</td>
        <td>${esc(vm.memory_mb)} MB</td>
        <td>${esc(vm.disk_size)}</td>
        <td>${esc(vm.net_mode)}</td>
        <td>${vm.ip ? esc(vm.ip) : (vm.ssh_port ? esc('localhost:' + vm.ssh_port) : '-')}</td>
        <td class="state-${esc(vm.state)}">${esc(vm.state)}</td>
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
    btns.push(`<button class="btn btn-small" onclick="openVMSettings('${vm.id}','${esc(vm.name)}')">Settings</button>`);
    btns.push(actionBtn(vm.id, "delete", "Delete", "btn-danger"));
  } else {
    btns.push(actionBtn(vm.id, "stop", "Stop", ""));
    btns.push(actionBtn(vm.id, "force-stop", "Force Stop", ""));
    btns.push(actionBtn(vm.id, "restart", "Restart", ""));
    btns.push(`<button class="btn btn-small" onclick="openConsole('${vm.id}','${esc(vm.name)}')">Console</button>`);
  }
  btns.push(`<button class="btn btn-small" onclick="openPasswordDialog('${vm.id}','${esc(vm.name)}')">Password</button>`);
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

function applyBridgeAvailability() {
  const opt = document.querySelector('#create-form [name="net_mode"] option[value="bridge"]');
  if (!opt) return;
  if (serverIsRoot) {
    opt.disabled = false;
    opt.textContent = "Bridge (requires root)";
  } else {
    opt.disabled = true;
    opt.textContent = "Bridge (requires root — restart v with sudo)";
  }
}

const PASSWORD_CHARS = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789";

function generatePassword() {
  const arr = new Uint8Array(16);
  crypto.getRandomValues(arr);
  return Array.from(arr, b => PASSWORD_CHARS[b % PASSWORD_CHARS.length]).join("");
}

function setCreatePasswordAuto() {
  const input = document.getElementById("create-pw-input");
  const toggle = document.getElementById("create-pw-toggle");
  input.type = "text";
  input.value = generatePassword();
  toggle.hidden = true;
  toggle.textContent = "Show";
}

function toggleCreatePassword() {
  const input = document.getElementById("create-pw-input");
  const toggle = document.getElementById("create-pw-toggle");
  if (input.type === "password") {
    input.type = "text";
    toggle.textContent = "Hide";
  } else {
    input.type = "password";
    toggle.textContent = "Show";
  }
}

function regenerateCreatePassword() {
  const pwInput = document.getElementById("create-pw-input");
  if (!pwInput.disabled) setCreatePasswordAuto();
}

document.getElementById("create-pw-input").addEventListener("input", () => {
  const input = document.getElementById("create-pw-input");
  const toggle = document.getElementById("create-pw-toggle");
  if (input.type === "text") {
    input.type = "password";
    toggle.hidden = false;
    toggle.textContent = "Show";
  }
});

document.getElementById("no-password-check").addEventListener("change", (e) => {
  const pwInput = document.getElementById("create-pw-input");
  pwInput.disabled = e.target.checked;
  if (!e.target.checked) setCreatePasswordAuto();
});

document.getElementById("btn-create").addEventListener("click", async () => {
  // Reset and auto-generate password
  const noPassCheck = document.getElementById("no-password-check");
  noPassCheck.checked = false;
  const pwInput = document.getElementById("create-pw-input");
  pwInput.disabled = false;
  pwInput.value = generatePassword();

  applyBridgeAvailability();
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

// Show/hide PCI addr field based on GPU selection in create dialog
document.getElementById("create-gpu-select").addEventListener("change", (e) => {
  document.getElementById("create-pci-label").hidden = e.target.value !== "passthrough";
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

    const noPass = document.getElementById("no-password-check").checked;
    const rootPassword = noPass ? "none" : (document.getElementById("create-pw-input").value || "");

    await api("POST", "/vms", {
      Name: form.get("name"),
      CPUs: parseInt(form.get("cpus")),
      MemoryMB: parseInt(form.get("memory_mb")),
      DiskSize: form.get("disk_size"),
      Image: image,
      NetMode: form.get("net_mode"),
      GPU: form.get("gpu") || "none",
      PCIAddr: form.get("pci_addr") || "",
      Audio: form.get("audio") || "none",
      SSHKey: form.get("ssh_key") || "",
      RootPassword: rootPassword,
    });

    document.getElementById("create-dialog").close();
    e.target.reset();
    document.getElementById("no-password-check").checked = false;
    document.getElementById("create-pw-input").disabled = false;
    document.getElementById("create-pci-label").hidden = true;

    await loadVMs();
  } catch (err) {
    alert("Error: " + err.message);
  } finally {
    btn.classList.remove("btn-loading");
    btn.textContent = "Create";
  }
});

// --- VM Settings Dialog ---

let vmSettingsId = null;

async function openVMSettings(id, name) {
  vmSettingsId = id;
  document.getElementById("vm-settings-title").textContent = `Settings — ${name}`;

  try {
    const vm = await api("GET", `/vms/${id}`);
    const form = document.getElementById("vm-settings-form");
    form.elements["cpus"].value = vm.cpus;
    form.elements["memory_mb"].value = vm.memory_mb;
    const gpu = vm.gpu || "none";
    form.elements["gpu"].value = gpu;
    form.elements["pci_addr"].value = vm.pci_addr || "";
    document.getElementById("settings-pci-label").hidden = gpu !== "passthrough";
    form.elements["audio"].value = vm.audio || "none";
    form.elements["boot_dev"].value = vm.boot_dev || "disk";
  } catch (err) {
    alert("Error loading VM: " + err.message);
    return;
  }

  document.getElementById("vm-settings-dialog").showModal();
}

// Show/hide PCI addr field based on GPU selection in settings dialog
document.getElementById("settings-gpu-select").addEventListener("change", (e) => {
  document.getElementById("settings-pci-label").hidden = e.target.value !== "passthrough";
});

document.getElementById("vm-settings-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = new FormData(e.target);
  const btn = e.target.querySelector('[type="submit"]');
  btn.classList.add("btn-loading");

  try {
    await api("PUT", `/vms/${vmSettingsId}`, {
      CPUs: parseInt(form.get("cpus")),
      MemoryMB: parseInt(form.get("memory_mb")),
      GPU: form.get("gpu"),
      PCIAddr: form.get("pci_addr") || "",
      Audio: form.get("audio"),
      BootDev: form.get("boot_dev"),
    });
    document.getElementById("vm-settings-dialog").close();
    await loadVMs();
  } catch (err) {
    alert("Error: " + err.message);
  } finally {
    btn.classList.remove("btn-loading");
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

// --- Password Dialog ---

let pwDialogVMId = null;

function openPasswordDialog(id, name) {
  pwDialogVMId = id;
  document.getElementById("pw-dialog-title").textContent = `Root Password — ${name}`;
  const display = document.getElementById("pw-display");
  display.value = vmPasswords[id] || "";
  display.type = "password";
  document.getElementById("pw-toggle-btn").textContent = "Show";
  document.getElementById("password-dialog").showModal();
}

function togglePwVisibility() {
  const display = document.getElementById("pw-display");
  const btn = document.getElementById("pw-toggle-btn");
  if (display.type === "password") {
    display.type = "text";
    btn.textContent = "Hide";
  } else {
    display.type = "password";
    btn.textContent = "Show";
  }
}

async function copyStoredPassword() {
  const val = document.getElementById("pw-display").value;
  if (val) await navigator.clipboard.writeText(val);
}


// --- Utils ---

function esc(s) {
  const d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

// --- Init ---

(async () => {
  try {
    const info = await api("GET", "/info");
    serverIsRoot = !!info.is_root;
  } catch (err) {
    serverIsRoot = false;
  }
})();

loadVMs();
refreshTimer = setInterval(loadVMs, 5000);
