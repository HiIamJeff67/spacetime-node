# Demo deployment runbook

This runbook deploys the backend on the Google Cloud VM and the frontend on
Cloudflare Pages. Replace the example URLs with the current public URLs; do
not commit them when they are temporary Quick Tunnel URLs.

## 1. Backend on the VM

From the VM SSH terminal:

```bash
cd /opt/spacetime-node
git pull --ff-only origin main

# Keep this file on the VM only. It must contain the Pages origin and runtime secrets.
grep '^CORS_ALLOWED_ORIGINS=' .env
docker compose --env-file .env -f deploy/compose/compose.yaml up -d --build

curl -fsS http://127.0.0.1:8000/healthz
curl -fsS http://127.0.0.1:8000/readyz
```

`CORS_ALLOWED_ORIGINS` should contain only the production frontend origin,
for example:

```dotenv
CORS_ALLOWED_ORIGINS=https://spacetime-node-demo.pages.dev
```

## 2. Public HTTPS backend URL

If the VM does not have a permanent domain, run the Quick Tunnel in a tmux
session so closing the SSH window does not stop it:

```bash
tmux new -s cloudflare
cloudflared tunnel --url http://127.0.0.1:8000
```

Copy the generated `https://*.trycloudflare.com` URL. Detach with `Ctrl-b`
then `d`; reattach later with `tmux attach -t cloudflare`. The URL changes when
the Quick Tunnel is recreated, so update the frontend API variable if that
happens.

## 3. Frontend on Cloudflare Pages

Set the Pages build environment variable:

```text
VITE_API_BASE_URL=https://<current-public-backend-url>
```

Build with the repository's pinned toolchain and publish the resulting `dist`
directory. After deployment, open the Pages URL and complete:

```text
onboarding → entry → recommendation → offer click → redemption
```

The browser should send `recommendation.impressed.v1`,
`recommendation.clicked.v1`, and `recommendation.dismissed.v1` to
`/v1/recommendation-events`; click, dismissal, and redemption signals update
bounded category weights for later recommendations. Failures from
these best-effort telemetry calls must not block the user flow.

Feedback event replays are idempotent per user, journey, recommendation, offer,
and event type, so browser retries do not repeatedly increase or decrease a
category weight.

## 4. Verification checklist

- Backend `/healthz` returns `ok`.
- Backend `/readyz` returns success after dependencies are ready.
- Browser requests do not receive a CORS error.
- Recommendation, impression, click, and redemption share the same
  `journey_id` and `trace_id`.
- Logs and events contain only the hashed user identifier, never raw PII or
  secrets.
