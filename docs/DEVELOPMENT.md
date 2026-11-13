# Local development

## Prerequisites

- Go 1.27, matching `api/go.mod`.
- Node.js 22 and npm, matching the container build.
- Network access to configured services, including any required VPN connection.

## API

Create `api/.env` using [the settings template](../api/.env.example). Set `SEERR_URL` and `SEERR_API_KEY`. Optional Radarr/Sonarr integrations each require a URL and API key.

From the repository root:

```sh
cd api
go run .
```

The API binds to `127.0.0.1:8080` by default. It loads `.env` from the working directory at startup; existing process environment variables take precedence, including empty values. Restart after changing settings. File values are literal, optionally quoted; full-line comments are supported, shell expansion and inline comments are not.

| Endpoint | Purpose |
| --- | --- |
| `GET /api/health` | HTTP process health |
| `GET /api/dashboard` | Combined request data |
| `GET /api/requests` | Raw Seerr response for private debugging |

Keep these endpoints private: responses can contain requester information. API keys remain server-side.

## Client

In a second terminal, from the repository root:

```sh
cd client
npm ci
npm run dev
```

On Windows PowerShell, use `npm.cmd` if script execution policy blocks npm. Open the URL printed by Vite, normally `http://localhost:5173`. Vite proxies `/api` to `127.0.0.1:8080`.

The dashboard refreshes every 90 seconds while visible. Refresh failures preserve the last successful snapshot; retries wait 5 seconds, then 10 seconds, and stop after three consecutive failures until manual Refresh.

Seerr supplies requests, approvals and requested seasons. Radarr's explicit imported-file flag determines movie availability; Sonarr per-season file totals determine availability for the requested seasons, without counting files from unrelated seasons. Seerr availability is a fallback when the relevant integration is not configured. Missing or failed checks preserve the last confirmed status in memory and mark it Unconfirmed. Available describes the managed library rather than media-server indexing; details show the evidence source and original Seerr status.

## Checks and production preview

From `api/`:

```sh
go test ./...
```

From `client/`:

```sh
npm test
npm run build
```

To serve the built client through Go locally, set `STATIC_DIR=../client/dist` when running the API from `api/`. Leave it unset for separate Vite development. Container deployment sets the static directory automatically.

Tests use mock HTTP services, controlled clocks and temporary SQLite databases. They do not establish live integration or container compatibility.

