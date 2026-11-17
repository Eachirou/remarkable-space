#!/bin/sh
# Space installer — installs the shell, built-in apps, and space-server.
#
# space-server replaces rmkit-cn's upload-server on port 8080 and preserves
# route compatibility. Installation stops and disables rmkit-cn-upload.service.
#
# Space still requires rmkit-cn as its integration base: the injection chain,
# system translations, and IME backend remain there. The base also injects the
# Space entry into the Advanced panel. The base is frozen and installed once.
#
# Device data paths retain their rmkit-cn locations
# (~/.local/share/rmkit-cn/...). Renaming them would hide user fonts, sleep
# screens, installed apps, and app data.
#
# Usage: DEVICE_IP=10.11.99.1 sh install.sh
set -eu
DEVICE_IP=${DEVICE_IP:-10.11.99.1}
DEVICE_USER=${DEVICE_USER:-root}
SSHOPT="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
D="$DEVICE_USER@$DEVICE_IP"
HERE=$(cd "$(dirname "$0")" && pwd)

ARCH=$(ssh $SSHOPT "$D" "uname -m")
case "$ARCH" in
  aarch64) BIN=space-server-aarch64 ;;
  armv7l)  BIN=space-server-armv7 ;;
  *) echo "✗ Unsupported device architecture: $ARCH" >&2; exit 1 ;;
esac
[ -f "$HERE/dist/$BIN" ] || { echo "✗ Missing dist/$BIN; run make first" >&2; exit 1; }
echo "Device $DEVICE_IP, architecture $ARCH → $BIN"

ssh $SSHOPT "$D" "[ -d /home/root/rmkit-cn ]" \
  || { echo "✗ rmkit-cn is not installed; Space requires its injection chain and panel entry" >&2; exit 1; }

echo "→ Installing shell and built-in apps"
ssh $SSHOPT "$D" "mkdir -p /home/root/rmkit-cn/space /home/root/remarkable-space \
                  /home/root/.local/share/rmkit-cn/space/apps \
                  /home/root/.local/share/rmkit-cn/space/data"
# Clear the installer-managed built-in app directory before extracting. tar
# overwrites files but does not remove files that were moved out of the bundle.
# Without clearing, old built-in copies can reappear after a catalog app is
# removed. User-installed apps live in ~/.local/share/.../apps and are untouched.
# shell/kit may be symlinks left by an on-device update (pointing into rel/<version>);
# remove those links, rel/, and .prev before restoring real directories.
S=/home/root/rmkit-cn/space
tar -C "$HERE" -cf - shell kit apps hooks themes | ssh $SSHOPT "$D" "rm -rf $S/apps $S/hooks $S/themes $S/shell $S/kit $S/rel $S/shell.prev $S/kit.prev && tar -C $S -xf -"

echo "→ Installing server and static files"
tar -C "$HERE" -cf - static | ssh $SSHOPT "$D" "tar -C /home/root/remarkable-space -xf -"
scp $SSHOPT "$HERE/dist/$BIN" "$D:/home/root/remarkable-space/space-server.new"
scp $SSHOPT "$HERE/systemd/remarkable-space.service" "$D:/tmp/remarkable-space.service"
scp $SSHOPT "$HERE/systemd/zz-space.conf" "$D:/tmp/zz-space.conf"

echo "→ Updating services (/etc is a tmpfs overlay; persist changes to both upper and ext4 lower layers)"
ssh $SSHOPT "$D" 'sh -s' <<'REMOTE'
set -eu
U=/etc/systemd/system
unit=remarkable-space.service
# Avoid restarting xochitl when the drop-in has not changed.
if cmp -s /tmp/zz-space.conf $U/xochitl.service.d/zz-space.conf 2>/dev/null; then
  SPACE_DROPIN_CHANGED=0
else
  SPACE_DROPIN_CHANGED=1
fi
old=rmkit-cn-upload.service

mkdir -p /tmp/lc && mount --bind / /tmp/lc 2>/dev/null || true
mount -o remount,rw /tmp/lc 2>/dev/null || true
L=/tmp/lc/etc/systemd/system

mkdir -p $U/multi-user.target.wants $L/multi-user.target.wants
mkdir -p $U/xochitl.service.d $L/xochitl.service.d
for d in $U $L; do
  cp /tmp/remarkable-space.service $d/$unit
  chmod 644 $d/$unit
  # Space's xochitl environment (compiled-app module search path) lives in a
  # separate drop-in. systemd merges all files in xochitl.service.d, so Space
  # can add this path without requiring an update to the frozen base.
  cp /tmp/zz-space.conf $d/xochitl.service.d/zz-space.conf
  chmod 644 $d/xochitl.service.d/zz-space.conf
  # systemctl enable creates a symlink in the overlay upper layer, which is
  # lost on reboot; create the link in both layers explicitly.
  ln -sf $U/$unit $d/multi-user.target.wants/$unit
  # Disable and mask the old service in both layers. Disabling alone is not
  # enough: reinstalling the rmkit-cn base could recreate its enable symlink,
  # causing both services to contend for port 8080. The /dev/null mask prevents
  # it from being started or enabled.
  rm -f $d/multi-user.target.wants/$old
  ln -sf /dev/null $d/$old
done
sync; umount -l /tmp/lc 2>/dev/null || true; rmdir /tmp/lc 2>/dev/null || true
rm -f /tmp/remarkable-space.service /tmp/zz-space.conf

systemctl stop $old 2>/dev/null || true

cd /home/root/remarkable-space
chmod +x space-server.new && mv space-server.new space-server
systemctl daemon-reload
systemctl restart $unit
# A changed drop-in takes effect after xochitl restarts. Restart only when it
# changed; UI restarts are disruptive and repeated restarts can hit systemd limits.
if [ "${SPACE_DROPIN_CHANGED:-0}" = 1 ]; then
  echo "  → Restarting xochitl once to apply the module search path"
  systemctl restart xochitl 2>/dev/null || true
fi
REMOTE

echo "→ Verifying installation"
ssh $SSHOPT "$D" 'sleep 2
systemctl is-active remarkable-space >/dev/null || { journalctl -u remarkable-space -n 25 --no-pager; exit 1; }
echo "  Service: $(systemctl is-active remarkable-space) / $(systemctl is-enabled remarkable-space)"
echo "  Legacy service: $(systemctl is-active rmkit-cn-upload.service) / $(systemctl is-enabled rmkit-cn-upload.service 2>&1) (expected: inactive / masked)"
for p in /space/apps /qr-info /fonts /screens; do
  code=$(wget -q -S -O /dev/null "http://127.0.0.1:8080$p" 2>&1 | sed -n "s|.*HTTP/1.1 \([0-9]*\).*|\1|p" | head -n 1)
  echo "  $p → ${code:-unreachable}"
done'
echo "✓ Installation complete"
