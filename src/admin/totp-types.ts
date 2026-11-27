// The admin one-time code's wire shapes (see docs/architecture/admin-api/admin-totp.md).

/** The admin one-time code: off, bound in the console, or the
 * environment's. Never carries the secret. */
export interface TOTPStatus {
  state: "off" | "console" | "environment"
  /** Whether login asks for the code (never in development mode). */
  asked: boolean
  boundAt: string | null
  /** A secret bound in the console while the environment's is in use. */
  storedUnused: boolean
  enrollment: { expiresAt: string } | null
}

/** A new secret waiting for its confirm; the only answer with the secret. */
export interface TOTPEnrollment {
  secret: string
  /** The otpauth:// address as one SVG path over `size` modules. */
  qr: { size: number; path: string }
  expiresAt: string
}
