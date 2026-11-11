// Package webbotauth signs this service's outbound requests with Web Bot
// Auth (draft-meunier-web-bot-auth-architecture over RFC 9421 HTTP Message
// Signatures) and serves the key directory that lets a site verify them.
//
// A Key is an Ed25519 key. Nothing here logs, stores or returns its private
// half; the directory holds public keys only. A Holder carries the signer in
// use and the keys a rotation keeps listed, and is swapped while the process
// runs (internal/webbotauth/keyadmin stores the key sealed and fills it).
package webbotauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yexca/kano-navi/backend/internal/config"
)

// EnvKey is the variable that overrides the stored signing key.
const EnvKey = "WEB_BOT_AUTH_KEY"

// DirectoryPath is where the key directory is served.
const DirectoryPath = "/.well-known/http-message-signatures-directory"

// DirectoryContentType is the directory's media type.
const DirectoryContentType = "application/http-message-signatures-directory+json"

// Tags of the two signatures this package makes.
const (
	RequestTag   = "web-bot-auth"
	DirectoryTag = "http-message-signatures-directory"
)

// RequestLifetime is how long a request signature is valid, and
// DirectoryLifetime how long the directory's own signatures are.
const (
	RequestLifetime   = time.Minute
	DirectoryLifetime = time.Hour
)

// ErrInvalidKey is a key value that is not a 32-byte Ed25519 seed.
var ErrInvalidKey = errors.New("WEB_BOT_AUTH_KEY must be a base64url Ed25519 seed of 32 bytes")

// ErrInvalidAgent is a signing origin that is not a bare https origin.
var ErrInvalidAgent = errors.New("the Web Bot Auth origin must be a bare https origin")

// Key is an Ed25519 signing key and its RFC 7638 thumbprint.
type Key struct {
	private    ed25519.PrivateKey
	public     ed25519.PublicKey
	thumbprint string
}

// ParseKey reads a 32-byte Ed25519 seed in base64url or standard base64,
// padded or not: the "d" member of an Ed25519 JWK is one.
func ParseKey(raw string) (*Key, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(strings.NewReplacer("+", "-", "/", "_").Replace(raw), "=")
	seed, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, ErrInvalidKey
	}
	return newKey(ed25519.NewKeyFromSeed(seed)), nil
}

// KeyFromEnv reads the WEB_BOT_AUTH_KEY override: nil without one, and
// ErrInvalidKey when it is set but malformed.
func KeyFromEnv(getenv func(string) string) (*Key, error) {
	raw := strings.TrimSpace(getenv(EnvKey))
	if raw == "" {
		return nil, nil
	}
	return ParseKey(raw)
}

// GenerateKey makes a new key from random (crypto/rand when nil).
func GenerateKey(random io.Reader) (*Key, error) {
	if random == nil {
		random = rand.Reader
	}
	seed := make([]byte, ed25519.SeedSize)
	if _, err := io.ReadFull(random, seed); err != nil {
		return nil, err
	}
	return newKey(ed25519.NewKeyFromSeed(seed)), nil
}

func newKey(private ed25519.PrivateKey) *Key {
	public := private.Public().(ed25519.PublicKey)
	return &Key{private: private, public: public, thumbprint: Thumbprint(public)}
}

// Seed is the key's private seed in base64url: the value the key command
// prints and the plaintext that storage seals. It is never logged or
// returned by an API.
func (k *Key) Seed() string { return base64.RawURLEncoding.EncodeToString(k.private.Seed()) }

// Public is the verification key.
func (k *Key) Public() ed25519.PublicKey { return k.public }

// Thumbprint is the key's RFC 7638 JWK thumbprint, the keyid of every
// signature.
func (k *Key) Thumbprint() string { return k.thumbprint }

// JWK is a public Ed25519 key as RFC 8037 writes it.
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
}

// JWK is the key's public JWK; it never carries the private "d".
func (k *Key) JWK() JWK {
	return JWK{Kty: "OKP", Crv: "Ed25519", X: base64.RawURLEncoding.EncodeToString(k.public)}
}

