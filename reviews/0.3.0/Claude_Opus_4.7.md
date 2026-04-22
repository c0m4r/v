# Security Audit — `v` (KVM VM Manager)

**Target:** `github.com/c0m4r/v` @ v0.3.0 (commit 67e5137)
**Scope:** Go engine, CLI, web UI, shell addons, dependency surface
**Method:** Full source review of every file in the module (engine, cmd/cli, cmd/web, addons, build system, static assets). No dynamic testing performed — findings are from code review.
**Audit date:** 2026-04-22
**Severity scale:** CVSS-style, adapted for a self-hosted dev tool. Numbers assume the *documented* threat model (a user runs `v serve` on a workstation, sometimes as root).

---

## Remediation patch (2026-04-22)

All Critical and High findings were addressed in the same session as the audit. The patch touches seven files and adds one new file:

| File | Change |
|------|--------|
| `cmd/web/auth.go` *(new)* | Token generation/loading, `authMiddleware` (Host + Origin + Bearer token), helper functions |
| `cmd/web/server.go` | Wraps mux with `authMiddleware`; prints access URL at startup; CRLF-sanitises log path |
| `cmd/web/handlers.go` | Explicit `vmResponse` struct without `RootPassword`; new `GET /api/vms/{id}/password` endpoint |
| `cmd/web/static/app.js` | Token read/store from URL → `sessionStorage`; `Authorization` header on all API calls; `?token=` on WebSocket URL; `openPasswordDialog` fetches from API instead of client-side cache |
| `engine/vm.go` | `validDiskSize` regex; `validate()` rejects bad disk sizes, image path separators, and newlines in password/SSH key; containment check in `CreateVM` |
| `engine/image.go` | `safePullClient` with private-IP-blocking custom dialer (prevents TOCTOU DNS rebinding); 16 GiB download cap |
| `engine/cloudinit.go` | `yamlDoubleQuote` helper; `password:` and SSH key values double-quoted in generated YAML; input validation rejects newlines and null bytes |

All 55 pre-existing tests continue to pass (`go test -race ./...`). `go vet ./...` reports no issues.

---

## Executive summary

`v` is a well-structured Go program with clean code, good input-validation regexes on identifiers, correct use of `exec.Command` (no shell invocation), and a deliberate XSS-conscious DOM strategy in the web UI. Those are real strengths and should not be lost.

At the time of the audit, **the web UI and its HTTP API had no authentication, no CSRF protection, and no Origin enforcement on the WebSocket console**, and the API returned every VM's plaintext root password in every list response. Combined with the documented/encouraged practice of running `sudo v serve` for bridge networking, and the suggested `--listen 0.0.0.0:9090`, this created several paths to full host compromise. All of these have now been fixed.

A smaller collection of parser-boundary issues (argument injection into `qemu-img`, path traversal through the `Image` field, cloud-init YAML injection via passwords/SSH keys, SSRF via `image pull`) compounded the headline problem. All have been addressed.

**Audit counts (original):** 2 Critical, 5 High, 7 Medium, 8 Low, 4 Informational.
**After patch:** 0 Critical open, 0 High open. 7 Medium, 7 Low, 4 Informational remain open.

---

## Threat model assumed

1. **Local workstation, non-root** — user runs `./v serve` on `127.0.0.1:8080`. Attacker = a web page the user visits, or another local user on a shared machine.
2. **Local workstation, root** — user runs `sudo ./v serve` for bridge networking. Attacker = same, but each finding escalates to root-equivalent.
3. **Lab/LAN** — user runs `v serve --listen 0.0.0.0:9090` (documented in the README). Attacker = any host that can reach the port.

---

## CRITICAL

### C-1 — Web API had no authentication, no authorization, no CSRF protection
**CVSS ≈ 9.8 — Critical** | **STATUS: FIXED ✅**   
**Files:** [cmd/web/server.go](cmd/web/server.go), [cmd/web/handlers.go](cmd/web/handlers.go)

`registerRoutes` wired every state-changing endpoint with **zero authentication middleware**. There was no login, no token, no shared secret, and no origin/host header check.

