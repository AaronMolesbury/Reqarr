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
| Reqarr: Completed | A film was imported. / A TV episode was imported. |
| Reqarr: Needs attention | Request had a problem; see dashboard for details. |

Service problems use the same Needs attention title with a short service message. Routine approval, processing, download progress and recovery stay on the dashboard. Messages contain no media titles, requester names or internal addresses.

Completion pushes watch successful Radarr/Sonarr import history across the configured libraries, including content added directly rather than requested through Seerr:

- One push for the first imported film.
- One push for each first imported TV episode, even while its season is partially available.
- Quality upgrades and subsequent reimports of known content stay silent, including after restarts.

Seerr completion does not add another push for the same content when import monitoring is configured. If the relevant Radarr/Sonarr integration is not configured, Seerr availability remains the request-completion fallback.

The dashboard stays focused on Seerr requests and uses current imported-file evidence for Available. An import push reports an import event; it does not prove that the file still exists or that a media server has indexed it. A grab or completed download is not an import. Existing missing-content attention rules remain request-focused.

Explicit failures, queue problems and movies continuously confirmed Missing for 10 minutes can trigger attention. Related evidence shares one active incident; additional evidence stays silent until positive progress permits a new incident. Uncertain data does not establish recovery.

Partial TV availability alone is silent. A requested episode must have aired at least 24 hours ago, explicitly have no imported file, and remain without a matching active download/import for 10 continuous confirmed minutes before raising Needs attention. Timers are per episode and persist in SQLite; missing episodes share one active notification incident per request. The dashboard lists the affected season/episode numbers. Importing the affected files or confirmed matching progress clears their timers, even if the series remains partially available.

Failed lookups, capped queues, unknown air dates/file flags and ambiguous queue matches cannot start an incident. Unconfirmed observations or gaps over three minutes break a pending timer. Already raised incidents remain unresolved until positive evidence returns. Generic Sonarr queue/history issues can concern other episodes in the series; the timed missing check is limited to requested seasons.

## Persistence and timing

The first successful request poll silently baselines existing requests and incidents. Each import source also establishes its own silent baseline of existing files and recent imports. Sonarr reads its episode lists for that initial baseline; afterward, episode identities are reused and lists are fetched for new history references as needed. Failed initial checks do not establish a baseline.

Keep `/data/reqarr.db` persistent in Docker, or `data/reqarr.db` when running the API locally. `NOTIFICATION_DB` overrides the path. SQLite retains imported-content identities, history checkpoints, deduplication and pending deliveries across restarts; failed deliveries retry. A lost acceptance response can still cause duplicate delivery. Removing the database creates fresh silent baselines.

Monitoring runs without a browser. Notifications default to 15-second cycles and reuse the dashboard's library/history responses when available. Import checks can continue when Seerr is unavailable; failed import sources wait at least 90 seconds before retrying. Browser refresh remains 90 seconds. Import history is bounded to 500 recent records per source, so long outages or very busy libraries can miss events. Imports that occur during initial baselining are not replayed as a backlog.

Failed availability checks retain the last confirmed dashboard status as Unconfirmed. That availability cache is in memory and resets on API restart; notification deduplication remains on disk. Older queued progress/recovery pushes are discarded on delivery.
