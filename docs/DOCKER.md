# Docker

The root image runs the client and API together on port `8080`. Start with the [Compose template](../README.md#docker-setup), replace the image placeholder and fill in the environment values. No separate settings file is required.

## Build from source

With Docker installed, run from the repository root. Edit the included `compose.yaml` first:

```sh
mkdir -p reqarr
sudo chown 65532:65532 reqarr
docker compose up -d --build
```

Credentials are supplied at runtime, not included in the image. Keep populated Compose files private.

## Publish an image

Create a `reqarr` repository on Docker Hub, then replace `DOCKERHUB_ACCOUNT` below:

```sh
docker login
docker build -t DOCKERHUB_ACCOUNT/reqarr:latest .
docker push DOCKERHUB_ACCOUNT/reqarr:latest
```

Build for the deployment machine's CPU architecture. To target AMD64 explicitly, replace the build and push commands with:

```sh
docker buildx build --platform linux/amd64 -t DOCKERHUB_ACCOUNT/reqarr:latest --push .
```

For a published image, remove `build: .` from the included Compose service and set `image: DOCKERHUB_ACCOUNT/reqarr:latest`. To update a deployment:

```sh
docker compose pull reqarr
docker compose up -d reqarr
```

## Storage

The `./reqarr:/data` mount stores notification state in `./reqarr/reqarr.db`. The directory must be writable by UID/GID `65532`. Retain it across updates; deleting the database resets the notification baseline.

To migrate existing state, stop the old API before copying the database and any remaining SQLite WAL/SHM sidecars into this directory. Set ownership to `65532:65532` and run only one instance against that database.

## Connectivity and ports

The supplied port mapping, `127.0.0.1:8080:8080`, allows access from the Docker host at `http://localhost:8080`. It does not expose the dashboard to the network. Protect dashboard and API access if changing that binding or adding a proxy; Reqarr has no built-in login.

The configured service URLs must be reachable from inside the container. Use internal service addresses where appropriate. `localhost` refers to the container's own network namespace, not the Docker host or an unrelated container.

To change only the host port, use a mapping such as `127.0.0.1:8095:8080`. The internal port and health check stay unchanged. If changing `HTTP_ADDR` itself, update the mapping's container port and override the health check to match.

## Logs and health

```sh
docker compose logs --tail=30 reqarr
```

The image health check calls `/api/health`. It checks Reqarr's HTTP process, not upstream service health. See [troubleshooting](../README.md#troubleshooting) for database startup errors.
