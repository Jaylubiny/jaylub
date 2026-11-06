# Production deployment

## What is included

- `cloudflared-config.yml.example` routes the public site, company site, email,
  and music hostnames to the app's local listeners.
- `jaylub.service` runs the prebuilt app as an unprivileged `jaylub` service
  account and restricts writes to the app's data paths.
- `jaylub.env.example` lists runtime environment variables. Copy it to
  `internal/database/.env`; never commit the populated file.

The app requires `DISCORD_BOT_TOKEN` to start. Set `RESEND_API_KEY` and
`WEBHOOK_SECRET` to enable Resend email. For the inbound `email.received`
webhook, configure Resend to call
`https://email.jaylub.com/api/webhooks/incoming`. Set `WEBHOOK_SECRET` to the
signing secret shown for that webhook. Receiving also requires enabling the
domain in Resend and publishing its verified MX record in DNS.

## Host setup

1. Install Go 1.24.4 or newer, `cloudflared`, and `systemd` on the Linux host.
   The host must have outbound HTTPS access for the Cloudflare Tunnel, Resend,
   Discord, and Go module downloads during builds.
2. Create the service account if it does not already exist:

   ```sh
   sudo useradd --system --home-dir /opt/jaylub --no-create-home \
     --shell /usr/sbin/nologin jaylub
   ```

   Install a clean checkout at `/opt/jaylub`.
   Set the checkout's owner to `jaylub:jaylub`, and create the runtime
   directories before starting the systemd unit:

   ```sh
   sudo install -d -o jaylub -g jaylub \
     /opt/jaylub/internal/database \
     /opt/jaylub/internal/db \
     /opt/jaylub/internal/database/email-attachments \
     /opt/jaylub/data/mp3s
   sudo chown jaylub:jaylub /opt/jaylub/internal/services/users.txt
   ```
3. Copy `deploy/jaylub.env.example` to
   `/opt/jaylub/internal/database/.env`, fill in required values, and restrict
   it to the service account:

   ```sh
   sudo chown jaylub:jaylub /opt/jaylub/internal/database/.env
   sudo chmod 600 /opt/jaylub/internal/database/.env
   ```

4. Build the production binary as the service account from the repository root.
   This also generates the obfuscated Jaylive browser script consumed by the
   site:

   ```sh
   sudo -u jaylub -H sh -c 'cd /opt/jaylub && go run tools/obfuscate_jaylive.go && go build -o .bin/jaylub-server ./cmd/server'
   ```

5. Install the service unit and start it:

   ```sh
   sudo cp /opt/jaylub/deploy/jaylub.service /etc/systemd/system/jaylub.service
   sudo systemctl daemon-reload
   sudo systemctl enable --now jaylub
   sudo systemctl status jaylub
   ```

6. Authenticate and create a named Cloudflare Tunnel, then add all five
   hostnames to its DNS routing:

   ```sh
   cloudflared tunnel login
   cloudflared tunnel create jaylub
   cloudflared tunnel route dns jaylub jaylub.com
   cloudflared tunnel route dns jaylub www.jaylub.com
   cloudflared tunnel route dns jaylub company.jaylub.com
   cloudflared tunnel route dns jaylub email.jaylub.com
   cloudflared tunnel route dns jaylub music.jaylub.com
   ```

   Copy `deploy/cloudflared-config.yml.example` to
   `/etc/cloudflared/config.yml`, replace the tunnel UUID and credentials-file
   path with the values from `cloudflared tunnel create`, and install the
   generated credentials file for the system service:

   ```sh
   sudo install -d -m 755 /etc/cloudflared
   sudo install -m 600 "$HOME/.cloudflared/<TUNNEL-UUID>.json" \
     "/etc/cloudflared/<TUNNEL-UUID>.json"
   ```

   Validate and run the tunnel:

   ```sh
   sudo cloudflared tunnel --config /etc/cloudflared/config.yml ingress validate
   sudo cloudflared service install
   sudo systemctl enable --now cloudflared
   ```

   If `cloudflared` is already installed as a service, put the config at the
   path that service uses and restart it rather than installing a second unit.

7. Check `journalctl -u jaylub -f` and `journalctl -u cloudflared -f` for
   startup or origin errors. The four origins should answer on loopback ports
   8080, 8090, 9010, and 9000 respectively. The app binds to all interfaces
   for compatibility with the previous setup, so use the host firewall to
   restrict direct access to these ports and allow access through the tunnel.

## Updating

Pull the intended commit, run `go test ./...` and `go vet ./...`, rebuild the
generated client and server using step 4, then restart with
`sudo systemctl restart jaylub`. Keep `internal/database/`, `data/`, and
`internal/services/users.txt` writable by the service account and back up the
SQLite databases and uploaded files persistently.

GitHub Actions runs tests, vet, and a server build on pushes and pull requests.
Set repository branch protection separately if those checks should be required
before merging.
