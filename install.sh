#!/bin/sh
# 「空间」安装器 — 把外壳、内置应用、后端推到设备。
#
# 与 rmkit-cn 的关系: 本脚本只装空间自己的东西。空间里那些"系统功能入口"应用的
# 启动动作仍由 rmkit-cn 的后端执行, space-server 会把不认识的 /apps/* 转发过去。
# 所以设备上要先装好 rmkit-cn, 否则蓝牙/输入法/上传那几个入口点了没反应。
#
# 用法: DEVICE_IP=10.11.99.1 sh install.sh
set -eu
DEVICE_IP=${DEVICE_IP:-10.11.99.1}
DEVICE_USER=${DEVICE_USER:-root}
SSH="ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
SCP="scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
D="$DEVICE_USER@$DEVICE_IP"
HERE=$(cd "$(dirname "$0")" && pwd)

ARCH=$($SSH "$D" "uname -m")
case "$ARCH" in
  aarch64) BIN=space-server-aarch64 ;;
  armv7l)  BIN=space-server-armv7 ;;
  *) echo "✗ 不认识的架构 $ARCH" >&2; exit 1 ;;
esac
[ -f "$HERE/dist/$BIN" ] || { echo "✗ 缺 dist/$BIN, 先跑 make" >&2; exit 1; }
echo "设备 $DEVICE_IP 架构 $ARCH → $BIN"

echo "→ 推外壳与内置应用"
$SSH "$D" "mkdir -p /home/root/rmkit-cn/space /home/root/remarkable-space/bin \
           /home/root/.local/share/rmkit-cn/space/apps \
           /home/root/.local/share/rmkit-cn/space/data"
tar -C "$HERE" -cf - shell kit apps | $SSH "$D" "tar -C /home/root/rmkit-cn/space -xf -"

echo "→ 推后端与矢量底图"
$SCP "$HERE/dist/$BIN" "$D:/home/root/remarkable-space/bin/space-server.new"
$SCP "$HERE/data/geo.json" "$D:/home/root/remarkable-space/bin/geo.json"
$SCP "$HERE/systemd/remarkable-space.service" "$D:/etc/systemd/system/remarkable-space.service"

echo "→ 启用服务"
$SSH "$D" "cd /home/root/remarkable-space/bin && chmod +x space-server.new && mv space-server.new space-server \
           && systemctl daemon-reload && systemctl enable --now remarkable-space \
           && sleep 2 && systemctl is-active remarkable-space"
echo "✓ 完成。高级面板里的「空间」入口会在下次打开时自动出现 (rmkit-cn 按 Space.qml 是否存在判断)。"
