# ModelPort Container Image

ModelPort is a multi-model AI API gateway based on Sub2API `v0.1.183`. The formal image is published from `abingooo/modelport` to GHCR.

## Quick Start

```bash
docker run -d \
  --name sub2api \
  -p 8080:8080 \
  -e DATABASE_URL="postgres://user:pass@host:5432/sub2api" \
  -e REDIS_URL="redis://host:6379" \
  ghcr.io/abingooo/modelport:custom-v0.1.183.1
```

The image defaults to the unprivileged `sub2api` user (UID/GID `1000:1000`).
Named volumes are initialized with the image's writable data directory. For a
host bind mount, prepare the directory before the first start:

```bash
mkdir -p data
chown -R 1000:1000 data
```

An operator may explicitly run the image as root for a one-time ownership
repair; normal application containers should use the image default user.

## Docker Compose

```yaml
version: '3.8'

services:
  sub2api:
    image: ghcr.io/abingooo/modelport:custom-v0.1.183.1
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://postgres:postgres@db:5432/sub2api?sslmode=disable
      - REDIS_URL=redis://redis:6379
    depends_on:
      - db
      - redis

  db:
    image: postgres:15-alpine
    environment:
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=postgres
      - POSTGRES_DB=sub2api
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
```

## Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `DATABASE_URL` | PostgreSQL connection string | Yes | - |
| `REDIS_URL` | Redis connection string | Yes | - |
| `PORT` | Server port | No | `8080` |
| `GIN_MODE` | Gin framework mode (`debug`/`release`) | No | `release` |

## Supported Architectures

- `linux/amd64`

## Tags

- `custom-v0.1.183.1` - Immutable four-part ModelPort release
- `sha-<40-character-commit>` - Immutable publication commit anchor

ModelPort does not publish `latest`, major, or minor floating tags. For production, use the exact digest recorded in the matching GitHub Release.

## Links

- [GitHub Repository](https://github.com/abingooo/modelport)
- [Documentation](https://github.com/abingooo/modelport#readme)
- [Upstream Sub2API `v0.1.183`](https://github.com/Wei-Shaw/sub2api/tree/v0.1.183)
