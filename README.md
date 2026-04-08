<div align="center">

# v

![Linux](https://img.shields.io/badge/made%20for-linux-yellow?logo=linux&logoColor=ffffff)
![Go](https://img.shields.io/badge/go%20go-go-blue?logo=go&logoColor=ffffff)
[![License: GPL v3](https://img.shields.io/badge/License-AGPLv3-red.svg)](https://www.gnu.org/licenses/agpl-3.0)

Lightweight KVM virtual machine manager for Linux.

Provides a CLI and web UI for creating, running, and managing QEMU/KVM virtual machines with cloud-init support.

</div>

<img width="991" height="344" alt="image" src="https://github.com/user-attachments/assets/d45729db-8881-4cce-b1fd-d81ec5b54cdf" />

## Features

- Create VMs from cloud images (qcow2/img) or ISO installers
- Thin-provisioned disks (copy-on-write clones of base images)
- Cloud-init for automatic SSH key injection, hostname, and root/default-user password
- Auto-generated root password per VM, revealable from the web UI
- Serial console access (CLI and web terminal via xterm.js)

<img width="751" height="465" alt="image" src="https://github.com/user-attachments/assets/c928f6db-6e17-4c47-8a96-d8ae96f317da" />

- User-mode (NAT) and bridged networking
- Web UI with real-time VM management
- Configurable image registry (add your own images)
- Graceful shutdown cleans up tap devices on exit

## Requirements

- Linux with KVM support (`/dev/kvm`)
- QEMU (`qemu-system-x86_64`, `qemu-img`)
- `genisoimage` or `mkisofs` (for cloud-init ISO generation)
- Go 1.26+ (to build from source)
- Node.js / npm (for web UI frontend dependencies)

Optional (for bridged networking):
- `dnsmasq`
- `iptables`
- Root privileges

## Installation

```bash
git clone https://github.com/c0m4r/v.git
cd v
./addons/npm.sh        # fetches xterm.js into cmd/web/static/vendor/
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(cat VERSION)" -buildvcs=false -o v .
```

Cross build:

```bash
./addons/build.sh
```

## Running v

`v` can run either as an unprivileged user or as root. The two setups differ in networking capabilities and data location:

| | **User mode** (no root) | **Bridge mode** (root) |
|---|---|---|
| **Setup** | `./v serve` | `sudo ./v net setup`<br>`sudo ./v serve` |
| **Networking** | QEMU user-mode NAT | `v-br0` bridge + dnsmasq DHCP + iptables NAT |
| **VM IPs** | none (guest sees 10.0.2.15 internally) | real IPs on `10.10.10.0/24` |
| **SSH access** | host port forwarding<br>`ssh -p 2222 localhost` | direct to guest IP<br>`ssh root@10.10.10.x` |
| **Root required** | no | yes — for bridge, taps, iptables, dnsmasq |
| **Data dir** | `~/.local/share/v/` | `/root/.local/share/v/` |
| **Create flag** | `--net user` (default) | `--net bridge` |

`./start.sh` is a convenience wrapper that sets up bridge networking if missing and launches `v serve` under sudo.

> **Note on the data directory:** Because the default data path is derived from `$HOME`, user-mode VMs live under your home directory while root-mode VMs live under `/root`. VMs created in one mode are not visible from the other. Override the location with the `V_DATA_DIR` environment variable if you want a shared path.

Dashboard will be available at http://127.0.0.1:8080 once `v serve` is running.

## Quick Start

```bash
# Pull a cloud image
v image pull ubuntu-24.04

# Create a VM (root password is auto-generated and printed)
v create --name myvm --image ubuntu-24.04 --memory 1024 --cpus 2

# Start it
v start myvm

# Connect via SSH (user-mode networking)
ssh -p 2222 ubuntu@localhost

# Or attach to the serial console
v console myvm    # Ctrl+] to detach
```

## CLI Reference

### VM Management

| Command | Description |
|---------|-------------|
| `v create --name NAME --image IMAGE [options]` | Create a new VM |
| `v list` | List all VMs |
| `v info <name\|id>` | Show VM details |
| `v start <name\|id>` | Start a VM |
| `v stop <name\|id>` | Graceful ACPI shutdown |
| `v force-stop <name\|id>` | Kill VM process immediately |
| `v restart <name\|id>` | Restart a VM |
| `v delete <name\|id>` | Delete a VM (must be stopped) |
| `v console <name\|id>` | Attach to serial console |
| `v set-boot <name\|id> <disk\|cdrom>` | Change boot device |

### Create Options

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | (required) | VM name |
| `--image` | (required) | Base image name or cached filename |
| `--cpus` | 1 | Number of vCPUs |
| `--memory` | 512 | Memory in MB |
| `--disk` | 10G | Disk size |
| `--net` | user | Network mode: `user` or `bridge` |
| `--ssh-key` | | SSH public key (string or path to .pub file) |
| `--password` | (auto-gen) | Root/default-user password; `none` to disable password auth |
| `--user-data` | | Path to custom cloud-init user-data file (overrides `--ssh-key` and `--password`) |

The password is applied to **both** `root` and the distro default user (`ubuntu`, `debian`, `rocky`, …) via cloud-init.

### Image Management

| Command | Description |
|---------|-------------|
| `v image pull <name\|url>` | Download a cloud image |
| `v image list` | List cached images |
| `v image available` | Show all known images and their URLs |

### Disk Management

| Command | Description |
|---------|-------------|
| `v disk create --path PATH --size SIZE [--backing FILE]` | Create a standalone disk image |

### Networking

| Command | Description |
|---------|-------------|
| `v net setup` | Set up bridge networking (requires root) |
| `v net teardown` | Remove bridge and NAT rules (requires root) |
| `v net status` | Show network infrastructure status |

Bridge networking creates a `v-br0` bridge with subnet `10.10.10.0/24`, runs dnsmasq for DHCP, and configures NAT via iptables.

### Configuration

| Command | Description |
|---------|-------------|
| `v config` | Show current configuration |
| `v config set ssh-key <key\|path>` | Set default SSH key for new VMs |

### Other

| Command | Description |
|---------|-------------|
| `v serve [--listen ADDR]` | Start the web UI (default: `127.0.0.1:8080`) |
| `v version` | Show version |
| `v help` | Show usage |

## Web UI

Start the web server:

```bash
v serve
# or bind to a specific address
v serve --listen 0.0.0.0:9090
```

The web UI provides:
- VM list with state, IP, and action buttons (start/stop/force-stop/restart/delete/console)
- VM creation dialog with image selection (cached + available for download), auto-generated root password with regenerate button, and a "no password" option
- Per-VM password reveal dialog (show/hide/copy the stored root password)
- Live serial console via WebSocket (xterm.js) with fullscreen toggle
- Bridge network option is automatically disabled if v is not running as root
- Settings management (default SSH key)

On shutdown (Ctrl-C or SIGTERM), the web server cleans up all `v-tap-*` interfaces it created.

## ISO Images

You can boot VMs from ISO installer images (e.g., Alpine Linux, any OS installer):

```bash
# Pull an ISO image
v image pull alpine-v3.23

# Create a VM from it (creates blank disk, skips cloud-init)
v create --name alpine --image alpine-virt-3.23.3-x86_64.iso --disk 5G

# Boot from ISO and install via console
v start alpine
v console alpine

# After installation, switch to booting from disk
v stop alpine
v set-boot alpine disk
v start alpine
```

When a VM is created from an ISO:
- A blank disk is created (no backing file)
- Cloud-init is skipped (installers don't use it)
- Boot device is set to `cdrom` automatically
- On start, the ISO is attached as CDROM with boot priority

## Custom Images

Images are configured in `~/.local/share/v/config.json`. Built-in defaults are always available; user entries are merged on top (and can override defaults):

```json
{
  "images": {
    "my-distro": "https://example.com/my-distro-cloud.qcow2",
    "ubuntu-24.04": "https://my-mirror.example.com/ubuntu-noble.img"
  }
}
```

You can also pull any image by URL directly:

```bash
v image pull https://example.com/some-image.qcow2
```

### Built-in Images

| Name | Format |
|------|--------|
| `ubuntu-24.04` | qcow2 (cloud image) |
| `ubuntu-22.04` | qcow2 (cloud image) |
| `debian-13` | qcow2 (cloud image) |
| `debian-12` | qcow2 (cloud image) |
| `alpine-v3.23` | ISO (installer) |
| `rocky-10` | qcow2 (cloud image) |

## Data Directory

VM data, images, and configuration are stored under `$HOME/.local/share/v/` — which means the location depends on which user runs `v`:

- Run as your user → `~/.local/share/v/`
- Run as root (directly or via `sudo`) → `/root/.local/share/v/`

Override the location entirely with the `V_DATA_DIR` environment variable.

```
<data-dir>/
  config.json          # user settings and custom images
  dnsmasq.pid          # dnsmasq PID (bridge mode)
  dnsmasq.leases       # DHCP leases (bridge mode)
  dnsmasq.log          # dnsmasq log (bridge mode)
  images/              # cached base images
  vms/
    <vm-id>/
      vm.json          # VM metadata (includes stored root password)
      disk.qcow2       # VM disk (thin clone or blank)
      cloud-init.iso   # cloud-init data (cloud images only)
      pid              # QEMU process ID (when running)
      qmp.sock         # QMP control socket (when running)
      console.sock     # serial console socket (when running)
```

## Networking Modes

### User Mode (default)

No root required. QEMU provides built-in NAT. SSH access is via port forwarding on the host (starting at port 2222, auto-incremented per VM).

```bash
v create --name myvm --image ubuntu-24.04      # --net user is the default
v start myvm
ssh -p 2222 ubuntu@localhost
```

### Bridge Mode

Requires root. VMs get real IPs on a shared bridge network (`10.10.10.0/24`). IPs are assigned via dnsmasq DHCP and visible in `v info` / `v list`.

```bash
sudo v net setup                                           # one-time bridge/NAT/DHCP setup
sudo v create --name myvm --image ubuntu-24.04 --net bridge
sudo v start myvm
sudo v info myvm                                           # shows assigned IP
ssh ubuntu@10.10.10.x
```

Or, for the web UI:

```bash
sudo v net setup
sudo v serve
```

`v net setup` creates `v-br0`, assigns `10.10.10.1/24`, enables IP forwarding, adds iptables MASQUERADE + FORWARD rules for the default route interface, and starts dnsmasq. `v net teardown` reverses all of it and removes any leftover `v-tap-*` interfaces.

## Running Tests

```bash
./addons/check.sh
```

## License

See [LICENSE](LICENSE) file.
