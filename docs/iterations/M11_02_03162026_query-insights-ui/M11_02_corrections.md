# M11_02 Pre-Flight Corrections

**Purpose:** Verify frontend patterns before agent spawn.

---

## 1. Check Sidebar Instance Navigation Pattern

```bash
# Find where instance-scoped links are rendered
grep -n "explain\|settings.*diff\|serverId" web/src/components/layout/Sidebar.tsx
# Note: how instance-scoped links detect the current serverId
# Note: what icon library is used (lucide-react? heroicons?)
```

## 2. Check Route Pattern in App.tsx

```bash
# See current route structure
grep -n "Route\|path=" web/src/App.tsx
# Note: exact pattern for ProtectedRoute wrapping
# Note: import style for lazy-loaded vs direct page imports
```

## 3. Check API Client Usage

```bash
# How do existing hooks pass query params?
head -30 web/src/hooks/useSettingsTimeline.ts
# Note: api.get() signature — does it accept params object?
grep -n "api.get\|api.post" web/src/hooks/useStatements.ts
```

## 4. Check Icon Availability

```bash
# What icon library is used?
grep -rn "from.*lucide\|from.*heroicons\|from.*react-icons" web/src/components/layout/Sidebar.tsx | head -5
# Note: available icon names for Query Insights and Workload Report nav items
```

## 5. Check EChartWrapper Usage

```bash
# How is EChartWrapper used in existing components?
head -40 web/src/components/charts/TimeSeriesChart.tsx
# Note: option structure, sizing, theme
```

## 6. Check go:embed Pattern

```bash
# How does the existing static embed work?
cat web/embed.go
# Check if templates/ directory needs its own embed.go or if it can be embedded from handler file
grep -rn "go:embed" internal/ --include="*.go"
```

## 7. Check Existing Formatters

```bash
cat web/src/lib/formatters.ts
# Note: available functions — formatDuration, formatBytes, formatNumber
# Reuse these in DiffTable instead of writing new ones
```

---

## Corrections to Include

Based on grep results, update team-prompt with:
1. Exact icon import path and available icon names
2. Exact api.get() param passing pattern
3. Exact route wrapping pattern (ProtectedRoute nesting)
4. Sidebar link registration pattern (how serverId is obtained)
5. go:embed path constraints for template file
6. Available formatter functions to reuse
