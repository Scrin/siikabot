import { useEffect, useRef } from 'react'
import { useReducedMotion } from '../../hooks/useReducedMotion'

interface DataStreamProps {
  columns?: number
  speed?: 'slow' | 'normal' | 'fast'
  opacity?: number
  color?: string
  className?: string
}

// Characters to use for the stream effect
const CHARS = '01アイウエオカキクケコサシスセソタチツテトナニヌネノハヒフヘホマミムメモヤユヨラリルレロワヲン'

interface Column {
  x: number
  y: number
  speed: number
  chars: string[]
  length: number
}

export function DataStream({
  columns = 15,
  speed = 'normal',
  opacity = 0.15,
  color = '#a855f7',
  className = '',
}: DataStreamProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const prefersReducedMotion = useReducedMotion()

  useEffect(() => {
    if (prefersReducedMotion || !canvasRef.current) return

    const canvas = canvasRef.current
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const speedMultiplier = {
      slow: 0.5,
      normal: 1,
      fast: 2,
    }

    let animationFrameId: number
    let streamColumns: Column[] = []

    const initColumns = () => {
      streamColumns = []
      const columnWidth = canvas.width / columns
      for (let i = 0; i < columns; i++) {
        const length = Math.floor(Math.random() * 15) + 5
        streamColumns.push({
          x: i * columnWidth + columnWidth / 2,
          y: Math.random() * -canvas.height,
          speed: (Math.random() * 2 + 1) * speedMultiplier[speed],
          length,
          chars: Array.from({ length }, () =>
            CHARS.charAt(Math.floor(Math.random() * CHARS.length)),
          ),
        })
      }
    }

    const resize = () => {
      canvas.width = canvas.offsetWidth
      canvas.height = canvas.offsetHeight
      initColumns()
    }

    resize()
    window.addEventListener('resize', resize)

    const animate = () => {
      animationFrameId = requestAnimationFrame(animate)

      // Clear with fade effect
      ctx.fillStyle = `rgba(0, 0, 0, 0.05)`
      ctx.fillRect(0, 0, canvas.width, canvas.height)

      ctx.font = '12px monospace'

      streamColumns.forEach((col) => {
        col.chars.forEach((char, i) => {
          const y = col.y + i * 14
          if (y > 0 && y < canvas.height) {
            // Lead character is brightest
            const isLead = i === col.chars.length - 1
            const fadeOut = 1 - i / col.chars.length

            if (isLead) {
              ctx.fillStyle = `rgba(255, 255, 255, ${opacity * 2})`
              ctx.shadowColor = color
              ctx.shadowBlur = 10
            } else {
              ctx.fillStyle = color.replace(')', `, ${opacity * fadeOut})`)
              ctx.shadowBlur = 0
            }

            ctx.fillText(char, col.x, y)
          }

          // Randomly change characters
          if (Math.random() < 0.02) {
            col.chars[i] = CHARS.charAt(Math.floor(Math.random() * CHARS.length))
          }
        })

        // Move column down
        col.y += col.speed

        // Reset when off screen
        if (col.y - col.length * 14 > canvas.height) {
          col.y = Math.random() * -200 - col.length * 14
          col.speed = (Math.random() * 2 + 1) * speedMultiplier[speed]
        }
      })
    }

    animate()

    return () => {
      window.removeEventListener('resize', resize)
      cancelAnimationFrame(animationFrameId)
    }
  }, [prefersReducedMotion, columns, speed, opacity, color])

  if (prefersReducedMotion) return null

  return (
    <canvas
      ref={canvasRef}
      className={`pointer-events-none ${className}`}
      style={{ width: '100%', height: '100%' }}
    />
  )
}

// Simpler version with CSS animations for lighter weight
export function DataStreamLite({
  columns = 8,
  className = '',
}: {
  columns?: number
  className?: string
}) {
  const prefersReducedMotion = useReducedMotion()

  if (prefersReducedMotion) return null

  return (
    <div className={`pointer-events-none overflow-hidden ${className}`}>
      {Array.from({ length: columns }).map((_, i) => (
        <div
          key={i}
          className="absolute top-0 font-mono text-[10px] text-purple-500/20"
          style={{
            left: `${(i / columns) * 100 + Math.random() * 5}%`,
            animation: `dataStreamFall ${3 + Math.random() * 4}s linear infinite`,
            animationDelay: `${-Math.random() * 5}s`,
          }}
        >
          {Array.from({ length: 20 }).map((_, j) => (
            <div key={j} className="leading-tight">
              {CHARS.charAt(Math.floor(Math.random() * CHARS.length))}
            </div>
          ))}
        </div>
      ))}
    </div>
  )
}
