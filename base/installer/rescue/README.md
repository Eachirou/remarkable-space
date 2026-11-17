# Archived RM2 recovery scripts

> **Legacy procedure:** These scripts were written for an earlier rmkit-cn installation and are not the supported
> Space recovery workflow. For a device that will not boot, connect it over USB and use **Space Assistant →
> Device won't boot → Emergency repair** first. Do not follow the old manual repair commands below without
> checking them against the installed firmware and current project instructions.

The scripts in this directory were intended to assist with recovery and diagnostics on a reMarkable 2:

- `rm2-rescue.sh` runs on a Linux host (such as a Raspberry Pi), monitors USB/SSH state, and collects evidence.
- `rm2-forensic.sh` runs on the device after SSH becomes available and investigates library-loading failures.

## Initial triage

| Observation | Possible state |
|---|---|
| No SSH and no USB device | The device may not be booting; check power, cable, and USB port first. |
| USB device identifies as NXP `15a2:0076` | The device may be in SDP/BootROM mode and require firmware-specific recovery. |
| USB gadget is present but SSH is unavailable | The kernel may be running while xochitl or sshd has failed. |
| SSH is available | Collect diagnostics before making changes. |

These observations are not a diagnosis. If the device is under warranty or contains important data, stop and seek
qualified reMarkable recovery support before attempting low-level recovery.

## Legacy monitoring and diagnostics

The historical host workflow copied both scripts to a Linux machine connected to the tablet over USB, then ran:

```sh
chmod +x rm2-rescue.sh rm2-forensic.sh
bash rm2-rescue.sh
```

The script created an evidence directory and reported USB/SSH state changes. When SSH was available, the
forensic script could be copied to the device and run there. Review its source before running it: it may not match
current firmware.

The former procedure described an SDP recovery path using the upstream
[remarkable2-recovery project](https://github.com/ddvk/remarkable2-recovery). Do not run that project or write
bootloader images unless the recovery instructions are confirmed for the exact device and firmware.

## Historical failure mode

The old installation could add a xochitl systemd drop-in that preloaded xovi and IME libraries. If a library was
incompatible with the firmware, xochitl could crash repeatedly. The old recovery approach disabled that drop-in
while preserving files under `/home/root/rmkit-cn/` and `/home/root/xovi/` for diagnosis.

The current assistant's Emergency repair flow is the supported way to restore stock startup. It is designed to
preserve user notes; review the confirmation text in the assistant before proceeding.

## Safety notes

- Never run `fsck` on reMarkable partitions as a generic repair step; device-specific metadata can be damaged.
- RM2 has no A/B boot slots, so low-level boot changes can make the device difficult to recover.
- Do not commit or share the device's SSH password. Set `RM2_PASSWORD` privately when a legacy script requires it;
  this guide intentionally does not contain a default password.
- `/etc` may be an overlay. A change made only in the active upper layer may not survive reboot.
- Prefer the current assistant's diagnostics and repair workflow over these archived scripts.
