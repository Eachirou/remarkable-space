# 应用商店的客户端标识

让「空间」之外的东西拿不到商店索引：用浏览器打开 `https://boangs.com/rmkit-space/index.json`
看到的是 404，只有设备上的 space-server 能取到内容。

## 这道门防什么、不防什么

**防住**：随手把地址粘进浏览器、爬虫顺着链接扒、从 GitHub 上读源码找清单。
索引和应用包都归同一道门管，所以记下来的旧链接也失效。

**防不住**：二进制在用户手里，`strings` 能把密钥取出来；设备是 root 的，
抓包或者直接 `curl http://设备:8080/space/store` 也能拿到完整索引 —— 后面这个接口
必须存在，发现页要靠它拿数据。

所以它是一道门槛，不是授权。真要对付白嫖，得在**下载环节**验激活码（见 README
「收费应用」一节），那条路上密钥不在客户端手里，才拦得住。

## 服务端 (nginx)

`http` 块里列出认可的密钥 —— 写成 map 是为了换密钥时能让新旧两版并存一段时间，
等老版本的设备都升级完再删掉旧的那行：

```nginx
map $http_x_space_key $space_denied {
    default                                        1;   # 不带头 / 对不上 → 拒
    "<当前密钥>"                                     0;   # 当前
    # "上一版的密钥"                                0;   # 过渡期保留, 设备升完再删
}
```

`server` 块里管住索引和应用包：

```nginx
location = /rmkit-space/index.json {
    if ($space_denied) { return 404; }
}

location ~ ^/rmkit-space/.+\.zip$ {
    if ($space_denied) { return 404; }
}
```

**返回 404 而不是 403**：403 等于告诉对方"这儿有东西但你没权限"，404 看起来就是没这个文件。

改完 `nginx -t && nginx -s reload`。

## 发版

密钥**绝不能进仓库** —— 这个仓库是开源的，密钥写进来这道门就等于没有。
生成一个：`python3 -c "import secrets; print(secrets.token_urlsafe(32))"`，
存在你自己的地方（密码管理器 / 服务器上的 .env），编译时注入：

```sh
make STORE_KEY=<当前密钥>
```

留空也能编，这时 space-server 不带那个头 —— **自建商店和本地开发不受影响**，
别人 clone 下来指向自己的索引地址照样跑得通。

## 换密钥

设备上不必重新发版，xochitl 的 drop-in 里加一行环境变量就能覆盖编译期的值：

```ini
Environment=SPACE_STORE_KEY=新密钥
```

顺序是：nginx 的 map 里**先加**新密钥（新旧并存）→ 发新版 → 等设备升级完 → 删掉旧的那行。
反过来做会让还没升级的设备当场看不到商店。

## 验证

```sh
# 浏览器视角: 应当 404
curl -s -o /dev/null -w '%{http_code}\n' https://boangs.com/rmkit-space/index.json

# 程序视角: 应当 200
curl -s -o /dev/null -w '%{http_code}\n' -H 'X-Space-Key: <密钥>' \
     https://boangs.com/rmkit-space/index.json
```

## 握手模式 (store-server)

上面那把密钥是最外层的粗筛。真正的授权走三步握手, 服务端是另一个仓库
(`boangs/remarkable-space-store`, 私有), 部署和运维看它的 README。

设备侧打开握手:

```ini
# /etc/systemd/system/remarkable-space.service.d/10-store-api.conf
[Service]
Environment=SPACE_STORE_API=https://boangs.com/rmkit-space/api
```

**放 drop-in 不要改主 service 文件** —— 后者由 install.sh 生成, 重装会把改动冲掉。
RMPP 的 `/etc` 是 tmpfs overlay (upperdir 在 /var/volatile), 写完要双写到下层
才扛得过重启:

```sh
mount --bind / /tmp/lowerroot && mount -o remount,rw /tmp/lowerroot
mkdir -p /tmp/lowerroot/etc/systemd/system/remarkable-space.service.d
cp /etc/systemd/system/remarkable-space.service.d/10-store-api.conf /tmp/lowerroot/etc/...
sync && umount /tmp/lowerroot
```

**上线顺序**: 先让设备都切到握手并确认正常, **最后**才在 nginx 里把静态
`index.json` 和 zip 直链 404 掉。反了的话, 所有还没切的设备当场看不到商店。
