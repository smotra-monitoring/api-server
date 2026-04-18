---
applyTo: "internal/database/**,data/db/**,data/prod/**"
---

# Database Guidelines

## Database Backends

- **Production**: PostgreSQL + TimescaleDB for time-series data
- **Development/Testing**: SQLite for simplicity
- Database access must be implemented via **interface abstractions** to allow easy swapping of backends
- Schema managed using `go-migrate`

## UUIDv7 Requirement

All primary key `id` fields **must use UUIDv7** format:
- UUIDv7 is time-ordered → better B-tree index efficiency, less fragmentation
- Applies to all entities: tenants, agents, sections, endpoints, users, checks, etc.
- SQL migrations: `UUID` type for PostgreSQL, `TEXT` for SQLite with validation
- Application code: use UUIDv7 libraries/functions when generating IDs

## Database Schema Overview

Multi-tenant hierarchical structure:

1. **Tenants** — top-level organizational units; each has multiple sections; identified by UUIDv7
2. **Sections** — logical groupings within a tenant; namespace separation for agents and tags
3. **Agents** — deployed on hosts; belongs to a section; config stored as JSON blob; authenticated via SHA-256 hashed API key; version-tracked
4. **Endpoints** — target addresses monitored by agents; belongs to an agent; can be enabled/disabled; supports port; multi-tag
5. **Tags** — metadata labels; belongs to a section; scope: `agent`, `endpoint`, or `global`; can be assigned to agents and/or endpoints

### Automatic Versioning (DB Triggers)

- Agent version bumps on configuration changes
- Endpoint changes ripple up to agent version
- Tag changes propagate based on scope
- Timestamps automatically updated via triggers

## sqlc — All Database Access

**All database interactions must use sqlc-generated code. Direct SQL queries in application code are prohibited.**

- **Config file**: `./data/db/dev/sqlc/sqlc.yaml`
- **Generated package**: `internal/database/queries`
- **Generation command**: `just generate-sqlc`

### Query File Organization

Files in `internal/database/queries/` organized by entity:
- `agents.sql` — queries for agents table
- `users.sql` — queries for users table
- `checks.sql` — queries for checks table
- etc. (one file per primary entity)

### Development Workflow

1. Create or modify SQL queries in the entity file (e.g., `internal/database/queries/agents.sql`)
2. Run `just generate-sqlc` to regenerate Go code
3. Import and use generated code from `internal/database/queries`
4. **Never write raw SQL queries directly in Go code**

## Migration Files

- **Development**: `data/db/dev/migrations/` (e.g., `0001_schema.up.sql`)
- **Production**: `data/prod/` (when applicable)
