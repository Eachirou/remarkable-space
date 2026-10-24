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
# 先清空内置应用目录再解包: 这个目录完全由安装器管, tar 只会覆盖同名文件、不会删
# 已经移走的。应用从内置挪到商店之后 (五个小游戏就是这么挪的), 老设备上的内置副本
# 会一直留着 —— 用户从商店装了新版、再删掉, 内置那份又冒出来, 看着就是"删不掉",
# 再删一次还会收到"内置应用随 rmkit-cn 一起管理"。
# 用户自己装的应用在 ~/.local/share/.../apps, 不在这里, 清空伤不到。
tar -C "$HERE" -cf - shell kit apps hooks | ssh $SSHOPT "$D" "rm -rf /home/root/rmkit-cn/space/apps /home/root/rmkit-cn/space/hooks && tar -C /home/root/rmkit-cn/space -xf -"

echo "→ 推后端与静态页"
tar -C "$HERE" -cf - static | ssh $SSHOPT "$D" "tar -C /home/root/remarkable-space -xf -"
scp $SSHOPT "$HERE/dist/$BIN" "$D:/home/root/remarkable-space/space-server.new"
scp $SSHOPT "$HERE/systemd/remarkable-space.service" "$D:/tmp/remarkable-space.service"
scp $SSHOPT "$HERE/systemd/zz-space.conf" "$D:/tmp/zz-space.conf"

echo "→ 换服务 (/etc 是 tmpfs overlay, 必须 upper + ext4 lower 双写, 否则重启即失)"
ssh $SSHOPT "$D" 'sh -s' <<'REMOTE'
set -eu
U=/etc/systemd/system
unit=remarkable-space.service
# 内容没变就别重启 xochitl
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
  # 空间自己的 xochitl 环境 (编译应用的模块搜索路径)。单独一个 drop-in, 不动底座那份 ——
  # systemd 会把 xochitl.service.d/ 下所有 .conf 合并, 所以空间加能力不必让用户先去
  # 更新那个已经冻结的公开底座。
  cp /tmp/zz-space.conf $d/xochitl.service.d/zz-space.conf
  chmod 644 $d/xochitl.service.d/zz-space.conf
  # systemctl enable 建的 symlink 落在 overlay upper 上, 重启就丢; 两层都手工建
  ln -sf $U/$unit $d/multi-user.target.wants/$unit
  # 旧服务两层都要撤自启并 mask。只 disable 不够: 以后重装 rmkit-cn 底座, 它的
  # 安装器会把自启 symlink 又建回来, 两个进程抢 8080。mask 指向 /dev/null,
  # 谁 enable/start 都起不来。
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
# drop-in 改了要 xochitl 重启才生效。只在它确实变过时才重启 —— 界面重启代价不小,
# 而且短时间内反复重启会触发 systemd 的启动限制。
if [ "${SPACE_DROPIN_CHANGED:-0}" = 1 ]; then
  echo "  → xochitl 重启一次 (让新的模块搜索路径生效)"
  systemctl restart xochitl 2>/dev/null || true
fi
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
