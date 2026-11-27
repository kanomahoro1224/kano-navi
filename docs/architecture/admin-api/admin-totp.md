# Admin one-time code

Binding the
[admin one-time code](../../operations/configuration/environment.md#admin-one-time-code)
in the console. `backend/internal/adminauth/totpadmin` owns the routes; the
login itself is `adminauth`.

`TOTPStatus`, from `GET /security/totp` and every write:

```json
{"state", "asked", "boundAt", "storedUnused", "enrollment": {"expiresAt"} | null}
```

`state` is `off`, `console` (a secret bound here) or `environment`
(`ADMIN_TOTP_SECRET`). `asked` is whether login asks for the code, never in
development mode. `boundAt` is when the console's secret was bound.
`storedUnused` reports a console secret kept while the environment's is in
use. `enrollment` is the open setup, if any. No status carries a secret.

`POST /security/totp/enrollment` opens a setup with a new 160-bit secret,
held in process memory for ten minutes and replacing any open one. It
answers `{secret, qr: {size, path}, expiresAt}`: the secret in base32 and its
`otpauth://` address as one SVG path over `size` modules with the quiet zone.
It is the only answer that carries the secret. `DELETE …/enrollment` ends
the open setup.

`PUT /security/totp` takes `{password, code, currentCode}` and binds the open
setup's secret. `code` is checked against the new secret first: a wrong one
is `400 totp_code_invalid` and spends no password attempt, five end the
setup, and without an open setup the answer is
`409 totp_enrollment_missing`. Then the admin password and, while a console
secret is bound, `currentCode` of that secret are checked as a login checks
them: `403 invalid_credentials` names neither, `429 too_many_attempts`
follows the login limits, and the current code's step is spent. The secret
is sealed with `LLM_SECRETS_KEY` in `app_settings` (`admin_totp`), the
confirming code's step is spent, and the next login asks for the new code.

`DELETE /security/totp` takes `{password, code}`, checked the same way, and
removes the console's secret.

Binding, replacing and turning off end every other admin session. The
writes answer `409 environment_managed` while `ADMIN_TOTP_SECRET` is set,
`503 secrets_key_unavailable` when the secret cannot be sealed and
`503 read_only` on a read-only snapshot. Development mode checks neither the
password nor a code. A field other than these, or one that is not a string,
is `400 invalid_input`.
