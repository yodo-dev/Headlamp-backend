# HeadLamp Backend

## Overview

HeadLamp is a family digital wellness platform. This backend service powers the HeadLamp mobile app, providing a secure RESTful API for parent, child, and family management — including social media monitoring, learning content, daily reflections, digital permit assessments, and AI-generated parent insights.

## Core Technologies

| Technology | Purpose |
|---|---|
| **Go 1.23** | Primary language |
| **Gin** | HTTP framework and middleware |
| **PostgreSQL** | Primary relational database |
| **sqlc** | Type-safe SQL code generation |
| **PASETO v2** | Stateless auth tokens (parents, children) |
| **OpenAI GPT-4o** | AI-generated daily insights and reflections |
| **Firebase** | Social auth (Google, Apple) via ID token |
| **robfig/cron** | Scheduled background jobs |
| **zerolog** | Structured, leveled logging |
| **golang-migrate** | Database schema migrations |

## Getting Started

### Prerequisites

- Go 1.23+
- PostgreSQL 14+
- `sqlc` CLI (`brew install sqlc`)
- `golang-migrate` CLI (`brew install golang-migrate`)

### Installation & Setup

1. **Clone the repository:**
   ```bash
   git clone <repository-url>
   cd Headlamp-backend
   ```

2. **Install dependencies:**
   ```bash
   go mod tidy
   ```

3. **Set up environment variables:**
   ```bash
   cp app.env.example app.env
   ```
   Edit `app.env` with your database credentials, API keys, and configuration values.

4. **Run database migrations:**
   ```bash
   make migrateup
   ```

5. **Start the server:**
   ```bash
   go run main.go
   ```
   The server starts on the port defined in your config (default `0.0.0.0:8080`).

### Makefile Commands

```bash
make migrateup        # Apply all pending migrations
make migratedown      # Roll back the last migration
make sqlc             # Regenerate sqlc code from db/query/
make server           # Run the server
make test             # Run tests
```

## Project Structure

```
├── api/              # HTTP handlers and middleware (Gin)
├── db/
│   ├── migration/    # SQL migration files (golang-migrate)
│   ├── query/        # SQL query files (sqlc input)
│   ├── seeders/      # Database seed scripts
│   └── sqlc/         # Generated type-safe Go DB code
├── gpt/              # OpenAI client, prompts, and types
├── service/          # Business logic and background schedulers
├── token/            # PASETO token maker and payload
├── util/             # Config, crypto, helpers
├── sqlc.yaml         # sqlc configuration
├── go.mod
└── main.go
```

## Key Architectural Concepts

### Authentication

- **Parents**: Email/password with PASETO tokens, or social auth (Google/Apple) via Firebase ID token or OAuth polling flow.
- **Children**: Device-specific PASETO tokens. Tokens embed `UserID`, `FamilyID`, and `DeviceID`. When a child logs in on a new device, the old device is atomically deactivated (`ReplaceDeviceTx`). Short-lived access tokens are renewed via single-use refresh tokens stored in `auth_sessions`.

### Background Schedulers

Two cron-based schedulers run on configurable schedules:

- **ReflectionScheduler** (`REFLECTION_CRON_SCHEDULE`): Generates GPT daily reflections for eligible children.
- **ParentInsightScheduler** (`PARENT_INSIGHT_CRON_SCHEDULE`): Generates GPT daily digest insights for all parent-child pairs.

Both schedulers are idempotent — re-running them on the same day returns the cached result.

### AI Insights (GPT-4o)

- **Daily Reflections**: Personalised question/prompt delivered to children daily, generated from their activity context.
- **Parent Daily Insights**: A structured digest for parents summarising each child's last 24 hours — social media usage, learning progress, reflection status, and digital permit score. Returns `summary`, `highlights`, `areas_to_watch`, `conversation_starter`, `overall_tone`, and `action_suggested`.

### Database

- `sqlc` generates type-safe Go code from all SQL files in `db/query/`. After modifying any query file, run `make sqlc`.
- Complex multi-step operations use explicit DB transactions. See `db/sqlc/tx_*.go`.
- The codebase uses a hybrid approach: most queries are sqlc-generated; AI insights queries (`ai_insights`, `content_monitoring`) are hand-coded in `db/sqlc/ai_insights_queries.go`.

## API Endpoints

All endpoints are prefixed with `/v1`.

### Auth (Public)

| Method | Path | Description |
|---|---|---|
| `POST` | `/auth/parent` | Sign up parent (email/password) |
| `POST` | `/auth/parent/login` | Login parent |
| `POST` | `/auth/parent/oauth/:provider/initiate` | Initiate OAuth polling flow |
| `GET` | `/auth/parent/oauth/poll/:session_id` | Poll OAuth result |
| `GET` | `/auth/parent/oauth/:provider/start` | OAuth redirect start |
| `GET` | `/auth/parent/oauth/:provider/callback` | OAuth redirect callback |
| `POST` | `/auth/parent/oauth/:provider/process` | Process OAuth ID token |
| `POST` | `/auth/parent/firebase` | Firebase social auth |
| `POST` | `/child/link-code/verify` | Child device link via code |

