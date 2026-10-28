import { useState } from 'react'
import { motion } from 'framer-motion'
import { useReducedMotion } from '../hooks/useReducedMotion'

interface CopyButtonProps {
  text: string
  /** Names what is copied, for screen readers: "Copy <label>" */
  label: string
  disabled?: boolean
}

/**
 * A small button that copies text to the clipboard, and says so for a moment
 */
export function CopyButton({ text, label, disabled = false }: CopyButtonProps) {
  const prefersReducedMotion = useReducedMotion()
  const [copied, setCopied] = useState(false)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // The clipboard isn't available, such as on a page not served over HTTPS
    }
  }

  return (
    <motion.button
      type="button"
      aria-label={`Copy ${label}`}
      className="shrink-0 border border-slate-500/50 bg-slate-500/20 px-3 py-1 font-mono text-xs text-slate-300 transition-all hover:bg-slate-500/30 disabled:cursor-not-allowed disabled:opacity-50"
      onClick={copy}
      disabled={disabled}
      whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
      whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
    >
      {copied ? 'Copied!' : 'Copy'}
    </motion.button>
  )
}
