# WLAN music bridge

The `music` Space app is a remote control. It does not play audio on the reMarkable
2 and it does not store a Spotify credential on the tablet.

## Run the computer bridge

Build it on the computer:

```bash
go build -o /tmp/music-bridge ./cmd/music-bridge
```

Set two different tokens:

```bash
export MUSIC_BRIDGE_TOKEN="$(openssl rand -hex 24)"
export SPOTIFY_ACCESS_TOKEN="your-short-lived-spotify-access-token"
```

The bridge token is typed into the Space app. The Spotify token stays on the
computer. Start the bridge on the computer's WLAN address:

```bash
/tmp/music-bridge -addr 0.0.0.0:9120
```

Then open **Music** on the tablet and save:

```text
Bridge URL: http://<computer-wlan-address>:9120
Bridge token: <value of MUSIC_BRIDGE_TOKEN>
```

The bridge exposes:

```text
GET  /health
GET  /api/status
POST /api/control  {"action":"play|pause|next|previous"}
```

The API routes require `Authorization: Bearer <bridge token>`. Spotify playback
must already be available through a Spotify Connect device. The bridge does not
stream audio; the computer or another Spotify device remains the player.

The Spotify access token is intentionally supplied externally and is normally
short-lived. OAuth token acquisition and a `cliamp` adapter are separate follow-up
work; the current bridge provides a small, testable Spotify Connect milestone
without embedding credentials or player-specific shell commands.

## Safety

Bind to `127.0.0.1` unless the RM2 needs WLAN access. When using `0.0.0.0`,
restrict port `9120` to the private WLAN in the computer firewall. Do not expose
the bridge to the public internet.
