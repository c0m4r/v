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
- Cloud-init for automatic SSH key injection and initial setup
- Serial console access (CLI and web terminal via xterm.js)

<img width="751" height="465" alt="image" src="https://github.com/user-attachments/assets/c928f6db-6e17-4c47-8a96-d8ae96f317da" />

- User-mode (NAT) and bridged networking
- Web UI with real-time VM management
- Configurable image registry (add your own images)

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
./v serve
```

Dashboard will be available at http://127.0.0.1:8080

Cross build:

```bash
./addons/build.sh
```

## Quick Start

```bash
# Pull a cloud image
v image pull ubuntu-24.04

# Create a VM
v create --name myvm --image ubuntu-24.04 --memory 1024 --cpus 2

# Start it
v start myvm

# Connect via SSH (user-mode networking)
ssh -p 2222 localhost

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
| `--user-data` | | Path to custom cloud-init user-data file |

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
- VM list with state, IP, and action buttons (start/stop/restart/delete)
- VM creation dialog with image selection (cached + available for download)
- Live serial console via WebSocket (xterm.js)
- Settings management (default SSH key)

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

All VM data, images, and configuration are stored in `~/.local/share/v/` by default. Override with the `V_DATA_DIR` environment variable.

```
~/.local/share/v/
  config.json          # user settings and custom images
  images/              # cached base images
  vms/
    <vm-id>/
      vm.json          # VM metadata
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
ssh -p 2222 localhost
```

### Bridge Mode

Requires root for initial setup. VMs get real IPs on a shared bridge network (`10.10.10.0/24`). IP addresses are assigned via dnsmasq DHCP and visible in `v info` / `v list`.

```bash
sudo v net setup       # one-time bridge/NAT/DHCP setup
v create --name myvm --image ubuntu-24.04 --net bridge
v start myvm
v info myvm            # shows assigned IP
ssh 10.10.10.x
```

## Running Tests

```bash
./addons/check.sh
```

## License

See [LICENSE](LICENSE) file.
