# App store client key

The client key prevents casual access to the Space catalog index. Opening
`https://boangs.com/rmkit-space/index.json` in a browser returns 404; the Space server on the device can access it.

## What this does and does not protect

**It helps prevent:** casual browsing, crawlers following public links, and retrieving the index directly from the
repository. The same check applies to the index and app packages, so saved direct links stop working as well.

**It does not prevent:** extracting the key from a client binary with tools such as `strings`, or accessing the
index from a rooted device with a packet capture or a direct request to `http://device:8080/space/store`. That
device endpoint must exist for the Discover page to work.

This is a gate, not an authorization system. Enforcing paid access requires validating an activation code during
the download flow, where the secret is not distributed in the client.

## Server configuration (nginx)

List accepted keys in the `http` block. Using a map allows old and new keys to overlap during a rollout; remove
the old key after devices have updated:

```nginx
map $http_x_space_key $space_denied {
    default                                        1;   # Missing or unrecognized key: deny.
    "<current-key>"                                0;   # Current key.
    # "<previous-key>"                             0;   # Keep during rollout; remove after devices update.
}
```

Protect the catalog index and app packages in the `server` block:

```nginx
location = /rmkit-space/index.json {
    if ($space_denied) { return 404; }
}

location ~ ^/rmkit-space/.+\.zip$ {
    if ($space_denied) { return 404; }
}
```

**Return 404 instead of 403.** A 403 confirms that a protected resource exists; a 404 does not.

After editing, run `nginx -t && nginx -s reload`.

## Release

**Never commit the key to the repository.** This repository is public, so committing the key would defeat the
check. Generate one with `python3 -c "import secrets; print(secrets.token_urlsafe(32))"` and store it privately
(for example, in a password manager or a server-side `.env` file). Inject it at build time:

```sh
make STORE_KEY=<current-key>
```

The build also works with no key; in that case, space-server does not send the header. **Self-hosted stores and
local development are unaffected**; a fork can point to its own catalog.

## Rotate the key

You do not need to publish a new device bundle to rotate the key. Override the build-time value with an
environment variable in the xochitl drop-in:

```ini
Environment=SPACE_STORE_KEY=<new-key>
```

Order of operations: **first add** the new key to the nginx map (keep both keys active), publish the update,
wait for devices to update, then remove the old key. Reversing this order will disconnect devices that have not
updated yet.

## Verify

```sh
# Browser request: expected response is 404.
curl -s -o /dev/null -w '%{http_code}\n' https://boangs.com/rmkit-space/index.json

# Authenticated request: expected response is 200.
curl -s -o /dev/null -w '%{http_code}\n' -H 'X-Space-Key: <key>' \
     https://boangs.com/rmkit-space/index.json
```

## Handshake mode (store-server)

The key above is only a coarse outer check. Authorization uses a three-step handshake; the server lives in a
separate private repository (`boangs/remarkable-space-store`). Refer to that repository's README for deployment
and operations.

Enable the handshake on the device:

```ini
# /etc/systemd/system/remarkable-space.service.d/10-store-api.conf
[Service]
Environment=SPACE_STORE_API=https://boangs.com/rmkit-space/api
```

**Use a drop-in; do not edit the main service file.** The installer generates that file, so a reinstall overwrites
local edits. On RMPP, `/etc` is a tmpfs overlay (upperdir is under `/var/volatile`); persist the change to the
lower layer as well:

```sh
mount --bind / /tmp/lowerroot && mount -o remount,rw /tmp/lowerroot
mkdir -p /tmp/lowerroot/etc/systemd/system/remarkable-space.service.d
cp /etc/systemd/system/remarkable-space.service.d/10-store-api.conf /tmp/lowerroot/etc/...
sync && umount /tmp/lowerroot
```

**Rollout order:** move devices to handshake mode and verify it works first. **Only then** make the static
`index.json` and ZIP links return 404 in nginx. Doing this in the opposite order will disconnect every device
that has not switched yet.
