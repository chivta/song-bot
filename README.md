# songbot

A Telegram bot that turns a song name or a YouTube link into an audio message,
tagged with the title, artist and cover art.

## How it works

```
bot ──▶ downloader ──▶ youtube (yt-dlp + ffmpeg)
 ▲           │
 │           └──▶ storage (SQLite: video ID → Telegram file ID)
 └── delivery: audio message with cover, title, artist
```

- `cmd/songbot` — wiring and graceful shutdown
- `internal/domain` — types and sentinel errors, no dependencies
- `internal/bot` — Telegram delivery: handlers, progress messages, every
  user-facing string
- `internal/downloader` — business logic: queue, workers, rate limits, match
  ranking
- `internal/youtube` — yt-dlp driver: search, metadata, download, cover art
- `internal/storage` — SQLite state, migrations embedded and applied on startup
- `internal/health`, `internal/metrics`, `internal/logging`, `internal/config`

A request is answered immediately with a placeholder message and queued. Workers
edit that message as the job progresses (searching → downloading with a percent →
sending), then delete it and post the finished track.

### Picking a match

A link is resolved directly. Free text is searched, and the results are ranked:
YouTube's own ordering dominates, a plausible single length is rewarded, and
titles carrying "live", "cover", "1 hour", "nightcore" and similar are demoted —
unless the user asked for exactly that, in which case they are not.

Metadata is always fetched before any transfer starts, so a live stream or an
over-long video is refused without downloading anything.

### Not downloading the same song twice

Every delivered track's Telegram file ID is stored against its video ID. When
anyone asks for that song again the bot re-sends it by ID: one API call, no
yt-dlp, no transcode, no transfer. Telegram file IDs do not expire.

## Rate limiting

Three independent limits, because they protect three different things:

| Limit | Set by | Protects |
| --- | --- | --- |
| Per-user token bucket | `USER_RATE_PER_HOUR`, `USER_BURST` | Other users' share of the bot |
| Bounded queue + worker pool | `QUEUE_SIZE`, `WORKERS` | The host's CPU and disk |
| Global outbound send limiter | fixed at 25/s | The bot's standing with Telegram |

Nothing blocks: a user over budget or a full queue is rejected immediately and
told so. Because downloads run on a fixed pool rather than inline, one person
queueing an album never delays anyone else's request — their jobs simply take
their turn.

## Configuration

Copy `.example.env` to `.env` and fill it in. Every variable is documented there.
The process exits immediately if anything required is missing or invalid.

## Running

```sh
go run ./cmd/songbot           # local
docker compose up              # local, hot reload via air
docker build --target production -t songbot .
```

`GET /health` returns 204, `GET /metrics` exposes counters in Prometheus format.

## yt-dlp and ffmpeg

The two are handled differently, on purpose.

**yt-dlp is downloaded at startup** into `YTDLP_DIR` by
[`go-ytdlp`](https://github.com/lrstanley/go-ytdlp), which verifies it against
the release's signed SHA-256 sums. That directory sits on the persistent volume,
so a restart reuses it. YouTube breaks extractors far more often than this bot
changes, and a yt-dlp bump should not require an app release.

**ffmpeg comes from the image** (`apk add ffmpeg`) and is resolved from `PATH`.
It cannot be downloaded: go-ytdlp fetches BtbN's builds, which are glibc-linked
with no musl variant published, so on Alpine they fail to exec with
`exit status 127`. Alpine's own build is musl-native and works. ffmpeg is a
stable dependency that does not need chasing, so baking it in costs nothing.

Startup deletes any ffmpeg or ffprobe in the managed cache that fails to run.
That matters twice over: go-ytdlp resolves its cache ahead of `PATH` and
hard-fails on a binary it cannot execute, and it prepends that cache directory
to `PATH` when invoking yt-dlp, where a broken binary would shadow the working
system one during post-processing. A volume carrying a bad copy heals itself on
the next start.

The version go-ytdlp installs is the one it was built against, and its checksum
verification is tied to that version — so `YTDLP_VERSION` is not a knob that
could work. To run a different build, put the binary somewhere readable and
point `YTDLP_PATH` at it; the app then uses it verbatim and skips the managed
install. The routine upgrade path is bumping `github.com/lrstanley/go-ytdlp`.

The production image is Alpine rather than distroless because ffmpeg and a
shell-based healthcheck both need a real userland.

The probe server starts before any of this, so a first start on a cold volume —
which downloads ~37 MB of yt-dlp — answers `/health` throughout instead of being
killed by the liveness probe. A `startupProbe` allows five minutes for it, and a
download that times out is retried in process rather than crashing the pod.

## Deploying

Manifests live in `k8s/` and are the ground truth for what runs in the cluster.
Flux reconciles them — nothing is applied by hand.

### Secrets

`BOT_TOKEN` and `ALLOWED_USERS` live in `k8s/secrets.yaml`, which is
**gitignored**, and are committed only in SOPS-encrypted form as
`k8s/secrets.enc.yaml` — the same layout instalker uses. Encryption targets the
shared `ruscan` age recipient, so Flux decrypts it in-cluster with the existing
`ruscan-sops-age` secret, plus the local dev key so the file can be edited
without the cluster.

```sh
sops -d k8s/secrets.enc.yaml > k8s/secrets.yaml
$EDITOR k8s/secrets.yaml
sops -e k8s/secrets.yaml > k8s/secrets.enc.yaml
```

The registry pull secret is the one thing SOPS does not cover, since it is a
cluster-level docker config rather than app config:

```sh
cp .env.secrets.example .env.secrets   # GHCR credentials only
./create-secrets.sh                    # creates the namespace and ghcr-secret
```

The PAT needs `read:packages`. If image pulls fail with a 403 from
`ghcr.io/token`, that token has expired — it is the usual cause.

The SQLite file and the binary cache share a 5Gi `ReadWriteOnce` PVC at
`/app/data`. Because that volume cannot be attached twice, the Deployment uses
the `Recreate` strategy and stays at one replica. The container runs as UID
10001 with a read-only root filesystem and all capabilities dropped; `/tmp` is a
2Gi `emptyDir`, which is where in-flight downloads land before being deleted.

`terminationGracePeriodSeconds` is 60. A download still running when the pod is
told to stop is abandoned rather than waited out — the user just asks again, and
the second attempt is usually a cache hit anyway.

### Pipelines

- **CI** (`.github/workflows/ci.yaml`) — gofmt check, vet, test, build.
- **CD** (`.github/workflows/cd.yaml`) — runs only after CI succeeds on `main`.
  Builds the `production` stage, pushes it to GHCR tagged with the full commit
  SHA (and `latest`), then rewrites the image tag in `k8s/bot/deployment.yaml`
  and commits it as `deploy: songbot <sha>`.

## The chat must message the bot first

Telegram forbids bots from opening a conversation, so each allowed user sends
`/start` themselves once.

`ALLOWED_USERS` is set, so the bot is private: only the Telegram user IDs listed
in `k8s/secrets.yaml` are answered and everyone else is turned away. Clearing
it would make the bot public.

## Notes

- SQLite (pure-Go `modernc.org/sqlite`) is used instead of Postgres: the state is
  a single cache table, and a one-binary deploy with no database server is worth
  more here than the shared convention.
- `MAX_FILE_SIZE_MB` cannot exceed 50 — that is Telegram's Bot API upload
  ceiling, and there is no way around it short of a local Bot API server.
- Cover art is cropped to a 320×320 square with ffmpeg before being attached.
  YouTube stills are 16:9, and music players expect album-art proportions.
- Downloading from YouTube is subject to YouTube's Terms of Service. Run this
  against content you have the right to download.
