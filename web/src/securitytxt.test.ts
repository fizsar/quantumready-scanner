/// <reference types="vite/client" />
import { describe, expect, it } from 'vitest'
import text from '../public/.well-known/security.txt?raw'

// public/.well-known/security.txt (RFC 9116): the required fields, and an
// Expires date far enough ahead (renew it before this test starts failing).
const field = (name: string) =>
  text
    .split('\n')
    .filter((l: string) => l.startsWith(name + ': '))
    .map((l: string) => l.slice(name.length + 2))

describe('security.txt', () => {
  it('has the required fields once', () => {
    expect(field('Contact')).toEqual(['mailto:soporte@fizsar.com'])
    expect(field('Expires')).toHaveLength(1)
    expect(field('Preferred-Languages')).toEqual(['es, en'])
    expect(field('Canonical')).toEqual(['https://quantumready.fizsar.com/.well-known/security.txt'])
  })

  it('does not expire within 30 days, nor more than a year ahead', () => {
    const expires = Date.parse(field('Expires')[0])
    const day = 24 * 3600 * 1000
    expect(expires - Date.now()).toBeGreaterThan(30 * day)
    expect(expires - Date.now()).toBeLessThan(366 * day)
  })
})
