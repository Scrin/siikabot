import { useState, useEffect, useCallback } from 'react'
import { requestAuthChallenge, pollAuthStatus } from '../api/client'
import { useAuth } from '../context/AuthContext'

type AuthFlowState =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'challenge'; challenge: string; pollSecret: string; expiresAt: Date }
  | { status: 'success' }
  | { status: 'error'; message: string }

const POLL_INTERVAL = 2000 // 2 seconds

export function AuthFlow() {
  const { login } = useAuth()
  const [state, setState] = useState<AuthFlowState>({ status: 'idle' })
  const [copied, setCopied] = useState(false)

  const startAuth = useCallback(async () => {
    setState({ status: 'loading' })
    try {
      const response = await requestAuthChallenge()
      setState({
        status: 'challenge',
        challenge: response.challenge,
        pollSecret: response.poll_secret, // Keep secret in memory only, never shown to user
        expiresAt: new Date(response.expires_at),
      })
    } catch (error) {
      setState({
        status: 'error',
        message: error instanceof Error ? error.message : 'Failed to start authentication',
      })
    }
  }, [])

  // Poll for auth completion
  useEffect(() => {
    if (state.status !== 'challenge') return

    const { challenge, pollSecret, expiresAt } = state

    const poll = async () => {
      // Check if challenge expired
      if (new Date() > expiresAt) {
        setState({ status: 'error', message: 'Authentication challenge expired' })
        return
      }

      try {
        const response = await pollAuthStatus(challenge, pollSecret)
        if (response.status === 'authenticated' && response.token && response.user_id) {
          login(response.token, response.user_id)
          setState({ status: 'success' })
        }
      } catch {
        // Ignore polling errors, just keep trying
      }
    }

    const interval = setInterval(poll, POLL_INTERVAL)
    poll() // Initial poll

    return () => clearInterval(interval)
  }, [state, login])

  const copyCommand = useCallback(() => {
    if (state.status === 'challenge') {
      navigator.clipboard.writeText(`!auth ${state.challenge}`)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }, [state])

  const reset = useCallback(() => {
    setState({ status: 'idle' })
  }, [])

  if (state.status === 'idle') {
    return (
      <button
        onClick={startAuth}
        className="group relative overflow-hidden border border-purple-500/50 bg-purple-900/30 px-6 py-3 font-mono text-purple-300 transition-all duration-300 hover:border-purple-400 hover:bg-purple-800/40 hover:text-purple-200"
      >
        <span className="relative z-10">Login with Matrix</span>
        <div className="absolute inset-0 -translate-x-full bg-gradient-to-r from-transparent via-purple-500/10 to-transparent transition-transform duration-500 group-hover:translate-x-full" />
      </button>
    )
  }

  if (state.status === 'loading') {
    return (
      <div className="flex items-center gap-3 text-purple-300">
        <div className="h-5 w-5 animate-spin rounded-full border-2 border-purple-500 border-t-transparent" />
        <span className="font-mono">Generating challenge...</span>
      </div>
    )
  }

  if (state.status === 'challenge') {
    const timeLeft = Math.max(
      0,
      Math.floor((state.expiresAt.getTime() - Date.now()) / 1000)
    )

    return (
      <div className="space-y-4">
        <div className="border border-purple-500/30 bg-slate-800/50 p-4">
          <p className="mb-3 font-mono text-sm text-slate-400">
            Send this command in any Matrix room with the bot:
          </p>
          <div className="flex items-center gap-2">
            <code className="flex-1 overflow-x-auto whitespace-nowrap bg-slate-900/80 px-3 py-2 font-mono text-sm text-green-400">
              !auth {state.challenge}
            </code>
            <button
              onClick={copyCommand}
              className="shrink-0 border border-slate-600 bg-slate-700/50 px-3 py-2 font-mono text-xs text-slate-300 transition-colors hover:border-purple-500 hover:bg-slate-600/50"
            >
              {copied ? 'Copied!' : 'Copy'}
            </button>
          </div>
        </div>

        <div className="flex items-center justify-between text-sm">
          <div className="flex items-center gap-2 text-slate-400">
            <div className="h-2 w-2 animate-pulse rounded-full bg-yellow-500" />
            <span className="font-mono">Waiting for authentication...</span>
          </div>
          <span className="font-mono text-slate-500">
            {Math.floor(timeLeft / 60)}:{(timeLeft % 60).toString().padStart(2, '0')}
          </span>
        </div>

        <button
          onClick={reset}
          className="font-mono text-xs text-slate-500 transition-colors hover:text-slate-300"
        >
          Cancel
        </button>
      </div>
    )
  }

  if (state.status === 'success') {
    return (
      <div className="flex items-center gap-2 text-green-400">
        <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M5 13l4 4L19 7"
          />
        </svg>
        <span className="font-mono">Authenticated successfully!</span>
      </div>
    )
  }

  if (state.status === 'error') {
    return (
      <div className="space-y-3">
        <div className="flex items-center gap-2 text-red-400">
          <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M6 18L18 6M6 6l12 12"
            />
          </svg>
          <span className="font-mono">{state.message}</span>
        </div>
        <button
          onClick={startAuth}
          className="font-mono text-sm text-purple-400 transition-colors hover:text-purple-300"
        >
          Try again
        </button>
      </div>
    )
  }

  return null
}
