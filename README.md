# HeadLamp Backend

## Overview

This is the backend service for the HeadLamp application, designed to manage parent, child, and admin users, with a strong focus on secure authentication and data management. It provides a RESTful API for all client-side interactions, including features for admin-configurable child onboarding steps and parent-facing progress tracking.

The key security feature is a robust, device-specific authentication system for child users, ensuring that sessions are tied to a specific device and cannot be easily compromised.

## Core Technologies

- **Go**: The primary programming language.
- **Gin**: A high-performance HTTP web framework for routing and middleware.
- **PostgreSQL**: The relational database for storing all application data.
- **sqlc**: For generating type-safe Go code from SQL queries.
- **PASETO**: For secure, stateless session tokens (access and refresh).
- **Clerk**: For handling authentication for parent and admin users.
- **zerolog**: For structured, leveled logging.
- **Docker**: For containerizing the application and its dependencies.

## Getting Started

### Prerequisites

- Go (version 1.18+)
- Docker and Docker Compose
- `sqlc` CLI
- A Clerk.dev account for parent/admin authentication keys.

### Installation & Setup

1.  **Clone the repository:**
    ```bash
    git clone <repository-url>
    ```

2.  **Install dependencies:**
    ```bash
    go mod tidy
    ```

3.  **Set up environment variables:**
    Create a `.env` file in the root directory by copying the `.env.example` file. Populate it with your database credentials, Clerk secret key, and other configuration values.

4.  **Start the database:**
    Use Docker Compose to start the PostgreSQL database container.
    ```bash
    docker-compose up -d
    ```

5.  **Run database migrations:**
    Apply the latest database schema migrations.
    ```bash
    make migrateup
    ```

### Running the Application

To start the server, run:

```bash
go run main.go
```

The server will start on the port specified in your configuration (default is `8080`).

## Key Architectural Concepts

### Authentication

-   **Parents**: Authenticated via Clerk. A middleware verifies the Clerk session and fetches the user's profile.
-   **Admins**: Authenticated using a custom PASETO token-based system, separate from Clerk.
-   **Children**: Authenticated using a custom PASETO token-based system. This was a major focus of our recent work.
    -   **Device-Specific Tokens**: Access and refresh tokens contain the `UserID`, `FamilyID`, and `DeviceID`.
    -   **Atomic Device Replacement**: When a child logs in on a new device, their old device is automatically deactivated, and a new one is created in a single, atomic database transaction (`ReplaceDeviceTx`).
    -   **Refresh Token Flow**: Short-lived access tokens can be renewed using long-lived, single-use refresh tokens stored in the `auth_sessions` table.

### Database & Transactions

-   **`sqlc`**: We use `sqlc` to generate Go code for all our database queries, ensuring type safety and reducing boilerplate. All SQL queries are located in the `db/query/` directory.
-   **Transactions**: For operations that require multiple database steps (e.g., creating a user and their first device), we use explicit database transactions to ensure atomicity. See `db/sqlc/tx_*.go` for examples.

### Code Generation

If you modify any SQL queries in `db/query/`, you must regenerate the `sqlc` code:

```bash
make sqlc
```

### Onboarding Flow

The application now includes a fully configurable onboarding system managed by admins.

-   **Admin-Managed Steps**: Admins can create, update, delete, and reorder onboarding steps via a dedicated API. This allows the business to tailor the initial child experience without requiring new code deployments.
-   **Child Progress Tracking**: The backend tracks each child's progress through these onboarding steps.
-   **Parent Visibility**: A new API endpoint allows parents to see the onboarding status of all their children, providing visibility into their setup progress.