## Change Summary

- What changed:
- Why:

## Risk Classification

- [ ] Low risk (docs/refactor/tests only, no external input path changes)
- [ ] Medium risk (business logic/API/model changes)
- [ ] High risk (auth/authz, external calls, untrusted input parsing, crypto, gateway/middleware, new dependency)

## Threat Model (required for high-risk changes)

- Entry points:
- Assets at risk:
- Trust boundaries:
- Failure modes and safe defaults:
- Audit and observability updates:

## Security Checklist

- [ ] Authentication and authorization checks reviewed
- [ ] Object-level access control (BOLA) verified
- [ ] Input validation and injection protections verified
- [ ] SSRF and outbound call protections verified (allowlist/timeouts/redirect policy)
- [ ] Secrets and sensitive data are not logged or hardcoded
- [ ] Crypto and key management changes use approved primitives/storage
- [ ] Security regression tests added/updated

## Exception (only when needed)

- Risk acceptance ticket:
- Compensating controls:
- Expiration date:
- Owner:

## Merge Gate Notes

For high-risk PRs, add label `security-reviewed`.
If approved as exception, use `security-exception-approved` and fill expiration above.
