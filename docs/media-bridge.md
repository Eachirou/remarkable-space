# WLAN media bridge

The RM2 does not play audio. The `podcast` and `radio` apps are remote controls;
audio is played on the computer over WLAN.

Build and run the bridge:

```bash
go build -o /tmp/media-bridge ./cmd/media-bridge
export MEDIA_BRIDGE_TOKEN="$(openssl rand -hex 24)"
export PODCAST_FEEDS="Example=https://example.com/feed.xml"
export RADIO_STATIONS="Example=https://example.com/stream.mp3"
/tmp/media-bridge -addr 0.0.0.0:9130
```

`mpv` must be installed on the computer. The bridge accepts only HTTP(S) media
URLs and requires the bearer token for every media request. Bind the port to
the private WLAN only and do not expose it to the public internet.

Configure the tablet apps with:

```text
Bridge URL: http://<computer-wlan-address>:9130
Bridge token: <MEDIA_BRIDGE_TOKEN>
```

The podcast app lists the configured RSS feeds and fetches their episodes.
The radio app lists the configured stream URLs. Selecting an item starts or
replaces the computer-side `mpv` process. The tablet never receives or stores
the audio stream.
