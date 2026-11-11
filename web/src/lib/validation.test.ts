import { describe, expect, it } from 'vitest'
import { PASSWORD_MAX_BYTES, PASSWORD_MIN_LENGTH, USERNAME_PATTERN, passwordProblem } from './validation'

describe('USERNAME_PATTERN', () => {
  it.each(['abc', 'Alice_99', 'a'.repeat(20), '___'])('accepts %s', (name) => {
    expect(USERNAME_PATTERN.test(name)).toBe(true)
  })

  it.each(['ab', 'a'.repeat(21), 'two words', 'a/b', '@alice', 'алиса', 'trailing\n', ''])(
    'rejects %j',
    (name) => {
      expect(USERNAME_PATTERN.test(name)).toBe(false)
    },
  )
})

describe('passwordProblem', () => {
  it('accepts a password at the minimum length', () => {
    expect(passwordProblem('x'.repeat(PASSWORD_MIN_LENGTH))).toBeNull()
  })

  it('rejects one character short of the minimum', () => {
    expect(passwordProblem('x'.repeat(PASSWORD_MIN_LENGTH - 1))).toMatch(/at least 8/)
  })

  it('accepts a long passphrase such as a password manager generates', () => {
    expect(passwordProblem('x'.repeat(64))).toBeNull()
  })

  it('accepts exactly the byte limit and rejects one byte over', () => {
    expect(passwordProblem('x'.repeat(PASSWORD_MAX_BYTES))).toBeNull()
    expect(passwordProblem('x'.repeat(PASSWORD_MAX_BYTES + 1))).toMatch(/at most 72 bytes/)
  })

  it('counts the minimum in characters but the maximum in bytes', () => {
    expect(passwordProblem('я'.repeat(8))).toBeNull() // 8 characters, 16 bytes
    expect(passwordProblem('я'.repeat(37))).toMatch(/at most/) // 37 characters, 74 bytes
  })
})
