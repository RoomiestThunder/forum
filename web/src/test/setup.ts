import '@testing-library/jest-dom/vitest'
import { afterEach, vi } from 'vitest'
import { cleanup } from '@testing-library/react'

// globals:false means @testing-library/react can't auto-detect an afterEach
// to hook its cleanup into, so each rendered tree would otherwise leak into
// the next test in the same file.
afterEach(() => cleanup())

// Guard against accidental real network calls from a component under test.
// Tests that need specific responses mock '../lib/api' directly instead.
vi.stubGlobal(
  'fetch',
  vi.fn(() => Promise.reject(new Error('network calls are not allowed in tests'))),
)
