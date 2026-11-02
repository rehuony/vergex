# Remote access

Local and remote installs use the same deployment: `make up` builds and starts the containers, and the web UI (including the `/api/` proxy) listens on `VERGEX_BIND_ADDRESS:VERGEX_FRONTEND_PORT`, by default `127.0.0.1:8000`.

For remote access, add nginx on the host. It has three jobs:

1. Terminate TLS and forward public port 443 to `127.0.0.1:8000`.
2. Add access control on top of the application login: basic auth, an IP allowlist, or both.
3. Avoid buffering streams: the Futures agent chat and market feed use SSE.

Vergex has no setup script for this. Adapt the config below and place it at `/etc/nginx/conf.d/vergex.conf`.

## Before you start

- Keep `VERGEX_BIND_ADDRESS=127.0.0.1` in `.env`. Never expose port 8000 directly.
- Point a domain at the server and get a certificate, for example `certbot certonly --nginx -d trade.example.com`.
- For basic auth, create a password file: `htpasswd -c /etc/nginx/vergex.htpasswd <user>` (from `apache2-utils` or `httpd-tools`).

## nginx config

```nginx
# Must be in the http {} scope; files in conf.d usually already are.
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 80;
    server_name trade.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name trade.example.com;

    ssl_certificate     /etc/letsencrypt/live/trade.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/trade.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    add_header Strict-Transport-Security "max-age=31536000" always;

    # Second access layer: use one or both.
    auth_basic           "vergex";
    auth_basic_user_file /etc/nginx/vergex.htpasswd;
    # allow 203.0.113.10;
    # deny  all;

    client_max_body_size 10m;

    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_http_version 1.1;

        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade           $http_upgrade;
        proxy_set_header Connection        $connection_upgrade;

        # SSE: the backend sends X-Accel-Buffering: no; disable buffering here too.
        proxy_buffering off;
        proxy_cache     off;

        # An agent planning turn can run for minutes.
        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
    }
}
```

Apply it with `nginx -t && systemctl reload nginx`.

## Verify

```bash
curl -I https://trade.example.com/                 # 401 without credentials
curl -u <user> https://trade.example.com/api/health
```

In the browser, pass basic auth, sign in, then open Futures and confirm that candles update live and the agent's reply streams.

## Notes

- Bind your account API keys to the server's outgoing IP: `GET /api/system/ip` after signing in, or `curl -4 ifconfig.me` on the server.
- The container's nginx sees the host nginx as the client, so backend rate limiting may see one source address. On a single-user instance this rarely matters.
- To reach the backend directly for debugging, uncomment its port mapping in `docker-compose.yml` and keep it bound to `127.0.0.1`.
