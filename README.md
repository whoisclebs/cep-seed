# cep-seed

CEP database seed tooling: imports the Correios eDNE Básico dataset into an
API-initialized SQLite database. This repository contains the eDNE parser,
importer, and updater — it never creates or modifies schema DDL.

The companion `cep-api` repository owns migrations and the HTTP lookup service.

**⚠ Internal use only.** This service is designed for internal, non-commercial
deployment. You must supply your own authorized eDNE Básico copy. Neither
source nor transformed data is distributed with this project.

---

## Architecture

```
cmd/
├── cep-updater/      # One-shot updater: download, extract, import
└── edne-import/      # Standalone database importer

internal/
├── edne/             # eDNE delimited file parser
├── importer/         # Database population pipeline
├── updater/          # Download/extract/import orchestration
└── config/           # Shared env-var helpers
```

## Schema contract

This package **never creates or modifies schema**. It requires an API-initialized
database (schema version 1) at the target path. See `cep-api` for schema
initialization and migration ownership.

The importer validates the schema contract on every write:
- Expected migration version (≥1) from `_migrations`
- Required tables: `countries`, `states`, `localities`, `neighborhoods`, `postal_codes`, `dataset_metadata`
- Required columns on `postal_codes`
- Generated column `formatted_postal_code`

If the API has not initialized the database, the importer returns an actionable
error.

---

## Build

```sh
# Binaries
go build ./cmd/cep-updater/
go build ./cmd/edne-import/

# Cross-compile for Linux ARM64
GOOS=linux GOARCH=arm64 go build ./cmd/cep-updater/
```

### Import eDNE data (standalone)

```sh
./edne-import --src=/path/to/edne/delimited/files --out=cep.db --release="eDNE Basico 2025"
```

This reads the @-delimited files, converts ISO-8859-1 to UTF-8, resolves
locality and neighborhood references, validates the schema, and writes
canonical data to the prepared database. On success, the previous database
is preserved as `cep.db.previous` for rollback.

### Run the updater

```sh
./cep-updater --db=/data/cep.db --source=https://www2.correios.com.br/sistemas/edne/download/eDNE_Basico.zip
```

The updater is **one-shot**: each execution performs at most one update
attempt. It skips if the current release is already applied.

---

## Docker

### Build

```sh
docker build -t cep-seed:latest -f Dockerfile .
```

### Compose

```yaml
services:
  seed:
    build:
      context: .
      dockerfile: Dockerfile
    image: cep-seed:latest
    container_name: cep-seed
    profiles:
      - manual
    volumes:
      - cep-data:/data:rw
    environment:
      - CEP_DB_PATH=/data/cep.db
      - CEP_EDNE_URL=https://www2.correios.com.br/sistemas/edne/download/eDNE_Basico.zip
      - CEP_DOWNLOAD_TIMEOUT=5m

volumes:
  cep-data:
    name: cep-data
    external: true
```

### Usage

```sh
# Create the shared volume (once)
docker volume create cep-data

# Start the API first (from cep-api repo)
cd ../cep-api
docker compose up -d

# Run the seed tool to populate the database
cd ../cep-seed
docker compose --profile manual run --rm seed
```

The API detects the new database via polling and transitions to ready
without restarting. Subsequent updates:

```sh
docker compose --profile manual run --rm seed
```

### Volume permissions

Both images use UID 1000. The `/data` directory is created and owned by
`cep:cep` (1000:1000). On Docker for Linux, this matches the default volume
ownership. On Docker Desktop (macOS/Windows), the volume's actual owner may
differ; ensure the `/data` directory in the volume is writable by UID 1000.

---

## Database replacement and rollback

The updater and `edne-import` follow the same atomic publication protocol:

1. Copy the existing database to `{path}.new`
2. Validate schema contract
3. Clear data tables, insert new reference data and postal codes
4. Validate integrity (SQLite `PRAGMA integrity_check`)
5. Rename existing `{path}` → `{path}.previous`
6. Rename `{path}.new` → `{path}`

To roll back after an update, stop the API, restore the previous database,
and restart.

---

## Updater flags and env vars

| Flag | Default | Env var | Description |
|---|---|---|---|
| `--source` | `https://www2.correios.com.br/sistemas/edne/download/eDNE_Basico.zip` | `CEP_EDNE_URL` | Correios outer ZIP URL |
| `--db` | `/data/cep.db` | `CEP_DB_PATH` | Database path |
| `--data-dir` | `/tmp/cep-updater` | — | Temporary work directory |
| `--timeout` | `5m` | `CEP_DOWNLOAD_TIMEOUT` | HTTP download timeout |

## Import report

Every import emits a JSON report with:
- `source_release`, `built_at`, `elapsed`
- `consumed_files` — list of parsed source files
- `accepted_by_type` — record counts by address type
- `rejected` — always 0 (fail-fast on genuine errors)
- `collisions` — CEP collision details if any
- `warnings` — unresolved locality/neighborhood references
- `db_size_bytes`, `integrity` — SQLite integrity result
