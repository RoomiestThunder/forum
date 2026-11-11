/**
 * Account field rules. They mirror the server's (internal/api/validate.go); the
 * server is what enforces them, these only let the forms say so before a
 * round trip.
 */

export const USERNAME_PATTERN = /^[A-Za-z0-9_]{3,20}$/
export const USERNAME_HINT = 'Username must be 3–20 characters: letters, digits and underscores.'

export const PASSWORD_MIN_LENGTH = 8
/** bcrypt, which the server hashes with, only takes 72 bytes. */
export const PASSWORD_MAX_BYTES = 72

const encoder = new TextEncoder()

/** Returns a message if the password is unacceptable, otherwise null. */
export function passwordProblem(password: string): string | null {
  if ([...password].length < PASSWORD_MIN_LENGTH) {
    return `Password must be at least ${PASSWORD_MIN_LENGTH} characters.`
  }
  if (encoder.encode(password).length > PASSWORD_MAX_BYTES) {
    return `Password must be at most ${PASSWORD_MAX_BYTES} bytes.`
  }
  return null
}