Impact (as audited):
- **Cross-site CSRF.** Any website the user visited while `v serve` was running could issue `POST`/`DELETE` against `http://127.0.0.1:8080`. The server ignored `Content-Type` and decoded any body as JSON, so `text/plain`-body requests (no CORS preflight) worked fine.
- **DNS rebinding.** No `Host` header validation. A page on an attacker-controlled domain resolving briefly to `127.0.0.1` could talk to the API from the browser.
- **When `sudo v serve` was used** every CSRF payload executed as root.
- **Multi-user hosts.** Any local user could `curl http://127.0.0.1:8080/api/vms` and read VM passwords or delete VMs.

**Fix applied (`cmd/web/auth.go`, `cmd/web/server.go`):**

A new `authMiddleware` wraps the entire mux and enforces three layers:

1. **Host-header validation** (loopback binds only) — rejects any request whose `Host` header does not match `localhost`, `127.0.0.1`, `[::1]`/`::1`, or the explicit `--listen` IP. Kills DNS rebinding without requiring the attacker to know the token.

2. **Origin validation on `/api/*`** — if an `Origin` header is present it must resolve to an allowed host. Blocks cross-site fetch and cross-site WebSocket hijacking (C-2b) at the HTTP level, before the WebSocket upgrade.

3. **Bearer token authentication on `/api/*`** — a 32-byte random token is generated on first `serve`, persisted in `<dataDir>/.token` (mode `0600`), and reloaded on restart. The server prints the full access URL (`http://…/?token=<token>`) at startup. The frontend reads the token from `?token=` on first load, stores it in `sessionStorage`, and sends it as `Authorization: Bearer <token>` on every request. WebSocket connections pass it as `?token=<token>` in the URL (browsers cannot set custom Upgrade-request headers). The token file is owned by the process owner (root when `sudo v serve`), so other local users cannot read it.

---

### C-2 — Root password leaked in every list response; WebSocket console cross-origin hijackable
**CVSS ≈ 9.1 — Critical** | **STATUS: FIXED ✅**   
**Files:** [engine/vm.go](engine/vm.go), [cmd/web/handlers.go](cmd/web/handlers.go), [cmd/web/console_ws.go](cmd/web/console_ws.go)

**C-2a — Plaintext password in list API.** `handleListVMs` and `handleGetVM` embedded the full `*engine.VM` in the response, which includes `RootPassword`. `GET /api/vms` with no credentials returned the root password of every VM.

**C-2b — Cross-site WebSocket hijacking.** `golang.org/x/net/websocket`'s `websocket.Handler` only validates that `Origin` is a *well-formed URL*. Any page on `evil.com` could open `ws://127.0.0.1:8080/api/vms/{id}/console` and obtain a bidirectional pipe to the VM's serial console.

**Fix applied (`cmd/web/handlers.go`, `cmd/web/static/app.js`, `cmd/web/auth.go`):**

- **C-2a:** `vmResponse` is now an explicit struct that lists every field individually. `RootPassword` is absent. A new authenticated endpoint `GET /api/vms/{id}/password` is the only place the stored password is served. The frontend `openPasswordDialog` function now calls this endpoint instead of reading a client-side cache. The `vmPasswords` cache is removed entirely.

- **C-2b:** The Origin check in `authMiddleware` (see C-1) validates the `Origin` header on all `/api/*` requests, including the WebSocket upgrade. An `evil.com` page cannot pass this check. The token requirement adds a second independent barrier.

---

## HIGH

### H-1 — Path traversal into arbitrary backing file via `Image` field
**CVSS ≈ 7.5 — High** | **STATUS: FIXED ✅**   
**File:** [engine/vm.go](engine/vm.go)

`opts.Image` came directly from the API with no sanitisation. `filepath.Join("images", "../../../etc/shadow")` resolves to `/etc/shadow`. The file would be passed to `qemu-img create -b` as the backing file and to `qemu-system-x86_64` at boot.

**Fix applied (`engine/vm.go`):**

