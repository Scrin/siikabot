import { describe, it, expect } from 'vitest'
import { generateWebhookToken, validateWebhookName, validateWebhookToken } from './webhookToken'

describe('generateWebhookToken', () => {
  it('should make 32 random bytes as base64url, without padding', () => {
    const token = generateWebhookToken()
    expect(token).toMatch(/^[A-Za-z0-9_-]{43}$/)
  })

  it('should make a different token every time', () => {
    expect(generateWebhookToken()).not.toBe(generateWebhookToken())
  })

  it('should make tokens the server accepts', () => {
    expect(validateWebhookToken(generateWebhookToken())).toBeNull()
  })
})

describe('validateWebhookToken', () => {
  it('should accept printable ASCII from 16 to 256 characters', () => {
    expect(validateWebhookToken('a'.repeat(16))).toBeNull()
    expect(validateWebhookToken('a'.repeat(256))).toBeNull()
    expect(validateWebhookToken('!"#$%&\'()*+,-./0123456789:;<=>?@[\\]^_`{|}~')).toBeNull()
  })

  it('should refuse a token that is too short or too long', () => {
    expect(validateWebhookToken('a'.repeat(15))).toBe('The token must be at least 16 characters')
    expect(validateWebhookToken('a'.repeat(257))).toBe('The token can be at most 256 characters')
  })

  // A token goes into an Authorization header as it is
  it('should refuse spaces and characters outside printable ASCII', () => {
    const problem = 'The token can only have printable ASCII characters, without spaces'
    expect(validateWebhookToken('correct horse battery staple')).toBe(problem)
    expect(validateWebhookToken('tab\tseparated-token')).toBe(problem)
    expect(validateWebhookToken('sisäänpääsytunnus123')).toBe(problem)
  })
})

describe('validateWebhookName', () => {
  it('should accept a name of 1 to 64 characters on one line', () => {
    expect(validateWebhookName('hermes')).toBeNull()
    expect(validateWebhookName('x')).toBeNull()
    expect(validateWebhookName('ä'.repeat(64))).toBeNull()
  })

  it('should check the name without surrounding whitespace, as the server saves it', () => {
    expect(validateWebhookName('  hermes  ')).toBeNull()
    expect(validateWebhookName('   ')).toBe('The name is required')
    expect(validateWebhookName(` ${'a'.repeat(64)} `)).toBeNull()
  })

  it('should refuse a name that is too long, counting characters', () => {
    expect(validateWebhookName('a'.repeat(65))).toBe('The name can be at most 64 characters')
    // Each emoji is two UTF-16 code units but one character
    expect(validateWebhookName('🐟'.repeat(64))).toBeNull()
  })

  it('should refuse a name with line breaks or other control characters', () => {
    expect(validateWebhookName('her\nmes')).toBe('The name must be a single line')
    expect(validateWebhookName('her\tmes')).toBe('The name must be a single line')
  })
})
