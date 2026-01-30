import { useState, useEffect, useCallback } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { requestAuthChallenge, pollAuthStatus } from '../api/client'
import { useAuth } from '../context/AuthContext'
import { CyberButton } from './ui/CyberButton'
import { useReducedMotion } from '../hooks/useReducedMotion'

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
  const prefersReducedMotion = useReducedMotion()

  const startAuth = useCallback(async () => {
    setState({ status: 'loading' })
    try {
      const response = await requestAuthChallenge()
      setState({
        status: 'challenge',
        challenge: response.challenge,
        pollSecret: response.poll_secret,
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

  return (
    <AnimatePresence mode="wait">
      {state.status === 'idle' && (
        <motion.div
          key="idle"
          initial={prefersReducedMotion ? {} : { opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          exit={prefersReducedMotion ? {} : { opacity: 0, y: -10 }}
        >
          <CyberButton onClick={startAuth}>Login with Matrix</CyberButton>
        </motion.div>
      )}

      {state.status === 'loading' && (
        <motion.div
          key="loading"
          className="flex items-center gap-3 text-purple-300"
          initial={prefersReducedMotion ? {} : { opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={prefersReducedMotion ? {} : { opacity: 0 }}
        >
          <motion.div
            className="h-5 w-5 rounded-full border-2 border-purple-500 border-t-transparent"
            animate={{ rotate: 360 }}
            transition={{ duration: 1, repeat: Infinity, ease: 'linear' }}
          />
          <span className="font-mono">Generating challenge...</span>
        </motion.div>
      )}

      {state.status === 'challenge' && (
        <motion.div
          key="challenge"
          className="space-y-4"
          initial={prefersReducedMotion ? {} : { opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          exit={prefersReducedMotion ? {} : { opacity: 0, y: -10 }}
        >
          <div className="border border-purple-500/30 bg-slate-800/50 p-4">
            <p className="mb-3 font-mono text-sm text-slate-400">
              Send this command in any Matrix room with the bot:
            </p>
            <div className="flex items-center gap-2">
              <motion.code
                className="flex-1 overflow-x-auto whitespace-nowrap bg-slate-900/80 px-3 py-2 font-mono text-sm text-green-400"
                initial={prefersReducedMotion ? {} : { opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ delay: 0.2 }}
              >
                !auth {state.challenge}
              </motion.code>
              <CyberButton onClick={copyCommand} variant="secondary" className="shrink-0 px-3 py-2">
                {copied ? 'Copied!' : 'Copy'}
              </CyberButton>
            </div>
          </div>

          <div className="flex items-center justify-between text-sm">
            <div className="flex items-center gap-2 text-slate-400">
              <motion.div
                className="h-2 w-2 rounded-full bg-yellow-500"
                animate={
                  prefersReducedMotion
                    ? {}
                    : {
                        scale: [1, 1.3, 1],
                        boxShadow: [
                          '0 0 0 0 rgba(234, 179, 8, 0.8)',
                          '0 0 10px 3px rgba(234, 179, 8, 0.8)',
                          '0 0 0 0 rgba(234, 179, 8, 0.8)',
                        ],
                      }
                }
                transition={{ duration: 2, repeat: Infinity }}
              />
              <span className="font-mono">Waiting for authentication...</span>
            </div>
            <CountdownTimer expiresAt={state.expiresAt} />
          </div>

          <button
            onClick={reset}
            className="font-mono text-xs text-slate-500 transition-colors hover:text-slate-300"
          >
            Cancel
          </button>
        </motion.div>
      )}

      {state.status === 'success' && (
        <motion.div
          key="success"
          className="flex items-center gap-2 text-green-400"
          initial={prefersReducedMotion ? {} : { opacity: 0, scale: 0.9 }}
          animate={{ opacity: 1, scale: 1 }}
        >
          <motion.svg
            className="h-5 w-5"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            initial={prefersReducedMotion ? {} : { pathLength: 0 }}
            animate={{ pathLength: 1 }}
            transition={{ duration: 0.5 }}
          >
            <motion.path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M5 13l4 4L19 7"
              initial={prefersReducedMotion ? {} : { pathLength: 0 }}
              animate={{ pathLength: 1 }}
              transition={{ duration: 0.5, delay: 0.2 }}
            />
          </motion.svg>
          <span className="font-mono">Authenticated successfully!</span>
        </motion.div>
      )}

      {state.status === 'error' && (
        <motion.div
          key="error"
          className="space-y-3"
          initial={prefersReducedMotion ? {} : { opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
        >
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
          <CyberButton onClick={startAuth} variant="danger">
            Try again
          </CyberButton>
        </motion.div>
      )}
    </AnimatePresence>
  )
}

function CountdownTimer({ expiresAt }: { expiresAt: Date }) {
  const [timeLeft, setTimeLeft] = useState(Math.max(0, Math.floor((expiresAt.getTime() - Date.now()) / 1000)))

  useEffect(() => {
    const interval = setInterval(() => {
      setTimeLeft(Math.max(0, Math.floor((expiresAt.getTime() - Date.now()) / 1000)))
    }, 1000)
    return () => clearInterval(interval)
  }, [expiresAt])

  return (
    <span className="font-mono text-slate-500">
      {Math.floor(timeLeft / 60)}:{(timeLeft % 60).toString().padStart(2, '0')}
    </span>
  )
}
