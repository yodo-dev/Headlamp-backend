# Project Commands

## Prerequisites
Make sure the following are installed on your machine:
- Go (installed via `brew install go`)
- golang-migrate (installed via `brew install golang-migrate`)
- PostgreSQL 

---

## Running the Server

### Development
```bash
make dev
```
Copies `app.development.env` → `app.env` and starts the server.

### Production
```bash
make prod
```
Copies `app.production.env` → `app.env` and starts the server.

### Start server only (uses current app.env)
```bash
make server
# or
go run main.go
```

---

## Database Migrations

### Run all migrations (development)
```bash
make migratedev
```

### Run all migrations (production)
```bash
make migrateprod
```

### Run all migrations (current app.env)
```bash
make migrateup
```

### Run 1 migration up
```bash
make migrateup1
```

### Roll back all migrations
```bash
make migratedown
```

### Roll back 1 migration
```bash
make migratedown1
```

### Create a new migration file
```bash
make new_migration name=your_migration_name
```

### Customer.io analytics schema checks (migration 000028)
```bash
cp app.development.env app.env
DB_SOURCE=$(grep '^DB_SOURCE=' app.env | cut -d= -f2-)

make migratedev
psql "$DB_SOURCE" -c "\dt user_segments"
psql "$DB_SOURCE" -c "\dt customerio_event_attributions"
```

### Verify insert paths (rollback-safe)
```bash
psql "$DB_SOURCE" <<'SQL'
BEGIN;
INSERT INTO user_segments (person_id, user_id, role, segment_name, metadata, source)
VALUES ('verify:temp-user', 'temp-user', 'parent', 'plan_free', '{"check":"ok"}'::jsonb, 'manual_verify');
SELECT person_id, segment_name, source FROM user_segments WHERE person_id='verify:temp-user';
ROLLBACK;

BEGIN;
INSERT INTO customerio_event_attributions (event_type, person_id, campaign_id, message_id, delivery_id, link_id, action, payload)
VALUES ('opened', 'parent:temp-user', 'camp_test', 'msg_test', 'del_test', 'link_test', 'open', '{"check":"ok"}'::jsonb);
SELECT event_type, person_id, action FROM customerio_event_attributions WHERE person_id='parent:temp-user';
ROLLBACK;
SQL
```

### Customer.io backfill CLI
```bash
# Dry run
cp app.development.env app.env
go run ./cmd/customerio_backfill --role all --limit 100 --dry-run

# Real run
cp app.development.env app.env
go run ./cmd/customerio_backfill --role all --limit 100
```

Available flags:
- `--role` values: `parent`, `child`, `all`
- `--limit` max rows to process (`0` means no limit)
- `--dry-run` logs targets without queue writes

### Migration Recovery (after `force` or schema mismatch)

If `migrate version` shows a version as applied, but tables from that migration are missing, the DB state and migration state are out of sync.

Example symptom:
- `migrate ... version` returns `19`
- expected table from `000019` is not present in `public`

Recommended recovery:
1. Do not edit already-applied migration files.
2. Create a new forward-only repair migration (for example `000020_repair_...`) with `CREATE TABLE IF NOT EXISTS ...` and `CREATE INDEX IF NOT EXISTS ...`.
3. Run migrations again.

Useful checks:
```bash
cp app.development.env app.env
DB_SOURCE=$(grep '^DB_SOURCE=' app.env | cut -d= -f2-)

migrate -path db/migration -database "$DB_SOURCE" version
psql "$DB_SOURCE" -c "select * from schema_migrations;"
psql "$DB_SOURCE" -c "select current_database(), current_schema();"
psql "$DB_SOURCE" -c "select table_name from information_schema.tables where table_schema='public' and table_name='child_training_step_progress';"
```

Note on migration files:
- Seeing 4 files is normal when you have 2 migrations (`000019`, `000020`) because each migration has an `.up.sql` and a `.down.sql` file.

---

## Environment Files

| File | Purpose |
|------|---------|
| `app.env` | Active config loaded by the server (gitignored) |
| `app.development.env` | Local development credentials |
| `app.production.env` | Production credentials (fill in before deploying) |

> **Note:** Never commit `app.env`, `app.development.env`, or `app.production.env` — they are all gitignored.

---

## Kill Port & Restart

If port 8080 is already in use:
```bash
lsof -ti:8080 | xargs kill -9
go run main.go
```

---

## Go Modules

### Download all dependencies
```bash
go mod download
```

### Tidy up unused dependencies
```bash
go mod tidy
```

---

## Code Generation (sqlc)
```bash
make sqlc
```

## Run Tests
```bash
make test
```
