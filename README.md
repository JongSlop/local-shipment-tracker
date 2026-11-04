# local-shipment-tracker

Self-hosted shipment tracker for German carriers. One Go binary, SQLite, server-rendered HTML with htmx. No auth, no build step, no CDN.

## Carriers

- **DHL** uses the public `dhl.de` tracking endpoint, no key needed.
- **DPD, GLS, Hermes** go through [17TRACK](https://api.17track.net), so you need a free API key with `TRACK17_KEY`.
- Tracking numbers that do not match a known DHL format are sent to 17TRACK with carrier auto-detection.

## Run

```sh
cp .env.example .env   # add your 17TRACK key
set -a; . ./.env; set +a
go run .
```

Then open http://localhost:8080.

## Docker (Synology)

```sh
docker compose up -d
```

A prebuilt image is published on every push to `main`:

```sh
docker pull ghcr.io/jongslop/local-shipment-tracker:latest
```

If the package is still private, log in first with a GitHub token that has `read:packages`:

```sh
echo $TOKEN | docker login ghcr.io -u YOUR_USER --password-stdin
```

Use `docker compose up -d --build` to build from source instead.

The database lives in `./data/tracker.db`. Set `TRACK17_KEY` in `.env` next to `docker-compose.yml`.

## Notes

- Open shipments refresh every 30 minutes; the page re-polls every 60 seconds.
- The language toggle (EN/DE) is stored in a cookie.

## Third-party assets

- `static/icons/*` are [Lucide](https://lucide.dev) icons (ISC licence).
- `static/logos/dhl.svg`, `dpd.svg` and `hermes.svg` come from [Simple Icons](https://simpleicons.org) (CC0).
- `static/logos/gls.svg` is the GLS logo, taken from gls-group.eu and used here only for private, self-hosted use. The GLS mark is a trademark of General Logistics Systems; remove the file before publishing or redistributing this repo.