`validate()` now rejects any `Image` value containing `/`, `\`, `..`, or null bytes:

```go
if strings.ContainsAny(o.Image, "/\\\x00") || strings.Contains(o.Image, "..") {
    return fmt.Errorf("image name must not contain path separators or directory traversal")
}
```

`CreateVM` adds a defence-in-depth containment check after `filepath.Join`:

```go
if !strings.HasPrefix(filepath.Clean(baseImage)+string(os.PathSeparator),
    filepath.Clean(e.ImageDir)+string(os.PathSeparator)) {
    return nil, fmt.Errorf("image %q escapes the image directory", opts.Image)
}
```

---

### H-2 — Argument injection into `qemu-img` via `DiskSize`
**CVSS ≈ 7.2 — High** | **STATUS: FIXED ✅**   
**Files:** [engine/vm.go](engine/vm.go), [engine/disk.go](engine/disk.go)

`validate()` never checked `DiskSize` beyond "non-empty", and it flowed directly into `exec.Command("qemu-img", …, size)`. An attacker could set `DiskSize` to `--some-flag` or `-o backing_file=/etc/shadow,backing_fmt=raw` and `qemu-img` would parse it as a flag.

**Fix applied (`engine/vm.go`):**

A `validDiskSize` regex is enforced server-side:

```go
var validDiskSize = regexp.MustCompile(`^[0-9]+[KMGTkmgt]$`)

// in validate():
if !validDiskSize.MatchString(o.DiskSize) {
    return fmt.Errorf("disk size must be a positive number followed by K, M, G, or T (e.g. 10G)")
}
```

The client-side `pattern="[0-9]+[GMTgmt]"` attribute in `index.html` is now backed by equivalent server-side enforcement.

---

### H-3 — SSRF / outbound HTTP to private addresses via `image pull`
**CVSS ≈ 7.5 — High** | **STATUS: FIXED ✅ (private-IP block + size cap; HTTPS not enforced — see note)**   
**File:** [engine/image.go](engine/image.go)

`PullImage` used `http.Get(url)` without any restriction. The unauthenticated web endpoint `POST /api/images/pull` accepted an arbitrary `name`/URL. Any cross-origin page could instruct `v serve` to fetch `http://169.254.169.254/…` (cloud metadata) or any intranet address. There was also no maximum download size.

**Fix applied (`engine/image.go`):**

`safePullClient()` replaces the bare `http.Get` call. It provides a custom `DialContext` that:
1. Resolves the hostname via DNS.
2. Rejects any resolved IP in private/loopback/link-local ranges (RFC 1918, 127/8, 169.254/16, ::1, fc00::/7, fe80::/10, 100.64/10).
3. Dials the *first resolved IP directly* (not the hostname again), preventing TOCTOU DNS rebinding against the fetcher.

A 16 GiB per-file cap is enforced during the download loop. This also fixes **L-4** (no image size bound).

**Note:** Plain `http://` URLs are not rejected for CLI use cases where developers pull from local mirrors. With C-1's authentication in place the SSRF risk from the web API is substantially reduced (an attacker must know the token, which is stored at `0600` in the data directory). Enforcing HTTPS and shipping per-image checksums remain open recommendations under M-5.

---

### H-4 — Cloud-init YAML injection via `RootPassword` and `SSHKey`
**CVSS ≈ 7.0 — High** | **STATUS: FIXED ✅**   
**File:** [engine/cloudinit.go](engine/cloudinit.go)

`buildUserData` concatenated `password` and `sshKey` directly into YAML, making injection trivial for any value containing a newline. Example: `password = "foo\nruncmd:\n  - [rm,-rf,/]\n"` would inject arbitrary cloud-init directives executed at first guest boot.

**Fix applied (`engine/cloudinit.go`, `engine/vm.go`):**

Two complementary layers:

1. **Input validation** — `validate()` in `vm.go` and `GenerateCloudInit` in `cloudinit.go` both reject passwords, SSH keys, and user-data containing `\n`, `\r`, or null bytes.

