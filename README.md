# Reqarr

Reqarr is a dashboard that lets you monitor requests status' from *arr services. Supports push notifications through ntfy.

NOTE: despite being public, this is just something I've built for my own setup and shared in case it’s useful to others. Support is limited/best-effort, and features follow my personal needs.

<img width="1444" height="1177" alt="image" src="https://github.com/user-attachments/assets/6bd6da66-5812-4da1-83f7-d2995ca0fc40" />

## Docker setup

Reqarr serves the dashboard and API from one container on port `8080`.

For a published image, save this as `compose.yaml`. Replace `DOCKERHUB_ACCOUNT` with the published image account and fill in the environment values directly:

```yaml
services:
    reqarr:
        image: DOCKERHUB_ACCOUNT/reqarr:latest
        container_name: reqarr
        restart: unless-stopped
        environment:
            - SEERR_URL=https://seerr.example.com
            - SEERR_API_KEY=replace-with-seerr-api-key
            # Optional: set both URL and key, or leave both empty.
            - RADARR_URL=
            - RADARR_API_KEY=
            - SONARR_URL=
            - SONARR_API_KEY=
            - NOTIFICATIONS_ENABLED=false
            - NOTIFICATION_INTERVAL=15s
            - NTFY_URL=https://ntfy.example.com
            - NTFY_TOPIC=reqarr
            # Use a token OR username/password, never both.
            - NTFY_TOKEN=
            - NTFY_USERNAME=
            - NTFY_PASSWORD=
        ports:
            - "127.0.0.1:8080:8080"
        volumes:
            - ./reqarr:/data
        security_opt:
            - no-new-privileges:true
        cap_drop:
            - ALL
```

No separate settings file or top-level volume declaration is needed. Leave unused optional URL/key pairs empty. Prepare the writable data directory beside the Compose file, then start:

```sh
mkdir -p reqarr
sudo chown 65532:65532 reqarr
docker compose pull reqarr
docker compose up -d reqarr
```

The image must be published before pulling it; see [image publishing](docs/DOCKER.md#publish-an-image).

Seerr requires its URL and API key. Radarr and Sonarr are optional; each requires both its URL and key. Availability comes from imported Radarr files and the requested Sonarr seasons. Seerr supplies requests and approvals, and acts as the availability fallback when the relevant integration is not configured. Available means imported into the managed library, independently of media-server indexing.

Failed availability checks preserve the last confirmed status with an Unconfirmed marker. Incomplete season data cannot establish TV completion.

Open `http://localhost:8080` on the Docker host. The template binds to host loopback; protect access if exposing it elsewhere. Reqarr has no built-in login.

Credentials are passed at runtime and remain server-side. The `./reqarr` directory persists notification state at `/data/reqarr.db`; retain it across upgrades.

See [Docker deployment](docs/DOCKER.md) for building from source, updates, storage and ports.

## Notification setup

Configure an authenticated ntfy server and topic, then update the Compose environment entries:

```yaml
environment:
    - NOTIFICATIONS_ENABLED=true
    - NOTIFICATION_INTERVAL=15s
    - NTFY_URL=https://ntfy.example.com
    - NTFY_TOPIC=reqarr
    - NTFY_TOKEN=replace-with-ntfy-token
```

Alternatively, leave `NTFY_TOKEN` empty and set both `NTFY_USERNAME` and `NTFY_PASSWORD`. Subscribe to the same server and topic in the ntfy phone app.

Recreate the container to apply settings changes:

```sh
docker compose up -d --force-recreate reqarr
```

The first successful poll establishes a silent baseline. Pushes cover new requests, fulfilled requests and issues needing attention, even with the dashboard closed. Detailed progress stays on the dashboard. Keep the database to preserve the baseline and incident deduplication. Notifications are disabled by default.

See [notification configuration](docs/NOTIFICATIONS.md) for local settings and delivery behavior.

## Local API and client setup

See [local development](docs/DEVELOPMENT.md) for prerequisites, configuration, API/client commands and tests.

## Troubleshooting

### Could not open notification database

With the `./reqarr:/data` bind mount, the host directory and any existing database must be writable by the container's UID/GID `65532`. From the directory containing the Compose file, run:

```sh
docker compose stop reqarr
mkdir -p ./reqarr
sudo chown -R 65532:65532 ./reqarr
sudo chmod -R u+rwX ./reqarr
docker compose up -d reqarr
docker compose logs --tail=20 reqarr
```

These commands preserve the database and update its ownership and permissions. If the bind mount uses a different host path, apply them to that directory instead.

Leave `NOTIFICATION_DB` unset to use the image default, or set it to the container path:

```yaml
environment:
    - NOTIFICATION_DB=/data/reqarr.db
```

A local-development path such as `data/reqarr.db` does not point to the mounted `/data` directory. If startup still fails, check available disk space and confirm the mount is writable.