### Child Routes (child token required)

| Method | Path | Description |
|---|---|---|
| `GET` | `/child/` | Get child profile |
| `PATCH` | `/child/` | Update child profile |
| `POST` | `/child/logout` | Logout child |
| `POST` | `/child/device/register` | Register push notification device |
| `GET` | `/child/notifications` | Get notifications |
| `POST` | `/child/notifications/:id/read` | Mark notification read |
| `GET` | `/child/boosters` | Get this week's booster |
| `POST` | `/child/booster/:booster_id/reflection` | Submit booster reflection video |
| `GET` | `/child/booster/:booster_id/quiz` | Get booster quiz |
| `POST` | `/child/booster/:booster_id/quiz/submit` | Submit booster quiz |
| `GET` | `/child/:id/social-media` | Get social media settings |
| `GET` | `/child/reflections/pending` | Get pending reflections |
| `POST` | `/child/reflections/:id/respond` | Respond to reflection |
| `POST` | `/child/reflections/:id/acknowledge` | Acknowledge reflection |
| `GET` | `/child/reflections/history` | Reflection history |
| `GET` | `/child/reflections/stats` | Reflection stats |
| `GET` | `/child/reflections/daily` | Today's daily reflection |
| `GET` | `/child/courses` | Get enrolled courses |
| `GET` | `/child/courses/stats` | Course stats |
| `POST` | `/child/activity/ping` | Activity heartbeat |
| `POST` | `/child/activity/session/start` | Start app session |
| `POST` | `/child/activity/session/end` | End app session |
| `GET` | `/child/activity/session/:id` | Get session status |
| `GET` | `/child/activity/ws` | WebSocket session hub |

### Parent Routes (parent token required)

| Method | Path | Description |
|---|---|---|
| `GET` | `/parent/` | Get parent profile |
| `PATCH` | `/parent/` | Update parent profile |
| `POST` | `/parent/child` | Create child |
| `GET` | `/parent/child/all` | Get all children |
| `GET` | `/parent/child/:id` | Get child |
| `PATCH` | `/parent/child/:id` | Update child |
| `GET` | `/parent/child/:id/link-code` | Generate child link code |
| `GET` | `/parent/child/:id/social-media` | Get social media settings |
| `POST` | `/parent/child/:id/social-media` | Set social media access |
| `GET` | `/parent/child/:id/activity/summary` | Activity summary |
| `GET` | `/parent/child/:id/activity/weekly-summary` | Weekly activity summary |
| `GET` | `/parent/child/:id/courses` | All courses for child |
| `GET` | `/parent/child/:id/courses/stats` | Course stats for child |
| `GET` | `/parent/child/:id/courses/latest` | Latest course |
| `GET` | `/parent/child/:id/course/:course_id` | Specific course |
| `GET` | `/parent/child/:id/course/:course_id/module/:module_id` | Course module |
| `GET` | `/parent/child/:id/course/:course_id/module/:module_id/quiz/:quiz_id` | Quiz attempts |
| `GET` | `/parent/courses` | All available courses |
| `GET` | `/parent/child/:id/boosters` | Child boosters |
| `GET` | `/parent/child/:id/booster-reflections` | Child booster reflection videos |
| `GET` | `/parent/child/:id/reflections` | Child reflections |
| `POST` | `/parent/child/:id/reflections/trigger` | Trigger reflection manually |
| `GET` | `/parent/child/:id/digital-permit-test/ws` | Digital permit test WebSocket |
| `GET` | `/parent/child/:id/digital-permit-test/v2/ws` | Digital permit test WebSocket v2 |
| `GET` | `/parent/child/:id/insights/dashboard` | Dashboard insights |
| `GET` | `/parent/child/:id/insights/engagement` | Engagement overview |
| `GET` | `/parent/child/:id/insights/content-monitoring` | Content monitoring summary |
| `POST` | `/parent/child/:id/insights/content-monitoring/event` | Post content monitoring event |
| `GET` | `/parent/child/:id/insights/daily` | Today's GPT daily insight |
| `GET` | `/parent/child/:id/insights/daily/history` | Daily insight history |
| `POST` | `/parent/child/:id/insights/daily/:insight_id/read` | Mark insight as read |

## Environment Variables

Copy `app.env.example` to `app.env` and configure:

```env
DB_SOURCE=postgresql://user:password@localhost:5432/headlamp?sslmode=disable
SERVER_ADDRESS=0.0.0.0:8080
OPENAI_API_KEY=sk-...
FIREBASE_CREDENTIALS_JSON=...
PASETO_SYMMETRIC_KEY=...
REFLECTION_CRON_SCHEDULE=0 8 * * *
PARENT_INSIGHT_CRON_SCHEDULE=0 20 * * *
```