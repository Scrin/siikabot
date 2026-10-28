/**
 * Webhook names and tokens: the rules the server checks them by, so the form can say what's wrong
 * before anything is sent, and the tokens the form generates
 */

export const WEBHOOK_NAME_MAX_LENGTH = 64
export const WEBHOOK_TOKEN_MIN_LENGTH = 16
export const WEBHOOK_TOKEN_MAX_LENGTH = 256

/** A generated token has 256 random bits, 43 characters as base64url */
const GENERATED_TOKEN_BYTES = 32

/**
 * Generate a webhook token: 32 random bytes as base64url, without padding
 */
export function generateWebhookToken(): string {
  const bytes = new Uint8Array(GENERATED_TOKEN_BYTES)
  crypto.getRandomValues(bytes)
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

/**
 * Says why a token can't be used, or returns null if it can. A token goes into an Authorization
 * header as it is, so it may only have printable ASCII characters, and no spaces.
 */
export function validateWebhookToken(token: string): string | null {
  if (token.length < WEBHOOK_TOKEN_MIN_LENGTH) {
    return `The token must be at least ${WEBHOOK_TOKEN_MIN_LENGTH} characters`
  }
  if (token.length > WEBHOOK_TOKEN_MAX_LENGTH) {
    return `The token can be at most ${WEBHOOK_TOKEN_MAX_LENGTH} characters`
  }
  if (!/^[\x21-\x7e]+$/.test(token)) {
    return 'The token can only have printable ASCII characters, without spaces'
  }
  return null
}

/**
 * Says why a name can't be used, or returns null if it can. The name is checked as the server
 * saves it, without surrounding whitespace.
 */
export function validateWebhookName(name: string): string | null {
  const trimmed = name.trim()
  if (trimmed === '') {
    return 'The name is required'
  }
  // Counted in characters, not UTF-16 code units, as the server counts them
  if ([...trimmed].length > WEBHOOK_NAME_MAX_LENGTH) {
    return `The name can be at most ${WEBHOOK_NAME_MAX_LENGTH} characters`
  }
  if (/\p{Cc}/u.test(trimmed)) {
    return 'The name must be a single line'
  }
  return null
}
