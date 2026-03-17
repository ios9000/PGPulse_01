# M_UX_01 Pre-Flight Corrections

**Run these before spawning agents.**

---

## 1. Get ALL metric keys from alert rules

```bash
grep -oP "'[a-z]+\.[a-z._]+'" internal/alert/rules.go | sort -u
```

This gives the complete list of metrics the description registry must cover.

## 2. Check current alert rule descriptions

```bash
grep -B1 -A1 'Description:' internal/alert/rules.go | head -60
```

If descriptions are empty strings `""`, Agent 2 has work to do.

## 3. Check AlertRow click behavior

```bash
grep -n "onClick\|navigate\|Link\|useNavigate" web/src/components/alerts/AlertRow.tsx
```

Note what currently happens on click — this is what Agent 1 replaces.

## 4. Check AlertEvent JSON shape

```bash
curl -s -H "Authorization: Bearer $(curl -s -X POST http://185.159.111.139:8989/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"pgpulse_admin"}' | jq -r .access_token)" "http://185.159.111.139:8989/api/v1/alerts" | jq '.[0]'
```

Check which fields are present: rule_id, rule_name, metric, value, threshold, recommendations.

## 5. Check duration string format in API

```bash
curl -s -H "Authorization: Bearer $(curl -s -X POST http://185.159.111.139:8989/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"pgpulse_admin"}' | jq -r .access_token)" "http://185.159.111.139:8989/api/v1/instances/production-primary/snapshots/latest-diff" | jq '.duration'
```

Check the raw format — is it "4h24m36.79747s" or already formatted?

## 6. Check existing formatters

```bash
cat web/src/lib/formatters.ts
```

See what's already available to avoid duplication.

---

## Update team-prompt with:
1. Complete metric key list from grep
2. Current state of alert rule descriptions
3. Current AlertRow click behavior
4. AlertEvent JSON shape
5. Duration string format
6. Existing formatters available
