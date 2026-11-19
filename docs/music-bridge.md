# Spotify Connect bridge

The Space Music app is a remote control for Spotify Connect. It does not play audio on the reMarkable 2; Spotify
plays on the computer or another Spotify Connect device. Playback controls require Spotify Premium.

The computer runs the bridge and keeps Spotify credentials. The tablet stores only its paired bridge token, not
the Spotify access or refresh token.

## 1. Register a Spotify app

Create an app in the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard). In the app settings,
add this exact Redirect URI and save it:

```text
http://127.0.0.1:9120/oauth/callback
```

Copy the app's **Client ID**. The bridge uses Spotify's Authorization Code with PKCE flow, so no Client Secret
is needed. Request only the playback permissions used by the app:

- `user-read-playback-state`
- `user-modify-playback-state`

If the Spotify app is in Development Mode, add the Spotify account used for testing to its allowed users.

## 2. Build and start the bridge

From the repository root, set the Client ID and build:

```bash
export MUSIC_BRIDGE_CLIENT_ID='your-client-id'
go build -o /tmp/music-bridge ./cmd/music-bridge
```

Run the bridge on the computer's network interfaces so the tablet can reach it:

```bash
/tmp/music-bridge -addr 0.0.0.0:9120
```

The terminal prints an 8-digit, one-time pairing code, valid for 10 minutes, and a local Spotify authorization
URL. Open that URL in a browser **on the computer running the bridge**, sign in, and approve the requested
permissions. The callback page confirms authorization; the bridge stores the refresh token in
`~/.config/remarkable-space/music-bridge/spotify.json` with owner-only permissions and refreshes access tokens
automatically.

Keep the bridge terminal open while using the app. If the pairing code expires, restart the bridge to get a new
one. A previously paired device remains paired across bridge restarts.

## 3. Find the computer's Wi-Fi address

On Linux, use:

```bash
ip -br -4 addr show
```

Choose the IPv4 address on the **Wi-Fi interface** that shares the tablet's private network. Do not use
`127.0.0.1`, a Docker address such as `172.17.0.1`, a public address, or a wired-network address if the tablet is
on Wi-Fi. `hostname -I` may print addresses from several interfaces.

For example, if `wlo1` is the Wi-Fi interface and shows `192.168.178.28/24`, use `192.168.178.28`.

## 4. Pair the reMarkable

Open **Espace → Music** on the tablet and enter:

```text
Bridge URL: http://<computer-wifi-address>:9120
Pairing code: <8-digit code printed in the bridge terminal>
```

Press **Pair device**. The app exchanges the short-lived code for a random device token and stores the token in
its app data. You do not need to type or copy either Spotify tokens or the long bridge token.

## 5. Test playback

First start a track in Spotify on the computer or another Spotify Connect device. Then press **Refresh** on the
tablet. The status endpoint returns `{"playing":false}` when Spotify has no active playback; that response means
the bridge is reachable, not that pairing failed. Start playback and refresh again before testing Pause, Play,
Next, or Previous.

The bridge API includes:

```text
GET  /health
GET  /api/status
POST /api/control  {"action":"play|pause|next|previous"}
POST /api/pair      {"code":"12345678"}
```

Only `/health` is public. Playback requests require the paired device token. The pairing code expires after 10
minutes, accepts at most 10 attempts per bridge run, and can be used once.

## Troubleshooting

- **Bridge unavailable:** check that the process is running, both devices use the same Wi-Fi, the tablet URL uses
  the computer's Wi-Fi IPv4 address, and the firewall allows TCP port 9120 on the private network.
- **Pairing code rejected/expired:** copy the current 8-digit code from the bridge terminal; restart the bridge
  if it expired or was already used.
- **No playback/device:** start playback in Spotify and make sure a Spotify Connect device is active.
- **Spotify authorization error:** repeat the local authorization link. Check that the account is permitted in
  Spotify app Development Mode.
- **Playback command rejected:** verify the account has Premium and that the active device is not restricted.

## Security

- The bridge binds to loopback by default. Use `-addr 0.0.0.0:9120` only when the tablet needs LAN access.
- Restrict TCP port 9120 to the trusted private network in the computer firewall. Never port-forward it or expose
  it to the public internet.
- The temporary pairing code and device token are credentials. Do not post them publicly. Pair only on a trusted
  home network.
- The Spotify refresh token is stored locally with owner-only file permissions. Do not share it or commit it to
  Git. Revoke the bridge's access from your Spotify account settings if the computer or token file is compromised.
- For the older manual-token workflow, `SPOTIFY_ACCESS_TOKEN` remains supported, but OAuth is recommended because
  the bridge can refresh tokens automatically.
