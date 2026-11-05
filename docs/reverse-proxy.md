# Running Kivo behind a reverse proxy

Kivo works behind any reverse proxy. Two settings matter:

- **Trusted proxies.** Set `KIVO_TRUSTED_PROXIES` (or `trusted_proxies` in `server.toml`) to your proxy's address or range. Kivo then reads the client's address from `X-Forwarded-For`, which decides local or remote streaming limits and sign-in throttling. Without it, every client looks like the proxy.
- **Base path**, only if you serve Kivo under a subpath: `KIVO_BASE_PATH=/kivo`. Every route, asset and cookie then lives under it. Don't strip the prefix in the proxy.

Allow WebSocket upgrades on `/api/v1/events`, turn off response buffering, and set a long read timeout for video. The examples assume Kivo at `kivo:8484`.

## Caddy

```
kivo.example.com {
	reverse_proxy kivo:8484
}
```

Caddy passes WebSockets and `X-Forwarded-For` by default and doesn't buffer. For a subpath, use `handle /kivo*` with the same `reverse_proxy`, without `handle_path` (which strips the prefix).

## nginx

```nginx
server {
    listen 443 ssl;
    server_name kivo.example.com;

    location / {        # for a subpath: location /kivo/ { … } and KIVO_BASE_PATH=/kivo
        proxy_pass http://kivo:8484;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $http_connection;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_read_timeout 1h;
        client_max_body_size 30m;   # artwork uploads
    }
}
```

## SWAG

SWAG is nginx: put the `location` block above in `nginx/proxy-confs/kivo.subdomain.conf` (or `kivo.subfolder.conf` for `/kivo`), keeping SWAG's `include /config/nginx/proxy.conf;` and adding `proxy_buffering off;`.

## Nginx Proxy Manager

Add a proxy host for `kivo:8484` with **Websockets Support** on. Under **Advanced**, add `proxy_buffering off;` and `proxy_read_timeout 1h;`. For a subpath, add a custom location `/kivo` to the same host and set `KIVO_BASE_PATH=/kivo`.

## Traefik

```yaml
labels:
  - traefik.enable=true
  - traefik.http.routers.kivo.rule=Host(`kivo.example.com`)
  - traefik.http.routers.kivo.tls.certresolver=letsencrypt
  - traefik.http.services.kivo.loadbalancer.server.port=8484
```

Traefik passes WebSockets and doesn't buffer. For a subpath, use ``Host(`example.com`) && PathPrefix(`/kivo`)`` with no StripPrefix middleware.

## Cloudflare Tunnel

Not recommended for video: Cloudflare's terms have historically restricted streaming through its network. Tailscale works for your own devices.
