# Notifications

## Setup

Configure an authenticated HTTPS ntfy server, subscribe to a topic in the ntfy app, and add these Compose environment entries:

```yaml
environment:
    - NOTIFICATIONS_ENABLED=true
    - NOTIFICATION_INTERVAL=15s
    - NTFY_URL=https://ntfy.example.com
    - NTFY_TOPIC=reqarr
    - NTFY_TOKEN=replace-with-ntfy-token
```

Alternatively, leave `NTFY_TOKEN` empty and set both `NTFY_USERNAME` and `NTFY_PASSWORD`. Use one authentication method. Topics accept 1–64 letters, digits, underscores or hyphens.

Apply changed settings with `docker compose up -d --force-recreate reqarr`. For local development, set the same keys in `api/.env` and restart the API.

## Pushes

| Title | Message |
| --- | --- |
| Reqarr: New request | A new request was made. |
| Reqarr: Completed | Request fulfilled. |
| Reqarr: Needs attention | Request had a problem; see dashboard for details. |

Service problems use the same Needs attention title with a short service message. Routine approval, processing, downloading, importing and recovery stay on the dashboard. Messages contain no media titles, requester names or internal addresses.

Completion follows the dashboard's availability result:

- Radarr confirms an imported movie file.
- Sonarr confirms files for all known episodes in every requested season. Unrelated seasons do not count; missing season data cannot confirm completion.
- Seerr availability is the fallback when the relevant integration is not configured.

Available means imported into the managed library, independently of media-server indexing. Download completion or import history alone is insufficient. Seerr catching up afterward does not repeat the completion push.

Explicit failures, queue problems and movies continuously confirmed Missing for 10 minutes can trigger attention. Related evidence shares one active incident; additional evidence stays silent until positive progress permits a new incident. Uncertain data does not establish recovery. Sonarr issues can concern other episodes in the same series; details remain on the dashboard.

## Persistence and timing

The first successful poll silently baselines existing requests and incidents. Keep `/data/reqarr.db` persistent in Docker, or `data/reqarr.db` when running the API locally. `NOTIFICATION_DB` overrides the path. SQLite retains deduplication and pending deliveries across restarts; failed deliveries retry. A lost acceptance response can still cause duplicate delivery.

Monitoring runs without a browser. Notifications default to 15-second cycles and share upstream collection with the dashboard. Failed collections retain at least a 90-second cooldown; browser refresh remains 90 seconds. Polling can miss intermediate changes.

Failed availability checks retain the last confirmed dashboard status as Unconfirmed. That availability cache is in memory and resets on API restart; notification deduplication remains on disk. Older queued progress/recovery pushes are discarded on delivery.
