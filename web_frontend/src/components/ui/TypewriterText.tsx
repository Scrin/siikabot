import { motion } from 'framer-motion'
import { useReducedMotion } from '../../hooks/useReducedMotion'

interface TypewriterTextProps {
  text: string
  className?: string
  delay?: number
  speed?: number
  showCursor?: boolean
}

export function TypewriterText({
  text,
  className = '',
  delay = 0,
  speed = 0.05,
  showCursor = true,
}: TypewriterTextProps) {
  const prefersReducedMotion = useReducedMotion()

  if (prefersReducedMotion) {
    return <span className={className}>{text}</span>
  }

  return (
    <motion.span className={className} initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      {text.split('').map((char, i) => (
        <motion.span
          key={i}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: delay + i * speed }}
        >
          {char}
        </motion.span>
      ))}
      {showCursor && (
        <motion.span
          className="ml-0.5 inline-block w-2 bg-purple-500"
          animate={{ opacity: [1, 0, 1] }}
          transition={{ duration: 0.8, repeat: Infinity }}
        >
          |
        </motion.span>
      )}
    </motion.span>
  )
}
