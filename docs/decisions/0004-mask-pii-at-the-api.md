# 0004. Mask PII at the API layer

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

Customer records contain PAN and Aadhaar numbers. These will flow toward an AI agent and, eventually, an LLM. Relying on a single downstream redaction step means one bug exposes raw identity numbers.

## Decision

The banking API masks PII itself, as the first of two independent boundaries:

- PAN `ABCPS1234F` becomes `XXXXX1234F`
- Aadhaar `234567899012` becomes `XXXX XXXX 9012`
- A value with an unexpected length is masked completely rather than partially

The repository returns an internal `accounts.Customer` with raw values. Handlers only ever serialise `accounts.Profile`, which is built through `ToProfile` and is always masked. Raw values have no JSON path to the client.

A separate redaction layer (planned) will strip PII again before any data reaches an LLM.

## Consequences

- No client, human or agent, can obtain full identity numbers from this API.
- Features that genuinely need the raw value (for example, KYC verification) would need a separate, authorised endpoint.
