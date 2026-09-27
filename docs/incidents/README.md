# Incidents

Postmortems for anything that went wrong: outages, data issues, bugs that reached users, or near misses worth learning from. One file per incident, named `YYYY-MM-DD-short-title.md`.

Postmortems are **blameless**: they describe what the system allowed to happen, not who made a mistake.

No incidents recorded yet.

## Template

```markdown
# YYYY-MM-DD: Short title

- **Severity:** SEV1 (money or data at risk) | SEV2 (service down) | SEV3 (degraded)
- **Duration:** HH:MM to HH:MM (timezone)
- **Status:** Resolved | Monitoring

## Summary

Two or three sentences: what happened, who was affected, how it was fixed.

## Impact

Requests failed, customers affected, money or data involved.

## Timeline

| Time | Event |
|---|---|
| HH:MM | First signal (alert, report, log) |
| HH:MM | Mitigated |
| HH:MM | Resolved |

## Root cause

What in the system allowed this to happen.

## What went well / what didn't

## Action items

| Action | Owner | Status |
|---|---|---|
| | | |
```