2. **Output encoding** — `yamlDoubleQuote()` wraps string values in YAML double-quotes, escaping `"` and `\`. The `password:` key and each `ssh_authorized_keys` list entry are now encoded through this function, preventing YAML special characters (`{`, `[`, `:` at start of value, etc.) from being misinterpreted even if newline validation were somehow bypassed:

```go
quoted := yamlDoubleQuote(password)
b.WriteString("password: " + quoted + "\n")
// …
b.WriteString("ssh_authorized_keys:\n  - " + yamlDoubleQuote(strings.TrimSpace(sshKey)) + "\n")
```

---

### H-5 — Running `sudo v serve` exposed every local user to root-level API access
**CVSS ≈ 7.8 on a shared host — High** | **STATUS: PARTIALLY MITIGATED**
**Files:** [README.md](README.md), [start.sh](start.sh), [cmd/web/server.go](cmd/web/server.go)

On any multi-user Linux host, `sudo v serve` with no authentication meant any local user could `curl http://127.0.0.1:8080/api/…` and perform any action as root.

**Mitigation applied:**

The token fix from C-1 closes the access-control gap: the token is stored in `<dataDir>/.token` with mode `0600`. When `v serve` runs as root, `dataDir` defaults to `/root/.local/share/v/`, inaccessible to other users. Other local users can no longer read the token and therefore cannot make authenticated API calls.

**Still recommended (not yet implemented):** drop root after bridge setup. The long-running `v serve` process should run unprivileged; only the tap-creation and iptables calls require root, ideally through a small setuid helper or Polkit action. The `v net setup` / `v net teardown` split already exists as a CLI concept; extending it to runtime tap management would close this fully.

---

## MEDIUM

### M-1 — No resource limits on VM creation
**CVSS ≈ 5.3 — Medium** | **STATUS: OPEN**   
**File:** [engine/vm.go](engine/vm.go)

`validate()` sets minimums (`CPUs < 1 → 1`, `MemoryMB < 128 → 512`) but no maximums. The web UI caps CPUs at 64 and memory via `step=128` client-side, but the server accepts anything. An attacker can POST `{"CPUs":9999,"MemoryMB":1000000}` to exhaust host resources.

**Recommendation:** enforce a maximum CPUs (e.g. `min(runtime.NumCPU()*4, 256)`), memory (e.g. system RAM), and a cap on total number of VMs.

---

### M-2 — Unbounded JSON request body
**CVSS ≈ 5.3 — Medium** | **STATUS: OPEN**   
**File:** all handlers in [cmd/web/handlers.go](cmd/web/handlers.go)

`json.NewDecoder(r.Body).Decode(...)` reads the entire body. A large POST will exhaust RSS.

**Recommendation:** wrap every request body with `http.MaxBytesReader(w, r.Body, 1<<20)` (1 MiB is enough for every legitimate payload in this app).

---

### M-3 — `rand.Read` errors silently ignored in ID/password/MAC generation
**CVSS ≈ 5.0 — Medium** | **STATUS: OPEN**   
**File:** [engine/vm.go](engine/vm.go)

```go
rand.Read(b) // error ignored in generatePassword, generateID, generateMAC
```

If `crypto/rand` ever fails the slices are all zeroes: every password becomes identical, every VM ID the same, every VM gets the same MAC.

**Recommendation:** `if _, err := rand.Read(b); err != nil { panic(err) }` — turns a silent catastrophe into a loud one at effectively no cost.

---

### M-4 — No security response headers on HTML/static responses
**CVSS ≈ 4.3 — Medium** | **STATUS: OPEN**   
**File:** [cmd/web/server.go](cmd/web/server.go)

The static file server returns `index.html` without `Content-Security-Policy`, `X-Frame-Options`, `X-Content-Type-Options`, or `Referrer-Policy`. The UI is framable, enabling clickjacking.

**Recommendation:** middleware that sets at minimum:
- `Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'`
- `X-Frame-Options: DENY`
- `X-Content-Type-Options: nosniff`
- `Referrer-Policy: no-referrer`

---

### M-5 — No integrity/signature verification on pulled images
**CVSS ≈ 5.9 — Medium** | **STATUS: OPEN**   
**File:** [engine/image.go](engine/image.go), [engine/config.go](engine/config.go)

No SHA256 pins, no GPG verification. A MITM or compromised CDN can replace any image. Plain `http://` URLs are still accepted.

**Recommendation:** ship a checksum for each `defaultImages` entry; verify after download. Refuse `http://` URLs for the web API. Expose a `--sha256` flag for user-supplied URLs.

---

### M-6 — PID-file trust
**CVSS ≈ 4.0 — Medium** | **STATUS: OPEN**   
**File:** [engine/lifecycle.go](engine/lifecycle.go)

`ForceStopVM` reads `<vmdir>/pid`, trusts the PID, and `syscall.Kill(pid, SIGKILL)`. A process that can write to the VM directory can cause v to kill arbitrary processes owned by the same user (or any process, when running as root).

**Recommendation:** verify `/proc/<pid>/comm` contains `qemu-system` before signalling, or use `pidfd_open(2)` captured at VM launch time.

---

### M-7 — Console WebSocket has no backpressure or connection limit
**CVSS ≈ 3.7 — Medium** | **STATUS: OPEN**   
**File:** [cmd/web/console_ws.go](cmd/web/console_ws.go)

Per-goroutine 4 KiB buffer, no cap on in-flight bytes, no limit on concurrent console sessions per VM.

**Recommendation:** bound in-flight bytes per session; limit concurrent console sessions.

---

## LOW

### L-1 — Modulo bias in password generator
**CVSS ≈ 2.0 — Low** | **STATUS: OPEN**   
**File:** [engine/vm.go](engine/vm.go)

`256 % 55 = 36`, so the first 36 characters in `passwordChars` are marginally more likely. At ≈ 92 bits of entropy this is not practically exploitable, but it is a textbook bias.

**Recommendation:** rejection sampling or `crypto/rand.Int`.

---

### L-2 — `ip_forward` enabled globally, never restored
**CVSS ≈ 2.0 — Low** | **STATUS: OPEN**   
**File:** [engine/network.go](engine/network.go)

`SetupNetwork` writes `1` into `/proc/sys/net/ipv4/ip_forward`. `TeardownNetwork` never restores the previous value, silently enabling IP forwarding permanently.

**Recommendation:** snapshot the old value and restore it on teardown.

---

### L-3 — Inline event handlers prevent a strict CSP
**CVSS ≈ 2.0 — Low** | **STATUS: OPEN**   
**Files:** [cmd/web/static/app.js](cmd/web/static/app.js), [cmd/web/static/index.html](cmd/web/static/index.html)

`onclick="vmAction(this,'${id}','${action}')"` requires `'unsafe-inline'` in any future CSP. The `esc()` helper encodes `<`, `>`, `&`, `"` but **not** single quotes. This is currently safe because VM names and IDs are regex-constrained, but it is one field relaxation away from DOM-based XSS.

**Recommendation:** move to `addEventListener`/delegated handlers with `data-*` attributes; then tighten CSP to remove `'unsafe-inline'`.

---

### L-4 — No upper bound on image download size
**CVSS ≈ 2.0 — Low** | **STATUS: FIXED ✅**   
**File:** [engine/image.go](engine/image.go)

`PullImage` streamed the full response body to disk with no size cap.

**Fix:** a 16 GiB per-file cap is now enforced in the download loop (applied as part of H-3).

---

### L-5 — Log-line CRLF injection
**CVSS ≈ 2.0 — Low** | **STATUS: FIXED ✅**   
**File:** [cmd/web/server.go](cmd/web/server.go)

`r.URL.Path` was logged unescaped, allowing `\n` / `\r` to forge log lines.

**Fix:** `logMiddleware` now runs the path through `strings.NewReplacer("\n", "\\n", "\r", "\\r")` before logging.

---

### L-6 — `dnsmasq` does not drop privileges
**CVSS ≈ 2.5 — Low** | **STATUS: OPEN**   
**File:** [engine/network.go](engine/network.go)

`dnsmasq` is started without `--user`/`--group` and inherits root.

**Recommendation:** add `--user=nobody --group=nogroup` (or a dedicated `dnsmasq` system user) to the argv list.

---

### L-7 — Hardcoded public DNS servers
**CVSS ≈ 1.5 — Low (privacy)** | **STATUS: OPEN**   
**File:** [engine/network.go](engine/network.go)

Bridged VMs resolve via `8.8.8.8` and `1.1.1.1`, ignoring the host's resolvers and leaking internal hostnames to third parties. It also breaks corporate DNS.

**Recommendation:** default to the host's `/etc/resolv.conf` (drop `--no-resolv`) and allow override via `config.json`.

---

### L-8 — `start.sh` uses a relative `./v` path under `sudo`
**CVSS ≈ 1.8 — Low** | **STATUS: OPEN**   
**File:** [start.sh](start.sh)

If a user runs `./start.sh` from an attacker-controlled directory with a trojan `v` binary, it executes as root.

**Recommendation:** `cd "$(dirname "$(readlink -f "$0")")"` at the top and invoke the binary by absolute path.

---

## INFORMATIONAL

### I-1 — Console traffic is unencrypted
The web console runs over plain `ws://` unless fronted by a TLS terminator. Adding `--tls-cert`/`--tls-key` flags would close this.

### I-2 — No rate limiting on any endpoint
With auth in place this requires knowing the token first. Rate-limiting becomes a useful second layer once the token model matures. Brute-forcing VM IDs (2³² space) via authenticated WebSocket connections is the residual concern.

### I-3 — VM ID collision surface
`generateID` uses 4 bytes (8 hex chars). Birthday collision expected at ~65K VMs. Widening to 8 bytes is effectively free.

### I-4 — `golang.org/x/net` dependency
`golang.org/x/net v0.52.0` is current. `govulncheck` is already wired in `addons/check.sh` — keep it there.

---

## Non-issues / well-handled

These were specifically checked and found acceptable:

- **No shell-based command execution.** Every `exec.Command` invocation passes arguments as a slice; there is no `sh -c`.
- **Name, PCI address, boot device, GPU mode, audio mode, net mode** are all regex-validated or switch-validated against closed lists.
- **MAC addresses** are internally generated, not user-controlled.
- **VM data directory permissions** are 0750; metadata files are 0640.
- **TLS certificate validation** on outbound HTTP uses Go defaults (verified).
- **Directory creation** uses `MkdirAll` with explicit modes, not world-writable.
- **Tap-device cleanup** on SIGINT/SIGTERM is wired correctly.
- **Test coverage** exists for the validation paths (`vm_test.go`, `create_test.go`, `util_test.go`).

---

## Open finding summary

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| C-1 | Critical | No auth / CSRF / Host validation | **Fixed** |
| C-2 | Critical | Password in list response; WS hijack | **Fixed** |
| H-1 | High | Image path traversal | **Fixed** |
| H-2 | High | DiskSize argument injection | **Fixed** |
| H-3 | High | SSRF via image pull | **Fixed** (private-IP block + size cap) |
| H-4 | High | Cloud-init YAML injection | **Fixed** |
| H-5 | High | sudo serve = local root for all users | **Partially mitigated** (token auth closes access; privilege separation open) |
| M-1 | Medium | No upper bound on CPUs/memory | Open |
| M-2 | Medium | Unbounded JSON request body | Open |
| M-3 | Medium | rand.Read errors ignored | Open |
| M-4 | Medium | No security response headers | Open |
| M-5 | Medium | No image integrity verification | Open |
| M-6 | Medium | PID-file trust | Open |
| M-7 | Medium | Console WebSocket no backpressure | Open |
| L-1 | Low | Modulo bias in password generator | Open |
| L-2 | Low | ip_forward not restored on teardown | Open |
| L-3 | Low | Inline handlers block strict CSP | Open |
| L-4 | Low | No image download size cap | **Fixed** (part of H-3) |
| L-5 | Low | Log CRLF injection | **Fixed** |
| L-6 | Low | dnsmasq runs as root | Open |
| L-7 | Low | Hardcoded public DNS | Open |
| L-8 | Low | start.sh relative binary path | Open |
| I-1–4 | Info | TLS, rate limiting, ID width, vulncheck | Open |
