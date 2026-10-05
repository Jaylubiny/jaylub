# Self-hosted music subsystem

The main Jaylub server listens for the music app on port `9000` on all network
interfaces and uses the existing site session and terms middleware. Keep it
behind a Cloudflare Tunnel; use a host firewall to block direct public access
to port 9000. Unauthenticated users are sent to the main site's login page.

## Production deployment

1. Build and run from the repository root using `./run.sh`. It loads
   `internal/database/.env`, builds the server (including the embedded UI),
   and starts listeners on ports 8080, 8090, and 9000. The music listener is
   reachable on all interfaces; firewall it from direct public access.
2. Add this ingress rule to the existing Cloudflare Tunnel configuration
   before its final catch-all rule; preserve all the other site's ingress
   rules:

   ```yaml
   - hostname: music.jaylub.com
     service: http://127.0.0.1:9000
   ```

3. Ensure the runtime account can read `internal/database/.env` and write
   `internal/database/`, `data/mp3s/`, and `.bin/`. Restrict the environment
   file to the service account (for example, `chmod 600
   internal/database/.env`). Never put its contents in Git or deployment logs.
   `run.sh` enforces mode `0600` before loading the file, so the service must
   run as its owner. The account and music database files are also restricted
   to mode `0600` by their respective startup initializers.
   `DISCORD_BOT_TOKEN` is required by the overall server startup.
4. Build and restart the service after changing Go or embedded UI files. A
   plain browser refresh cannot update files embedded in the currently running
   binary. Keep the process under the existing service manager and let
   `run.sh` run in the foreground.
5. Back up `internal/database/users.db`, `internal/database/music.db`, and
   `data/mp3s/` together. Use SQLite's online backup mechanism (or stop the
   service for a filesystem snapshot) so each database backup is consistent;
   restore the databases and upload directory as one matching set.

Port 9000 has bounded header, body-read, and idle timeouts, but no response
write timeout because a fixed response deadline would interrupt long audio
streams. Audio is streamed from local storage; do not put a response-buffering
proxy in front of the tunnel route.

## Storage

- SQLite database: `internal/database/music.db`
- MP3 uploads: `data/mp3s/`
- Both paths are excluded from Git.

On first startup, the server creates the database directory, initializes both
`users.db` and `music.db` with their schemas, and creates the MP3 upload
directory when the first upload is made. Neither database needs to be
pre-created. Startup creates empty account tables; it does not seed an admin
account or create sample tracks.

For a new installation, build and run the admin tool from the repository root
to create the first account:

```sh
go build admin.go
./admin
```

Choose **Add user**. Keep the generated `admin` executable private and remove
it after account setup if it is not needed for ongoing maintenance.

The database mirrors authenticated user IDs into its local `users` table to
enforce foreign keys without exposing or modifying the main site's database
schema. It stores song metadata, favorites, and user-owned playlists separately.

## API

- `GET /api/songs` — global library with the current user's favorite state
- `GET /api/favorites` — current user's favorite playlist
- `POST /api/favorites` — JSON body `{"song_id":"<uuid>"}`
- `DELETE /api/favorites/{id}` — remove the current user's favorite
- `GET /api/playlists` — list the current user's playlists
- `POST /api/playlists` — create a playlist with JSON body `{"name":"<name>"}`
- `GET /api/playlists/{id}` — get a playlist and its tracks
- `DELETE /api/playlists/{id}` — delete the current user's playlist
- `POST /api/playlists/{id}/songs` — add a track with JSON body `{"song_id":"<uuid>"}`
- `DELETE /api/playlists/{id}/songs/{songID}` — remove a track from a playlist
- `POST /api/upload` — one MP3 in multipart field `file`, 100 MiB maximum per file
- `GET /api/stream?id=<uuid>` — MP3 stream with HTTP range/seek support

The browser app is embedded into the Go binary. Tab changes do not replace its
audio element, so playback continues while switching between library,
favorites, playlists, and upload views. Users can create multiple private
playlists, add tracks from the library or favorites, play a playlist, remove
tracks, and delete playlists; favorites remain available separately.
The global library can be searched by track title, artist, album, or ID.
The upload form accepts up to 25 selected MP3s and sends them individually, so
each file retains the 100 MiB limit without requiring one oversized batch
request.

## Install as a mobile app

The music page includes a web app manifest, home-screen icons, and a service
worker. Serve it over HTTPS (the Cloudflare Tunnel should terminate TLS) and
open `music.jaylub.com` in the device browser. Android browsers that support
installation offer it from their browser UI. On iPhone/iPad, use Safari's Share
menu and choose **Add to Home Screen**. Offline mode provides an offline
message page; it deliberately does not cache account pages, APIs, or audio.

`MockAuthMiddleware` supplies a fixed development user and can be used in
local-only handlers/tests. It is intentionally insecure and is never mounted
by the production server; production adapts the website's real session and
terms middleware.