// Thumbprint is the RFC 7638 thumbprint of a public Ed25519 key: SHA-256 of
// its required members in lexicographic order, base64url without padding.
func Thumbprint(public ed25519.PublicKey) string {
	canonical := `{"crv":"Ed25519","kty":"OKP","x":"` + base64.RawURLEncoding.EncodeToString(public) + `"}`
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// HTTPSOrigin normalizes a bare https origin (scheme://host[:port], a
// trailing slash allowed) and reports whether raw is one.
func HTTPSOrigin(raw string) (string, bool) {
	origin, err := config.PublicOrigin(raw)
	if err != nil || !strings.HasPrefix(origin, "https://") {
		return "", false
	}
	return origin, true
}

// Signer signs requests as the agent whose directory is at its origin.
type Signer struct {
	key *Key
	// agent is the https origin the Signature-Agent header names.
	agent string
	// authority is the agent's host[:port], the directory's @authority.
	authority string
	// Now and Random default to the clock and crypto/rand.
	Now    func() time.Time
	Random io.Reader
}

// NewSigner signs with key as the agent at origin, a bare https origin.
func NewSigner(key *Key, origin string) (*Signer, error) {
	normalized, ok := HTTPSOrigin(origin)
	if key == nil || !ok {
		return nil, ErrInvalidAgent
	}
	parsed, _ := url.Parse(normalized)
	return &Signer{key: key, agent: normalized, authority: parsed.Host}, nil
}

// Source gives the signer a request uses, or nil when none is configured.
// A *Signer is its own source and a *Holder gives the current one.
type Source interface{ Current() *Signer }

// Current is the signer itself.
func (s *Signer) Current() *Signer { return s }

// Key is the signing key.
func (s *Signer) Key() *Key { return s.key }

// Agent is the origin the Signature-Agent header names.
func (s *Signer) Agent() string { return s.agent }

func (s *Signer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// component is one covered component: its serialized identifier, such as
// "@authority" or "@authority";req, and its value.
type component struct{ id, value string }

// signatureBase builds the RFC 9421 section 2.5 signature base: one line per
// component, then the @signature-params line, joined by LF without a final
// newline.
func signatureBase(components []component, params string) string {
	var b strings.Builder
	for _, c := range components {
		b.WriteString(c.id)
		b.WriteString(": ")
		b.WriteString(c.value)
		b.WriteByte('\n')
	}
	b.WriteString(`"@signature-params": `)
	b.WriteString(params)
	return b.String()
}

// innerList serializes the covered identifiers as a structured field inner
// list followed by its parameters.
func innerList(components []component, params string) string {
	ids := make([]string, len(components))
	for i, c := range components {
		ids[i] = c.id
	}
	return "(" + strings.Join(ids, " ") + ")" + params
}

// Authority is a URL's @authority: the host in lower case with its port
// unless it is the scheme's default (RFC 9421 section 2.2.3).
func Authority(u *url.URL) string {
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port == "" || (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		return host
	}
	return host + ":" + port
}

// signature is one labelled entry of Signature-Input and Signature.
type signature struct{ input, value string }

func sign(key *Key, label string, components []component, params string) signature {
	list := innerList(components, params)
	raw := ed25519.Sign(key.private, []byte(signatureBase(components, list)))
	return signature{label + "=" + list, label + "=:" + base64.StdEncoding.EncodeToString(raw) + ":"}
}

// setSignatures writes the entries as the Signature-Input and Signature
// dictionaries.
func setSignatures(header http.Header, entries []signature) {
	inputs, values := make([]string, len(entries)), make([]string, len(entries))
	for i, entry := range entries {
		inputs[i], values[i] = entry.input, entry.value
	}
	header.Set("Signature-Input", strings.Join(inputs, ", "))
	header.Set("Signature", strings.Join(values, ", "))
}

// SignRequest adds Signature-Agent, Signature-Input and Signature to an
// outbound request. The signature covers @authority and signature-agent,
// expires a minute after it is made and carries a fresh nonce.
func (s *Signer) SignRequest(req *http.Request) error {
	random := s.Random
	if random == nil {
		random = rand.Reader
	}
	nonce := make([]byte, 32)
	if _, err := io.ReadFull(random, nonce); err != nil {
		return err
	}
	agent := strconv.Quote(s.agent)
	created := s.now().Unix()
	params := ";created=" + strconv.FormatInt(created, 10) +
		";expires=" + strconv.FormatInt(created+int64(RequestLifetime/time.Second), 10) +
		`;keyid="` + s.key.thumbprint + `";alg="ed25519";tag="` + RequestTag + `";nonce="` + base64.StdEncoding.EncodeToString(nonce) + `"`
	req.Header.Set("Signature-Agent", agent)
	setSignatures(req.Header, []signature{sign(s.key, "sig1", []component{{`"@authority"`, Authority(req.URL)}, {`"signature-agent"`, agent}}, params)})
	return nil
}

// Retired is a key a rotation replaced, listed in the directory until Until
// but no longer signing requests.
type Retired struct {
	Key   *Key
	Until time.Time
}

// Holder carries the signer requests use and the retired keys the directory
// still lists. It is safe for concurrent use: Set swaps both at once while
// requests and directory reads load them. A nil Holder holds nothing.
type Holder struct {
	state atomic.Pointer[holderState]
	// Now reads the clock for the directory; nil is time.Now.
	Now func() time.Time
}

type holderState struct {
	signer  *Signer
	retired []Retired
}

// NewHolder holds nothing until Set.
func NewHolder() *Holder { return &Holder{} }

// Set makes signer the one requests use (nil stops signing and the
// directory) and retired the keys the directory keeps listing.
func (h *Holder) Set(signer *Signer, retired []Retired) {
	h.state.Store(&holderState{signer: signer, retired: retired})
}

// Current is the signer in use, or nil.
func (h *Holder) Current() *Signer {
	if h == nil {
		return nil
	}
	if state := h.state.Load(); state != nil {
		return state.signer
	}
	return nil
}

func (h *Holder) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// directory is what one directory answer lists: the agent's authority and
// the current key followed by each retired key whose grace has not passed.
func (h *Holder) directory() (string, []*Key) {
	if h == nil {
		return "", nil
	}
	state := h.state.Load()
	if state == nil || state.signer == nil {
		return "", nil
	}
	keys := []*Key{state.signer.key}
	now := h.now()
	for _, retired := range state.retired {
		if retired.Key != nil && now.Before(retired.Until) && retired.Key.thumbprint != state.signer.key.thumbprint {
			keys = append(keys, retired.Key)
		}
	}
	return state.signer.authority, keys
}

// Serves reports whether the directory has a key to list.
func (h *Holder) Serves() bool {
	_, keys := h.directory()
	return len(keys) > 0
}

// DirectoryBody is a JWKS of the keys, public halves only.
func DirectoryBody(keys []*Key) []byte {
	jwks := make([]JWK, len(keys))
	for i, key := range keys {
		jwks[i] = key.JWK()
	}
	body, _ := json.Marshal(struct {
		Keys []JWK `json:"keys"`
	}{jwks})
	return body
}

// signDirectory signs a directory answer once per listed key (sig1 for the
// first): each signature covers the request's @authority, which is the
// agent's own, since verifiers fetch the directory from the Signature-Agent
// origin and a reverse proxy may rewrite Host on the way in.
func signDirectory(header http.Header, authority string, keys []*Key, now time.Time) {
	created := now.Unix()
	entries := make([]signature, len(keys))
	for i, key := range keys {
		params := `;alg="ed25519";keyid="` + key.thumbprint + `";tag="` + DirectoryTag + `";created=` + strconv.FormatInt(created, 10) +
			";expires=" + strconv.FormatInt(created+int64(DirectoryLifetime/time.Second), 10)
		entries[i] = sign(key, "sig"+strconv.Itoa(i+1), []component{{`"@authority";req`, authority}}, params)
	}
	setSignatures(header, entries)
}

// ServeDirectory answers GET and HEAD on DirectoryPath with the signed
// directory of the keys the holder lists, and 404 while it lists none.
func (h *Holder) ServeDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	authority, keys := h.directory()
	if len(keys) == 0 {
		http.NotFound(w, r)
		return
	}
	body := DirectoryBody(keys)
	header := w.Header()
	header.Set("Content-Type", DirectoryContentType)
	header.Set("Cache-Control", "public, max-age=300")
	header.Set("X-Content-Type-Options", "nosniff")
	signDirectory(header, authority, keys, h.now())
	header.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}
