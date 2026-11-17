# Space Assistant

Space Assistant is a desktop installer for macOS and Windows. Connect a supported reMarkable tablet over USB to
install Space and its required integration base together.

The assistant connects directly over SSH; it does not use a remote server or upload your files. Commands run on
the device and files written during installation are recorded in a local audit log. Use **Open log directory** in
the interface to view it.

## What it installs

**Integration base** — source is in `base/` (originally part of rmkit-cn). It contains xovi, runtime QML injection,
system language resources, and the input-method backend. This is the system-sensitive part of the installation.
Firmware mismatches can prevent the interface from starting, so deployment uses the staged checks and rollback
logic in the assistant.

**Space** — the shell, built-in apps, and the `space-server` backend. The server replaces the retired upload-server
and uses port 8080. Space loads QML apps asynchronously; an app error should not crash xochitl.

On supported Paper Pro Move configurations, the assistant can also install single-slot Android.

## Build

Run these commands from the repository root:

```sh
# 1. Build the device-side server binaries.
make all

# 2. Build the Space installation bundle (base artifacts are read from base/dist).
cd assistant && make bundle

# 3. Build the desktop application.
make build-mac          # or make build-win
open build/bin/space-assistant.app
```

See [base/README.md](../base/README.md) for base-layer build requirements.

## Command-line interface

The CLI uses the same deployment code as the GUI and is useful for CI and remote diagnostics:

```sh
make cli                 # macOS / Windows
make cli-linux           # Linux amd64

export RM_PASSWORD='<device SSH password>'
dist/assistant-cli-darwin-arm64 -host 10.11.99.1 probe
dist/assistant-cli-darwin-arm64 -host 10.11.99.1 \
  -bundle dist/space-bundle.zip plan-rmkit
dist/assistant-cli-darwin-arm64 -host 10.11.99.1 \
  -bundle dist/space-bundle.zip run-rmkit
dist/assistant-cli-linux-amd64 -host remarkable.local probe

# Calculate a deployment plan without connecting to a device.
dist/assistant-cli-darwin-arm64 -bundle dist/space-bundle.zip \
  -arch armv7l -model rm2 plan-offline
```

Run `assistant-cli -h` for the available flags and commands. The CLI currently prints its messages in English.

## Device paths and deployment constraints

- **Keep device data under `~/.local/share/rmkit-cn/`.** Fonts, sleep screens, installed apps, and app data use
  these paths; renaming them would make existing user data appear to be missing.
- **The Space shell remains under `/home/root/rmkit-cn/space/`.** This path is embedded in the injection library;
  changing it requires rebuilding the affected libraries.
- **`/etc` is a tmpfs overlay.** Persistent systemd units and `wants` symlinks must be written to both the
  upper layer and the ext4 lower layer. Changes written only to `/etc` do not survive a reboot.
- **Preserve file ownership during transfer.** The deployment scripts use `--no-same-owner`; restoring the
  build machine's UID could break access to `/home/root`. Never recursively change ownership of `/home/root`:
  single-slot Android system and data files also live there.
- **Verify device-side binaries before changing services.** An architecture mismatch produces an
  `Exec format error`; discovering this after replacing the existing service can disable dependent features.
