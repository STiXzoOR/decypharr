---
title: Reverse Proxy
description: Run Decypharr behind Nginx, Traefik, or any reverse proxy — subfolder or subdomain, with API-auth bypass for Sonarr/Radarr.
---

Decypharr exposes three HTTP surfaces that share a single port:

- the **Web UI** (HTML + static assets),
- a **qBittorrent-compatible API** at `/api/v2/`,
- a **Sabnzbd-compatible API** at `/sabnzbd/api`.

When you put it behind a reverse proxy you usually want the web UI protected by your proxy's auth (HTTP basic, OIDC, Authelia, Organizr, etc.) while the two API surfaces stay open — Sonarr and Radarr need to talk to them without your cookie or your basic-auth header.

This guide covers the two common layouts and what to configure at the Decypharr end so URL generation and redirects don't leak your proxy's prefix.

## Pick a layout

| Layout | URL example | `url_base` |
| --- | --- | --- |
| **Subfolder** (recommended) — Decypharr shares a domain with other apps | `https://example.com/decypharr/` | `/decypharr/` |
| **Subdomain** — Decypharr gets its own hostname | `https://decypharr.example.com/` | `/` (the default) |

Subfolder is the default choice if you're already running an Organizr/Heimdall/Hubble-style homepage on one domain. Subdomain is simpler at the proxy level but needs a DNS record per app.

## Set `url_base` in `config.json`

Before the proxy will produce working links, tell Decypharr its external path:

```json
{
  "url_base": "/decypharr/",
  "port": "8282"
}
```

A few rules:

- Must start with `/`. Decypharr will prepend one if you leave it off.
- Must end with `/`. Same idea — it gets appended on load.
- Use `/` (the default) for subdomain mode.

Every internal redirect, template-emitted link, and auth flow uses `url_base` as its root — without it, clicking "Log in" from behind `/decypharr/` would send you to `/login` at the proxy root.

## Nginx — subfolder

```nginx
upstream decypharr {
    server 127.0.0.1:8282;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name example.com;

    # ── Web UI: apply your auth of choice here ────────────────────
    location ^~ /decypharr/ {
        proxy_pass http://decypharr;

        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Host  $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade           $http_upgrade;
        proxy_set_header Connection        $http_connection;
        proxy_redirect off;

        # Example: HTTP basic — replace with your own auth module
        auth_basic "Decypharr";
        auth_basic_user_file /etc/nginx/htpasswd;
    }

    # ── qBittorrent API: open for Sonarr/Radarr download client ──
    location ^~ /decypharr/api/ {
        auth_basic off;
        auth_request off;
        proxy_pass http://decypharr;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
    }

    # ── Sabnzbd API: open for Usenet flow ────────────────────────
    location ^~ /decypharr/sabnzbd/api {
        auth_basic off;
        auth_request off;
        proxy_pass http://decypharr;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
    }
}
```

A couple of notes on why the blocks look like they do:

- `^~` prefix match on the API blocks makes Nginx prefer them over the web-UI block for any path under `/decypharr/api/` or `/decypharr/sabnzbd/api`. Without it, longer regex blocks elsewhere in your config could capture the API traffic.
- `auth_basic off` plus `auth_request off` on the API blocks: `auth_basic` handles nginx's own HTTP basic, `auth_request` handles sub-request authentication (Authelia, vouch, etc.). Turn off whichever applies to your setup — having both there is harmless.
- `keepalive 32` in the upstream is worth keeping — the qBit flow opens many short connections from Sonarr/Radarr.

## Nginx — subdomain

Simpler: the whole vhost is Decypharr, so auth is applied at the server level and only the API blocks override it.

