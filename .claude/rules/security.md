# Security Rules

## Database Connections
- All SQL MUST use parameterized queries via pgx
- NEVER concatenate user input into SQL strings
- Connection parameters from server registry, NEVER from URL params
- Monitoring user: pg_monitor role only — NEVER superuser
- Max 3 connections per monitored instance

## Authentication
- JWT tokens (HS256) with bcrypt-hashed passwords
- Access token expiry: configurable (default 24h)
- Refresh token expiry: configurable (default 7d), stateless
- RBAC: admin (full access) and viewer (read-only)
- All mutations require Bearer token
- CSRF tokens for browser-submitted forms (deferred to M5)

## OS Metrics
- NEVER use COPY TO PROGRAM
- OS metrics via Go agent (procfs/sysfs)
- Binary paths configurable — never hardcoded

## Secrets
- No passwords/tokens in source code
- Use env vars or YAML config with restricted file permissions
- Future: Vault, K8s secrets

## Input Validation
- Validate all API inputs with struct tags
- Sanitize server names/IPs
- Rate limiting on login endpoint: 10 failed attempts per 15 minutes per IP

## Agent Teams Security
- QA Agent MUST scan for SQL injection patterns after every merge
- No agent may disable or skip security middleware
- Auth tests must cover: unauthenticated (401), wrong role (403),
  valid token (200), expired token (401)
