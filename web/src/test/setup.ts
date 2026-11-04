import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

// Unmount rendered trees between tests (vitest globals are off, so RTL can't auto-register this).
afterEach(() => {
  cleanup()
})
