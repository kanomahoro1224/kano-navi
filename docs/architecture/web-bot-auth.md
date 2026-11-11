# Web Bot Auth

The server can identify its own outbound requests with Web Bot Auth: an
HTTP Message Signature (RFC 9421) in the profile of the IETF draft
`draft-meunier-web-bot-auth-architecture`. A site that verifies the
signature can tell this service's automated requests from anonymous
traffic and give them a higher limit. The official shop's Shopify storefront
limits unsigned automated traffic most strictly and accepts signed requests
without any enrollment, so the [shop source](sources.md#official-shop) is
the one client that signs. `backend/internal/webbotauth` owns the key,
request signing and the key directory.

## Key

`WEB_BOT_AUTH_KEY` holds an Ed25519 private key as its 32-byte seed in
base64url (standard base64 and padding are accepted too; it is the `d` of an
Ed25519 JWK). `kano-go web-bot-auth-key` prints a new key as a
`WEB_BOT_AUTH_KEY=` line followed by a comment with its keyid; it reads no
environment and stores nothing. A key needs an `https` `PUBLIC_ORIGIN`,
since that origin names the agent and serves its directory. `kano-go serve`
and `kano-go sync` refuse to start, before opening SQLite, when the key is
not a 32-byte seed or when `PUBLIC_ORIGIN` is empty or `http`. Without the
variable nothing is signed and the directory answers 404.

The key stays in process memory. It never reaches SQLite, `raw_json`, logs,
run results or any API response; the directory and the admin API carry the
public key and the variable's name only.

The keyid is the key's JWK thumbprint (RFC 7638): SHA-256 over
`{"crv":"Ed25519","kty":"OKP","x":"<public key>"}`, in base64url without
padding.

## Request Signature

A signed request carries three headers next to its own
`User-Agent: kano-navi-go/shop`:

```text
Signature-Agent: "https://<PUBLIC_ORIGIN host>"
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
`application/http-message-signatures-directory+json` with a JWKS holding the
public key only:

```json
{ "keys": [{ "kty": "OKP", "crv": "Ed25519", "x": "<public key>" }] }
```

The response is itself signed, so a verifier can check that the directory
holds the key it signs with:

```text
Signature-Input: sig1=("@authority";req);alg="ed25519";keyid="<thumbprint>";tag="http-message-signatures-directory";created=<unix>;expires=<unix + 3600>
Signature: sig1=:<base64>:
```

`"@authority";req` is the authority of the request the directory answers.
Verifiers fetch the directory from the `Signature-Agent` origin, and a
reverse proxy may rewrite `Host` on the way in, so the signed value is
`PUBLIC_ORIGIN`'s host rather than the incoming `Host` header. The response
is cacheable for five minutes, well inside the signature's hour, and is not
compressed. Without a key the route answers `404 {error: "not_found"}`; other
methods answer 405.

## Related Docs

- [Sources: official shop](sources.md#official-shop)
- [Public API](public-api.md#web-bot-auth-key-directory)
- [Configuration](../operations/configuration.md#web-bot-auth)
- [Deployment security](../operations/security.md#web-bot-auth-key)
