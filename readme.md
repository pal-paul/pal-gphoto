# PAL Google Photos Backup

A Go service for importing Google Photos selections into append-only,
account-specific folders on a NAS.

## Google API constraint

Since March 31, 2025, the Google Photos Library API cannot list a user's whole
library or arbitrary albums. Google only supports reading items explicitly
selected by the user through the Picker API. Consequently:

- OAuth authentication and multiple Google accounts are supported.
- A user starts a Picker session and selects photos, videos, or albums in Google Photos.
- The service polls that session and imports the selected media automatically.
- Fully unattended discovery of new library items is not possible with Google's public API.
- "From the beginning" means selecting the desired historical items in Picker.

## Backend requirements

- Store OAuth refresh tokens encrypted at rest.
- Keep each account under its own stable NAS directory.
- Filter imports by images, videos, or both.
- Track provider media IDs per account and download each item once.
- Never delete NAS files when an item disappears from Google Photos.
- Write downloads atomically so interrupted transfers do not leave final files.
- Resume pending Picker sessions after a restart.
- Expose health, account authorization, Picker session, and import status APIs.
- Run as a non-root Docker container with persistent data and media volumes.

## Implementation plan

1. Configuration, encrypted secrets, and SQLite state.
2. Append-only account storage and idempotent import service.
3. Google OAuth and Photos Picker API client.
4. HTTP API and background session polling.
5. Docker image, graceful shutdown, and operational documentation.

The backend follows the reference project's Go layout: composition roots in
`backend/cmd`, domain models in `backend/internal/domain`, interface-driven
services in `backend/internal/service`, adapters in `backend/internal/store`,
and Chi HTTP handlers in `backend/internal/httpapi`.

## Run locally

Create OAuth web credentials in Google Cloud, enable the Google Photos Picker
API, and register `http://localhost:8080/api/v1/google/callback` as a redirect
URI. Then configure and run the service:

```sh
cd backend
cp .env.gphoto.example .env.gphoto
openssl rand -base64 32
set -a; . ./.env.gphoto; set +a
go run ./cmd/gphoto
```

The generated key must be placed in `PAL_GPHOTO_TOKEN_KEY` and retained. Losing
it makes stored OAuth tokens unreadable.

For Docker, copy `install/.env.example` to a repository-level `.env`, replace
its account placeholders, then run:

```sh
docker compose up --build
```

The host `data` directory contains both SQLite state and account-specific media
folders. Back it up as a unit.

## Container CLI workflow

Set the account 1 placeholders in `.env`, then start the long-running API and
scheduler:

```sh
docker compose up -d --build
```

Generate the Google authorization URL from a second process in the running
container:

```sh
docker compose exec pal-gphoto pal-gphoto auth
```

Open the printed URL. Google redirects to the running container's callback,
which verifies the configured email and stores the encrypted refresh token in
`data/pal-gphoto.db`. Confirm the account and start a selection:

```sh
docker compose exec pal-gphoto pal-gphoto accounts
docker compose exec pal-gphoto pal-gphoto pick
```

Open the URL printed by `pick` and select media. The server imports the
selection in the background. Container restarts reuse the encrypted refresh
token and resume pending Picker sessions because `./data` is persistent.

## API workflow

1. `POST /api/v1/google/auth` with `{"name":"Personal"}`.
2. Open the returned `authorizationUrl` and complete Google consent.
3. `GET /api/v1/accounts` to obtain the account ID.
4. Optionally `PATCH /api/v1/accounts/{accountID}` with
   `{"mediaType":"images"}` or `{"mediaType":"videos"}`.
5. `POST /api/v1/accounts/{accountID}/picker-sessions`.
6. Open the returned `pickerUri` and select media or albums in Google Photos.
7. `GET /api/v1/picker-sessions` to monitor import status.

`GET /health` is available for container and NAS health checks.
