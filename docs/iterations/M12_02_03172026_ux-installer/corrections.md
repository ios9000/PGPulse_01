# PGPulse M12_02 — Corrections (Pre-Flight)

**Date:** 2026-03-17
**Applies to:** M12_02 team-prompt

---

## Correction 1 — MINGW64 NSIS path mangling

**Problem:** Git Bash (MINGW64) converts `/` prefixed arguments to Windows paths. Running `makensis /VERSION` becomes `makensis C:/Program Files/Git/VERSION`.

**Fix:** Use double slash `//` for NSIS flags in all bash commands:

```bash
# WRONG (Git Bash mangles /VERSION)
makensis /VERSION

# CORRECT
makensis //VERSION
```

**Applies to installer build command in team-prompt Task 3 (Installer/QA agent):**

```bash
# CORRECT command for building installer in Git Bash
makensis //V4 deploy//nsis//pgpulse.nsi
```

Or better — use relative paths without leading slash:

```bash
makensis deploy/nsis/pgpulse.nsi
```

The file path `deploy/nsis/pgpulse.nsi` is fine (no leading `/`). Only NSIS flags like `/VERSION`, `/V4`, `/DVERSION=1.0.0` need the double-slash treatment.

**Also in the .nsi script itself:** Any NSIS commands that use `/o` or similar flags inside the script are fine — MINGW64 mangling only affects command-line arguments passed from bash.

---

## Correction 2 — NSIS confirmed available

NSIS is installed at `C:\Program Files (x86)\NSIS\`. Verified:

```bash
$ makensis //VERSION
v3.10
```

Installer/QA agent can proceed with `makensis` commands — it is in PATH and working.