```nginx
upstream decypharr {
    server 127.0.0.1:8282;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name decypharr.example.com;

    auth_basic "Decypharr";
    auth_basic_user_file /etc/nginx/htpasswd;

    location / {
        proxy_pass http://decypharr;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Host  $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade           $http_upgrade;
        proxy_set_header Connection        $http_connection;
        proxy_redirect off;
    }

    location ^~ /api/ {
        auth_basic off;
        auth_request off;
        proxy_pass http://decypharr;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location ^~ /sabnzbd/api {
        auth_basic off;
        auth_request off;
        proxy_pass http://decypharr;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Ready-to-copy versions of both configs live at [`examples/nginx/subfolder.conf`](https://github.com/sirrobot01/decypharr/blob/main/examples/nginx/subfolder.conf) and [`examples/nginx/subdomain.conf`](https://github.com/sirrobot01/decypharr/blob/main/examples/nginx/subdomain.conf) in the repository.

## Point Sonarr / Radarr at the right URL

In Sonarr/Radarr → *Settings → Download Clients → qBittorrent*, set **Host** to your proxy (e.g. `example.com`), **Port** to `443`, **URL Base** to `/decypharr` (subfolder) or leave blank (subdomain), and tick **Use SSL**. The arrs call `…/api/v2/auth/login` and `…/api/v2/…` — those hit the unauthenticated API blocks above.

For Usenet, point the SABnzbd client at `https://example.com/decypharr/sabnzbd/` (subfolder) or `https://decypharr.example.com/sabnzbd/` (subdomain). The API key is the one shown under Decypharr's *Settings → API Token*.

## Traefik

Labels for Docker / Docker Compose — subfolder layout with a strip-prefix middleware is usually *not* what you want here: Decypharr needs to see `/decypharr/…` so `url_base` matches. Pass the path through:

```yaml
labels:
  - "traefik.enable=true"
  - "traefik.http.routers.decypharr.rule=Host(`example.com`) && PathPrefix(`/decypharr`)"
  - "traefik.http.routers.decypharr.entrypoints=websecure"
  - "traefik.http.routers.decypharr.tls=true"
  - "traefik.http.services.decypharr.loadbalancer.server.port=8282"

  # Open the two API surfaces to your arrs without auth middleware
  - "traefik.http.routers.decypharr-api.rule=Host(`example.com`) && (PathPrefix(`/decypharr/api`) || PathPrefix(`/decypharr/sabnzbd/api`))"
  - "traefik.http.routers.decypharr-api.entrypoints=websecure"
  - "traefik.http.routers.decypharr-api.tls=true"
  - "traefik.http.routers.decypharr-api.priority=20"
  - "traefik.http.routers.decypharr-api.service=decypharr"
```

If you're using Traefik's `basic-auth` / `forward-auth` middleware on the main router, just don't list it on the `decypharr-api` router.

## Troubleshooting

**Logging in bounces back to `/login`.** `url_base` isn't set or doesn't match what the proxy thinks Decypharr is mounted at. The SeeOther redirect Decypharr issues on login includes the `url_base` prefix — if it's empty, you land back at the proxy root (which is usually the web UI of a different app).

**Index page returns 404 behind Nginx.** Nginx's `StripSlashes` behavior and Decypharr's internal routing can disagree if `url_base` has an unusual shape. Make sure `url_base` both starts and ends with a `/`. If it does and you still 404, enable `debug` log level in `config.json` and check whether the request reaches Decypharr at all — the app logs each request path, so a missing log line means the proxy is swallowing it.

**Sonarr/Radarr says "Unable to connect to qBittorrent".** Probably auth: your proxy's basic-auth or forward-auth middleware is intercepting the `/api/v2/*` calls. Confirm by `curl -i https://example.com/decypharr/api/v2/app/version` — if you see a `401` with `WWW-Authenticate: Basic`, that's the proxy. Open that prefix up as in the examples above.

**`/sabnzbd/api` returns `401 unauthorized: Host and token are required`.** That's not nginx — it's Decypharr refusing a Sabnzbd API call without credentials. The SABnzbd protocol takes them as query parameters (`?apikey=…` or `?ma_username=…&ma_password=…`), and Sonarr/Radarr send them automatically when you paste the API key into the download-client dialog. Calling the endpoint with plain `curl` will always 401; that's not a proxy bug.

**WebSocket/live-update parts of the UI freeze.** The `Upgrade` / `Connection` headers need to be forwarded on the web-UI block — both examples above include them. If you've trimmed headers for size, re-add those two.
