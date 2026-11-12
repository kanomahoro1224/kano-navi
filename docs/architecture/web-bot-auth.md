# Web Bot Auth

The server can identify its own outbound requests with Web Bot Auth: an
HTTP Message Signature (RFC 9421) in the profile of the IETF draft
`draft-meunier-web-bot-auth-architecture`. A site that verifies the
signature can tell this service's automated requests from anonymous
traffic and give them a higher limit. The official shop's Shopify storefront
limits unsigned automated traffic most strictly and accepts signed requests
without any enrollment, so the [shop source](sources.md#official-shop) is
the one client that signs. `backend/internal/webbotauth` owns the key,
request signing and the key directory; `internal/webbotauth/keyadmin` owns
the stored key and its [admin routes](admin-api.md#web-bot-auth-key).

## Key

The signing key is an Ed25519 key that this server makes. It is an identity
the service presents, not a credential a source issued, so the operator
generates, rotates and removes it in `/admin` → Sources → Official shop,
like the YouTube push signing secret. Its 32-byte private seed is sealed
with `LLM_SECRETS_KEY` (AES-256-GCM, the provider-key format) in
`app_settings.web_bot_auth_key`, beside its creation time; the private key
is never returned by an API, logged, or written to `raw_json` or run
results. The console shows the keyid, when the key was made, the signing
origin, the directory address and whether signing is active.

`WEB_BOT_AUTH_KEY` overrides the stored key while it is set: the same
32-byte seed in base64url (standard base64 and padding are accepted; it is
the `d` of an Ed25519 JWK), which `kano-go web-bot-auth-key` prints with its
keyid. The console then shows the key as environment-managed and refuses to
generate, rotate or remove one; a stored key stays stored, unused. A
malformed value stops `kano-go serve` and `kano-go sync` before SQLite opens,
since the operator asked for a key the server cannot use.

The keyid is the key's JWK thumbprint (RFC 7638): SHA-256 over
`{"crv":"Ed25519","kty":"OKP","x":"<public key>"}`, in base64url without
padding.

## Signing State

Signing needs a key and an `https` origin to name as the agent. The origin
is the operator's signing origin setting (`app_settings.web_bot_auth_origin`,
a bare `https` origin) or, without one, `PUBLIC_ORIGIN` when it is `https`.
The verifier fetches the directory from that origin, so it must reach this
site. The state is one of:

| State            | Signing | Why                                                                  |
| ---------------- | ------- | -------------------------------------------------------------------- |
| `active`         | yes     | a key and an `https` origin                                          |
| `no_key`         | no      | no stored key and no `WEB_BOT_AUTH_KEY`                              |
| `no_origin`      | no      | a key, but no signing origin and no `https` `PUBLIC_ORIGIN`          |
| `key_unreadable` | no      | the stored key no longer opens, as after a changed `LLM_SECRETS_KEY` |

Only an unreadable settings table stops the start; a missing origin or an
unreadable key leaves signing off with its state shown, and generating a
key replaces an unreadable one. Startup loads the stored settings into a
holder that the shop client and the directory route read for every request,
and every change in the console swaps it at once, so no change needs a
restart.

## Rotation

Rotating generates a new key that signs every request from then on. The key
it replaces stays sealed in `app_settings.web_bot_auth_previous_key` and is
listed in the directory for 24 hours (`web_bot_auth_previous_until`), so a
verifier that looks up the old keyid during the change, or holds a request
signed just before it, still finds it; a day covers the caches verifiers
keep of a directory and the one-hour directory signature. Only the latest
replaced key is kept, and an unreadable key is replaced without a grace
entry. Removing the key deletes both keys: signing stops and the directory
answers 404. The signing origin stays.

## Request Signature

A signed request carries three headers next to its own
`User-Agent: kano-navi-go/shop`:

```text
Signature-Agent: "https://<signing origin host>"
Signature-Input: sig1=("@authority" "signature-agent");created=<unix>;expires=<unix + 60>;keyid="<thumbprint>";alg="ed25519";tag="web-bot-auth";nonce="<base64>"
Signature: sig1=:<base64 Ed25519 signature>:
```

`Signature-Agent` is a structured-field string. The signature base follows
RFC 9421 section 2.5: one line per covered component, `"@authority": ` and
the request's host in lower case (with its port unless it is the default),
then `"signature-agent": ` and the header value as sent, then the
`"@signature-params": ` line with the inner list exactly as
`Signature-Input` writes it, joined by LF without a final newline. Each
signature is valid for 60 seconds and carries a fresh 32-byte random nonce.
The client signs a request after its proxy area transport is chosen, so a
signed request still goes through the [outbound proxy](outbound-proxy.md)
area of its client.

## Key Directory

`GET /.well-known/http-message-signatures-directory` (and `HEAD`) answers
`application/http-message-signatures-directory+json` with a JWKS of public
keys only: the current key, then a key in its rotation grace.

```json
{ "keys": [{ "kty": "OKP", "crv": "Ed25519", "x": "<public key>" }] }
```

The response is itself signed once per listed key, `sig1` with the current
key and `sig2` with the replaced one, so a verifier can check that the
directory holds each key it lists:

```text
Signature-Input: sig1=("@authority";req);alg="ed25519";keyid="<thumbprint>";tag="http-message-signatures-directory";created=<unix>;expires=<unix + 3600>
Signature: sig1=:<base64>:
```

`"@authority";req` is the authority of the request the directory answers.
Verifiers fetch the directory from the `Signature-Agent` origin, and a
reverse proxy may rewrite `Host` on the way in, so the signed value is the
signing origin's authority in lower case, with its port omitted when it is
the HTTPS default (`443`), rather than the incoming `Host` header. The response
is cacheable for five minutes, well inside the signature's hour, and is not
compressed. While signing is not active the route answers
`404 {error: "not_found"}`; other methods answer 405.

## Related Docs

- [Sources: official shop](sources.md#official-shop)
- [Admin API](admin-api.md#web-bot-auth-key)
- [Public API](public-api.md#web-bot-auth-key-directory)
- [Configuration](../operations/configuration.md#web-bot-auth)
- [Deployment security](../operations/security.md#web-bot-auth-key)
