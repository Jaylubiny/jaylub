## Jaylub

This repository is representing the source code behind website https://jaylub.com it is small website for starting company named jaylubiny.
Jaylubiny is not claimed as official compay yet and probably will not be for the next following 3-4 years couse of age restrictions of the employees.
We welcome any support or feedback on support@jaylub.com.


This readme specifically is for the creator of this website (preclik02) for notes of future updates or existing and yet comming routes or sub-domains of this webiste.
If you have any idea on update you can create issue on github or email us on support@jaylub.com (the response time should be within one day including weekends and holidays).







## ROUTES && SUB-DOMAINS

.com/pages/portfolio -> actually good portfolio

LATER ^

.com/updates

WORKING ON ^

/
/login
/logout
/me
/settings
/contacts
/docs
/documentation
/game/test
/game/jaylive
/game/jaylive/state
/game/jaylive/start
/game/jaylive/shop
/game/jaylive/character
/game/jaylive/run
/game/jaylive/leaderboard
/typing
/api/typing/score
/api/typing/leaderboard
/api/typing/user-stats
/wiki
/wiki/C
/wiki/jaylub
/pages
/pages/jayware
/pages/jayware/download
/pages/jayos
/pages/nullc
/pages/allah
/pages/jaylub
/pages/services
/discord
/discord/allah/callback
/.well-known/discord
/chat
/chat/messages
/chat/send
/chat/files/<attachment-id>
/static/<file>
/web/static/<file>
/api/emails
/api/send

DONE ^

<<<<<<< HEAD
=======
## WEBMAIL

The authenticated webmail application is served by the dedicated listener on
port `9010` (normally published as `email.jaylub.com`). Each account's mailbox
address is `<username>@jaylub.com`. Inbox, Sent, and Trash are scoped to the
signed-in account; trashed messages can be restored or permanently deleted.

Configure `RESEND_API_KEY` to enable outgoing mail and `WEBHOOK_SECRET` for
incoming-mail delivery. The incoming webhook accepts `POST
/api/webhooks/incoming` with an `X-Secret-Auth` header and JSON fields `from`,
`to`, `subject`, and `text`. Mailbox data is stored in
`internal/database/emails.db`; mailbox-state columns are added without
replacing existing email rows. Messages in all folders (Inbox, Sent, and Trash)
are permanently deleted once their stored timestamp is more than 29 days old.
Expired messages are purged at startup and checked again every 24 hours.

For reverse-proxy deployments, set `SESSION_COOKIE_DOMAIN=.jaylub.com` so the
login and device-terms cookies are shared by the main site and email subdomain.
The application also recognizes the `X-Forwarded-Host` header and falls back
to `MY_DOMAIN` when the proxy rewrites the request host.

## TYPING TEST

The signed-in typing test is available at `/typing`. It generates a new random
passage of lowercase words without commas or periods for every test. Type
directly in the highlighted passage; more text is appended as you approach the
end, so the test does not run out of text before its timer. Press `Tab` or
`Escape` to restart the current mode. It supports `15s`, `30s`, and `60s` modes.
Text size can be adjusted from 1 to 25 px at `/typing/settings`; the preference
is saved in the current browser. Scores are stored in the shared
`internal/database/users.db` SQLite database.
Startup creates the
`typing_scores` table, its user foreign key, and leaderboard/personal-best
indexes without replacing existing data; the idempotent schema is in
[`internal/typing/schema.sql`](internal/typing/schema.sql).

`POST /api/typing/score` accepts `{ "wpm": 70, "accuracy": 98.5, "mode": "30s" }`
and requires a signed-in user. `GET /api/typing/user-stats?mode=30s` returns
the signed-in user's best WPM, average accuracy, and test count for that mode.
Unauthenticated score and stats requests receive JSON `401` responses. The public
`GET /api/typing/leaderboard?mode=30s` returns the top 20 users, ranked using
each user's best score for that mode. The server enforces score bounds and
allowed modes, but client-submitted timing metrics are not cryptographic proof
of a test and should not be treated as cheat-proof.

## CLEARING THE MUSIC LIBRARY

To clear all song rows from `internal/database/music.db` and remove MP3s from
`data/mp3s/` on the hosting machine, first stop the Jaylub service and run the
cleanup utility from the project checkout:

```sh
python3 tools/clear_music_library.py
python3 tools/clear_music_library.py --apply
```

The first command is a read-only preview. The second requires typing the
explicit confirmation phrase. It creates a timestamped archive under `data/`
with a consistent SQLite backup and moves MP3 files there rather than
permanently deleting them. Keep the archive until the cleared library has been
verified. Only MP3 files directly inside `data/mp3s/` are moved; other files
and subdirectories are left untouched. The script deletes song rows, allowing
SQLite foreign-key cascades to remove song favorites and playlist-track
references, while leaving accounts and playlists intact.

## RUNNING UNDER SYSTEMD

From the repository root, run `./run.sh`. It loads
`internal/database/.env` when present, builds the Jaylive client and Go server,
then runs the server in the foreground. Go and `DISCORD_BOT_TOKEN` are required;
the token can be supplied by the environment or the env file. Keep the env file
private (`run.sh` sets its mode to `0600`) and do not put it in Git.

The server listens on ports `8080` (main site), `8090` (company site), `9000`
(music), and `9010` (email). Keep the process under a service manager in
production; do not start a second copy while these ports are in use. For
systemd, let the unit run `./run.sh` in the foreground so systemd can collect
logs and stop the process with SIGTERM.

Startup uses the existing SQLite files in `internal/database/` and applies
idempotent schema additions/migrations in place; it does not recreate or seed
the user, email, or music databases. Existing music tracks, playlists,
favorites, account records, game profiles, chat history, and typing scores are
retained by their migrations. Back up `users.db`, `emails.db`, and `music.db`
consistently before deploying schema changes. The email service also deletes
mail older than its documented 29-day retention period at startup.

To run the unit and integration tests separately, use `go test ./...`.
`./stop.sh <systemd-unit>` stops a managed service.

>>>>>>> 410d938 (wpm test)
## UPDATES

add bioms and maps with stronger or weaker emeies (and in future custom enemies for every biom)

optimize the fuck out of this

bulvy jaylub that shoots 2 lazers

better chat experience and more secure chat

TowerTrack - run tracker for the game The Tower with parsed info into spreadsheet

bot sending message when an update is pushed to prod (on command by the owner - preclik02)

make dashboard on localhost port that is not published to the public
