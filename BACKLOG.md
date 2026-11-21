# Backlog

One row is one session: a ready-to-use prompt, its size (S/M/L) and the model.
State lives in issues and CHANGELOG.md, not here. A session without a specific
task takes the first row; once it is done, commit and delete the row.

| Prompt | Size | Model |
|---|---|---|
| #435 follow-up: in `workers/auth`, an account page for linking an external JWT identity (`AUTH_JWT_IDENTITY_CLAIM=sub`) to an existing account, modelled on the Access link page (`_access-pages.ts`): signed-in user + a valid external token → identity bound, audited; tests ≥98%, docs EXTERNAL-JWT.md | M | Opus |
| #435 follow-up: dedicated login-page messages for the error codes `jwt_failed`, `jwt_unavailable`, `jwt_not_linked`, `jwt_email_unusable` in `workers/auth`, English and Polish only (the other languages keep the generic text); tests, CHANGELOG | S | Sonnet |
