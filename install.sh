#!/bin/sh
# 「空间」安装器 — 前台 (外壳 + 内置应用) 与后台 (space-server) 一起装。
#
# space-server 顶替 rmkit-cn 的 upload-server: 同样监听 8080, 路由逐条兼容,
# 装的时候会停掉并禁用 rmkit-cn-upload.service。
#
# 仍然依赖 rmkit-cn 当底座 —— 注入链 (qmldiff/.so/中文汉化/输入法后端 ime-http)
# 还在它那边, 高级面板里的「空间」入口也由它注入。底座冻结不动, 只装一次。
#
# 设备上的数据路径一律沿用 rmkit-cn 时期的位置 (~/.local/share/rmkit-cn/...),
# 换名字就等于丢用户的字体、锁屏图、已装应用和应用数据。
#
# 用法: DEVICE_IP=10.11.99.1 sh install.sh
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
  *) echo "✗ 不认识的架构 $ARCH" >&2; exit 1 ;;
esac
[ -f "$HERE/dist/$BIN" ] || { echo "✗ 缺 dist/$BIN, 先跑 make" >&2; exit 1; }
echo "设备 $DEVICE_IP 架构 $ARCH → $BIN"

ssh $SSHOPT "$D" "[ -d /home/root/rmkit-cn ]" \
  || { echo "✗ 设备上没装 rmkit-cn, 空间依赖它的注入链与面板入口" >&2; exit 1; }

echo "→ 推外壳与内置应用"
ssh $SSHOPT "$D" "mkdir -p /home/root/rmkit-cn/space /home/root/remarkable-space \
                  /home/root/.local/share/rmkit-cn/space/apps \
                  /home/root/.local/share/rmkit-cn/space/data"
tar -C "$HERE" -cf - shell kit apps | ssh $SSHOPT "$D" "tar -C /home/root/rmkit-cn/space -xf -"

echo "→ 推后端 / 静态页 / 矢量底图"
tar -C "$HERE" -cf - static | ssh $SSHOPT "$D" "tar -C /home/root/remarkable-space -xf -"
scp $SSHOPT "$HERE/dist/$BIN" "$D:/home/root/remarkable-space/space-server.new"
scp $SSHOPT "$HERE/data/geo.json" "$D:/home/root/remarkable-space/geo.json"
scp $SSHOPT "$HERE/systemd/remarkable-space.service" "$D:/tmp/remarkable-space.service"

echo "→ 换服务 (/etc 是 tmpfs overlay, 必须 upper + ext4 lower 双写, 否则重启即失)"
ssh $SSHOPT "$D" 'sh -s' <<'REMOTE'
set -eu
U=/etc/systemd/system
unit=remarkable-space.service
old=rmkit-cn-upload.service

mkdir -p /tmp/lc && mount --bind / /tmp/lc 2>/dev/null || true
mount -o remount,rw /tmp/lc 2>/dev/null || true
L=/tmp/lc/etc/systemd/system

mkdir -p $U/multi-user.target.wants $L/multi-user.target.wants
for d in $U $L; do
  cp /tmp/remarkable-space.service $d/$unit
  chmod 644 $d/$unit
  # systemctl enable 建的 symlink 落在 overlay upper 上, 重启就丢; 两层都手工建
  ln -sf $U/$unit $d/multi-user.target.wants/$unit
  # 旧服务两层都要撤自启并 mask。只 disable 不够: 以后重装 rmkit-cn 底座, 它的
  # 安装器会把自启 symlink 又建回来, 两个进程抢 8080。mask 指向 /dev/null,
  # 谁 enable/start 都起不来。
  rm -f $d/multi-user.target.wants/$old
  ln -sf /dev/null $d/$old
done
sync; umount -l /tmp/lc 2>/dev/null || true; rmdir /tmp/lc 2>/dev/null || true
rm -f /tmp/remarkable-space.service

systemctl stop $old 2>/dev/null || true

cd /home/root/remarkable-space
chmod +x space-server.new && mv space-server.new space-server
systemctl daemon-reload
systemctl restart $unit
REMOTE

echo "→ 自检"
ssh $SSHOPT "$D" 'sleep 2
systemctl is-active remarkable-space >/dev/null || { journalctl -u remarkable-space -n 25 --no-pager; exit 1; }
echo "  服务: $(systemctl is-active remarkable-space) / $(systemctl is-enabled remarkable-space)"
echo "  旧服务: $(systemctl is-active rmkit-cn-upload.service) / $(systemctl is-enabled rmkit-cn-upload.service 2>&1)  (应为 inactive / masked)"
for p in /space/apps /qr-info /fonts /screens; do
  code=$(wget -q -S -O /dev/null "http://127.0.0.1:8080$p" 2>&1 | sed -n "s|.*HTTP/1.1 \([0-9]*\).*|\1|p" | head -n 1)
  echo "  $p → ${code:-连不上}"
done'
echo "✓ 完成"
