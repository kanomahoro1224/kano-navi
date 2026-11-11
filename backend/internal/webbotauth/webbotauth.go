// Package webbotauth signs this service's outbound requests with Web Bot
// Auth (draft-meunier-web-bot-auth-architecture over RFC 9421 HTTP Message
// Signatures) and serves the key directory that lets a site verify them.
//
// The private key is an Ed25519 seed from WEB_BOT_AUTH_KEY. It stays in
// process memory: nothing here logs, stores or returns it, and the directory
// holds the public key only.
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
	"time"
)

// EnvKey is the variable that holds the signing key.
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
// DirectoryLifetime how long the directory's own signature is.
const (
	RequestLifetime   = time.Minute
	DirectoryLifetime = time.Hour
)

// signatureLabel names the one signature each message carries.
const signatureLabel = "sig1"

// ErrInvalidKey is a WEB_BOT_AUTH_KEY that is not a 32-byte Ed25519 seed.
var ErrInvalidKey = errors.New("WEB_BOT_AUTH_KEY must be a base64url Ed25519 seed of 32 bytes")

// ErrInvalidAgent is a signer without an https PUBLIC_ORIGIN to name as its
// Signature-Agent.
var ErrInvalidAgent = errors.New("WEB_BOT_AUTH_KEY needs an https PUBLIC_ORIGIN")

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

// Seed is the key's WEB_BOT_AUTH_KEY value, for the key command only.
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

// Signer signs requests as the agent whose directory is at Agent.
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

// NewSigner signs with key as the agent at origin, an https origin such as
// a normalized PUBLIC_ORIGIN.
func NewSigner(key *Key, origin string) (*Signer, error) {
	parsed, err := url.Parse(origin)
	if key == nil || err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.User != nil {
		return nil, ErrInvalidAgent
	}
	return &Signer{key: key, agent: "https://" + parsed.Host, authority: strings.ToLower(parsed.Host)}, nil
}

// FromEnv reads WEB_BOT_AUTH_KEY: nil without one, and an error when it is
// set but invalid or origin is not https.
func FromEnv(getenv func(string) string, origin string) (*Signer, error) {
	raw := strings.TrimSpace(getenv(EnvKey))
	if raw == "" {
		return nil, nil
	}
	key, err := ParseKey(raw)
	if err != nil {
		return nil, err
	}
	return NewSigner(key, origin)
}

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
	components := []component{{`"@authority"`, Authority(req.URL)}, {`"signature-agent"`, agent}}
	req.Header.Set("Signature-Agent", agent)
	s.sign(req.Header, components, params)
	return nil
}

// DirectoryHeaders signs a directory response: the signature covers the
// request's @authority, which is the agent's own, since verifiers fetch the
// directory from the Signature-Agent origin and a reverse proxy may rewrite
// Host on the way in.
func (s *Signer) DirectoryHeaders(header http.Header) {
	created := s.now().Unix()
	params := `;alg="ed25519";keyid="` + s.key.thumbprint + `";tag="` + DirectoryTag + `";created=` + strconv.FormatInt(created, 10) +
		";expires=" + strconv.FormatInt(created+int64(DirectoryLifetime/time.Second), 10)
	s.sign(header, []component{{`"@authority";req`, s.authority}}, params)
}

func (s *Signer) sign(header http.Header, components []component, params string) {
	list := innerList(components, params)
	signature := ed25519.Sign(s.key.private, []byte(signatureBase(components, list)))
	header.Set("Signature-Input", signatureLabel+"="+list)
	header.Set("Signature", signatureLabel+"=:"+base64.StdEncoding.EncodeToString(signature)+":")
}

// Directory is the key directory's body: a JWKS with the public key only.
func (s *Signer) Directory() []byte {
	body, _ := json.Marshal(struct {
		Keys []JWK `json:"keys"`
	}{[]JWK{s.key.JWK()}})
	return body
}

// ServeDirectory answers GET and HEAD on DirectoryPath with the signed
// directory. A nil signer answers 404: no key is configured.
func (s *Signer) ServeDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s == nil {
		http.NotFound(w, r)
		return
	}
	body := s.Directory()
	header := w.Header()
	header.Set("Content-Type", DirectoryContentType)
	header.Set("Cache-Control", "public, max-age=300")
	header.Set("X-Content-Type-Options", "nosniff")
	s.DirectoryHeaders(header)
	header.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}
