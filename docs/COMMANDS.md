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
