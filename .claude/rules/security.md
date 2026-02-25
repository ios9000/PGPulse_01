# Security Rules

## Database Connections
- All SQL MUST use parameterized queries via pgx ($1, $2 or named args)
- NEVER concatenate user input into SQL strings
- NEVER use fmt.Sprintf to build SQL queries
- Connection parameters come from server registry, NEVER from URL params
- Monitoring user has pg_monitor role only — NEVER superuser
- Connection limit: max 3 connections per monitored instance

## Authentication
- JWT tokens with bcrypt-hashed passwords
- Token expiry: configurable (default 24h)
- RBAC: admin (full access) and viewer (read-only) roles
- All API mutations require Bearer token
- CSRF tokens for browser-submitted forms

## OS Metrics
- NEVER use COPY TO PROGRAM (PGAM's critical vulnerability)
- OS metrics collected by PGPulse Agent via Go os/exec and procfs
- Agent binary paths (Patroni, ETCD) must be configurable — never hardcoded

## Secrets
- No passwords/tokens in source code
- No secrets in git history
- Use environment variables or YAML config with restricted file permissions
- Support external secret managers (Vault, K8s secrets) in future

## Input Validation
- Validate all API inputs with struct tags
- Sanitize server names/IPs before use
- Rate limiting on auth endpoints (10 attempts/minute/IP)
- All user-facing error messages are generic (no internal details leaked)

## Agent Teams Security Workflow
- QA Agent MUST scan for SQL injection patterns after every merge:
  - grep for fmt.Sprintf in any file that imports pgx
  - grep for string concatenation with SQL keywords
- No agent may disable or skip security middleware
- Auth tests must cover: unauthenticated (401), wrong role (403),
  valid token (200), expired token (401)
