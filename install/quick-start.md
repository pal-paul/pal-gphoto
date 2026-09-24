# NAS quick start

This deployment runs one isolated container per Google account. Each account
has its own OAuth identity, encryption key, SQLite database, download history,
and NAS media directory. The second account is optional.

## Prerequisites

- A NAS with Docker Engine and Docker Compose v2.
- A writable directory for each account. The container runs as UID/GID `65532`.
- A Google Cloud OAuth web client with the Google Photos Picker API enabled.
- A browser-reachable callback URL for each account. Google normally requires
  HTTPS except for `localhost`, so use a NAS reverse proxy and hostname.

## Create Google OAuth credentials

1. Open [Google Cloud Console](https://console.cloud.google.com/).
2. Create or select a project.
3. Go to **APIs & Services > Library** and enable **Google Photos Picker API**.
4. Configure **Google Auth Platform / OAuth consent screen**:
   - For **Audience**, choose **Internal** only when every Google account belongs
     to the same Google Workspace organization. Internal apps are restricted to
     that organization and do not require verification.
   - Choose **External** for personal Gmail accounts or accounts outside your
     organization. The app starts in testing mode and only listed test users
     can authorize it; add every configured backup account as a test user.
     While publishing status is **Testing**, the app has a maximum of 100 test
     users before verification. Google counts this cap over the app's entire
     lifetime, including users later removed from the test-user list.
   - Add yourself as a test user.
   - Add the scope
     `https://www.googleapis.com/auth/photospicker.mediaitems.readonly`.
5. Go to **Credentials > Create credentials > OAuth client ID**.
6. Select **Web application**.
7. For local access, add this authorized redirect URI exactly:

   ```text
   http://localhost:8080/api/v1/google/callback
   ```

   For a NAS accessed from another computer, also add the exact HTTPS callback
   configured in `GOOGLE_ACCOUNT_1_REDIRECT_URL`. Add a separate callback for
   account 2 if it uses another hostname or port.
8. Copy the generated client ID and client secret into `install/.env`.

Google's public API cannot continuously scan an entire Photos library. After
authentication, you must use the Picker URL to select media or albums to
import.

## Configure accounts and storage

From the repository's `install` directory, generate one encryption key per
account:

```sh
cp .env.example .env
openssl rand -base64 32
openssl rand -base64 32
```

Edit `install/.env` and replace its placeholder values. Compose substitutes
them directly into each container; separate account env files are not needed.
Examples for Synology are shown, so use paths appropriate for your NAS:

```dotenv
GOOGLE_ACCOUNT_1_CLIENT_ID=<oauth-client-id>
GOOGLE_ACCOUNT_1_CLIENT_SECRET=<oauth-client-secret>
GOOGLE_ACCOUNT_1_EMAIL=first@example.com
GOOGLE_ACCOUNT_1_REDIRECT_URL=https://gphoto-first.example.com/api/v1/google/callback
PAL_GPHOTO_ACCOUNT_1_TOKEN_KEY=<first-output-from-openssl>
PAL_GPHOTO_ACCOUNT_1_DATA_DIR=/volume1/docker/pal-gphoto/account-1
PAL_GPHOTO_ACCOUNT_1_PORT=8080

GOOGLE_ACCOUNT_2_CLIENT_ID=<oauth-client-id>
GOOGLE_ACCOUNT_2_CLIENT_SECRET=<oauth-client-secret>
GOOGLE_ACCOUNT_2_EMAIL=second@example.com
GOOGLE_ACCOUNT_2_REDIRECT_URL=https://gphoto-second.example.com/api/v1/google/callback
PAL_GPHOTO_ACCOUNT_2_TOKEN_KEY=<second-output-from-openssl>
PAL_GPHOTO_ACCOUNT_2_DATA_DIR=/volume1/docker/pal-gphoto/account-2
PAL_GPHOTO_ACCOUNT_2_PORT=8081
```

Account 2 values may be omitted until its profile is used. Create each enabled
account's data directory and grant UID/GID `65532` read/write access. Every
directory contains its account's `pal-gphoto.db` and downloaded `media` tree.

## Configure account 1

Register the exact `GOOGLE_ACCOUNT_1_REDIRECT_URL` from `.env` in the Google
Cloud OAuth client. The reverse proxy hostname must forward to NAS port `8080`.

Start and authenticate the first account:

```sh
docker compose pull
docker compose up -d
docker compose exec pal-gphoto /usr/local/bin/pal-gphoto auth
```

Open the printed authorization URL. After Google redirects to the callback,
verify the saved account and start a Picker session:

```sh
docker compose exec pal-gphoto /usr/local/bin/pal-gphoto accounts
docker compose exec pal-gphoto /usr/local/bin/pal-gphoto pick
```

Open the Picker URL and select the photos, videos, or albums to import.

## Add account 2

Set all account 2 placeholders in `install/.env`. Register its callback in
Google Cloud and forward its reverse proxy hostname to NAS port `8081`. The
OAuth client ID and secret may be shared, but account data and token keys remain
separate.

Start the optional profile and authenticate it:

```sh
docker compose --profile account-2 pull
docker compose --profile account-2 up -d
docker compose exec pal-gphoto-account-2 /usr/local/bin/pal-gphoto auth
docker compose exec pal-gphoto-account-2 /usr/local/bin/pal-gphoto accounts
docker compose exec pal-gphoto-account-2 /usr/local/bin/pal-gphoto pick
```

For additional accounts, duplicate the account-2 service with a unique service
name and a new set of numbered placeholders, host port, data directory,
callback URL, and encryption key.

## Operations

```sh
docker compose ps
docker compose logs -f pal-gphoto
docker compose --profile account-2 logs -f pal-gphoto-account-2
docker compose --profile account-2 pull
docker compose --profile account-2 up -d
```

`PAL_GPHOTO_VERSION` in `install/.env` pins the deployment to a release. Change
it to a newer version, then run `docker compose pull` and `docker compose up -d`
to upgrade. Do not use `latest` when repeatable NAS deployments are required.

Back up each account's complete data directory and `install/.env` securely. A
container restart reuses the encrypted refresh token from SQLite. Losing or
changing an account's `PAL_GPHOTO_ACCOUNT_*_TOKEN_KEY` makes that token
unreadable and requires fresh authorization. The service never deletes NAS
media based on Google Photos state.
