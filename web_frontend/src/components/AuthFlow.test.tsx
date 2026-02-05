import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act } from '../test/test-utils'
import { AuthFlow } from './AuthFlow'
import * as client from '../api/client'

vi.mock('../api/client', () => ({
  requestAuthChallenge: vi.fn(),
  pollAuthStatus: vi.fn(),
  fetchCurrentUser: vi.fn(),
  logout: vi.fn(),
  AuthError: class AuthError extends Error {
    name = 'AuthError'
  },
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

// Mock clipboard
const mockClipboard = {
  writeText: vi.fn().mockResolvedValue(undefined),
}
Object.assign(navigator, { clipboard: mockClipboard })

describe('AuthFlow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  describe('idle state', () => {
    it('should show login button initially', () => {
      render(<AuthFlow />)
      expect(screen.getByRole('button', { name: /login with matrix/i })).toBeInTheDocument()
    })

    it('should have clickable login button', () => {
      render(<AuthFlow />)
      const button = screen.getByRole('button', { name: /login with matrix/i })
      expect(button).not.toBeDisabled()
    })
  })

  describe('loading state', () => {
    it('should show loading when starting auth', async () => {
      vi.mocked(client.requestAuthChallenge).mockImplementation(
        () => new Promise(() => {}) // Never resolves
      )

      render(<AuthFlow />)

      await act(async () => {
        screen.getByRole('button', { name: /login with matrix/i }).click()
      })

      expect(screen.getByText('Generating challenge...')).toBeInTheDocument()
    })

    it('should call requestAuthChallenge when login button is clicked', async () => {
      vi.mocked(client.requestAuthChallenge).mockImplementation(
        () => new Promise(() => {})
      )

      render(<AuthFlow />)

      await act(async () => {
        screen.getByRole('button', { name: /login with matrix/i }).click()
      })

      expect(client.requestAuthChallenge).toHaveBeenCalledTimes(1)
    })
  })

  describe('component structure', () => {
    it('should render within a container', () => {
      const { container } = render(<AuthFlow />)
      expect(container.firstChild).not.toBeNull()
    })
  })
})
