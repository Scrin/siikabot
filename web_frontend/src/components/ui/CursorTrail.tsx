import { useEffect, useRef } from 'react'
import { useReducedMotion } from '../../hooks/useReducedMotion'

interface Particle {
  x: number
  y: number
  vx: number
  vy: number
  life: number
  maxLife: number
  size: number
  color: string
  hue: number
}

const COLORS = [
  'rgba(168, 85, 247, ', // purple
  'rgba(59, 130, 246, ', // blue
  'rgba(6, 182, 212, ', // cyan
  'rgba(236, 72, 153, ', // pink
]

export function CursorTrail() {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const particlesRef = useRef<Particle[]>([])
  const mouseRef = useRef({ x: 0, y: 0, prevX: 0, prevY: 0 })
  const prefersReducedMotion = useReducedMotion()

  useEffect(() => {
    if (prefersReducedMotion || !canvasRef.current) return

    const canvas = canvasRef.current
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    // Set canvas size
    const resize = () => {
      canvas.width = window.innerWidth
      canvas.height = window.innerHeight
    }
    resize()
    window.addEventListener('resize', resize)

    // Mouse move handler
    const handleMouseMove = (e: MouseEvent) => {
      mouseRef.current.prevX = mouseRef.current.x
      mouseRef.current.prevY = mouseRef.current.y
      mouseRef.current.x = e.clientX
      mouseRef.current.y = e.clientY

      // Calculate velocity
      const vx = mouseRef.current.x - mouseRef.current.prevX
      const vy = mouseRef.current.y - mouseRef.current.prevY
      const speed = Math.sqrt(vx * vx + vy * vy)

      // Spawn particles based on speed
      const particleCount = Math.min(Math.floor(speed * 0.5), 5)
      for (let i = 0; i < particleCount; i++) {
        const hue = Math.random() * 60 + 250 // Purple to cyan range
        particlesRef.current.push({
          x: mouseRef.current.x + (Math.random() - 0.5) * 10,
          y: mouseRef.current.y + (Math.random() - 0.5) * 10,
          vx: (Math.random() - 0.5) * 2 + vx * 0.3,
          vy: (Math.random() - 0.5) * 2 + vy * 0.3,
          life: 1,
          maxLife: 0.5 + Math.random() * 0.5,
          size: 2 + Math.random() * 4,
          color: COLORS[Math.floor(Math.random() * COLORS.length)],
          hue,
        })
      }

      // Limit particles
      if (particlesRef.current.length > 100) {
        particlesRef.current = particlesRef.current.slice(-100)
      }
    }
    window.addEventListener('mousemove', handleMouseMove)

    // Animation loop
    let animationFrameId: number
    let time = 0

    const animate = () => {
      animationFrameId = requestAnimationFrame(animate)
      time += 0.016

      ctx.clearRect(0, 0, canvas.width, canvas.height)

      // Update and draw particles
      particlesRef.current = particlesRef.current.filter((p) => {
        // Update position
        p.x += p.vx
        p.y += p.vy

        // Apply friction and gravity
        p.vx *= 0.95
        p.vy *= 0.95
        p.vy += 0.1 // slight gravity

        // Decay life
        p.life -= 0.016 / p.maxLife

        if (p.life <= 0) return false

        // Draw particle with glow
        const alpha = p.life * 0.8
        const size = p.size * p.life

        // Outer glow
        const gradient = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, size * 3)
        gradient.addColorStop(0, p.color + (alpha * 0.5).toFixed(2) + ')')
        gradient.addColorStop(1, p.color + '0)')
        ctx.fillStyle = gradient
        ctx.beginPath()
        ctx.arc(p.x, p.y, size * 3, 0, Math.PI * 2)
        ctx.fill()

        // Core
        ctx.fillStyle = p.color + alpha.toFixed(2) + ')'
        ctx.beginPath()
        ctx.arc(p.x, p.y, size, 0, Math.PI * 2)
        ctx.fill()

        // Bright center
        ctx.fillStyle = `rgba(255, 255, 255, ${(alpha * 0.6).toFixed(2)})`
        ctx.beginPath()
        ctx.arc(p.x, p.y, size * 0.3, 0, Math.PI * 2)
        ctx.fill()

        return true
      })

      // Draw cursor glow halo
      const glowGradient = ctx.createRadialGradient(
        mouseRef.current.x,
        mouseRef.current.y,
        0,
        mouseRef.current.x,
        mouseRef.current.y,
        80,
      )
      glowGradient.addColorStop(0, 'rgba(168, 85, 247, 0.15)')
      glowGradient.addColorStop(0.5, 'rgba(59, 130, 246, 0.05)')
      glowGradient.addColorStop(1, 'rgba(6, 182, 212, 0)')
      ctx.fillStyle = glowGradient
      ctx.beginPath()
      ctx.arc(mouseRef.current.x, mouseRef.current.y, 80, 0, Math.PI * 2)
      ctx.fill()
    }

    animate()

    return () => {
      window.removeEventListener('resize', resize)
      window.removeEventListener('mousemove', handleMouseMove)
      cancelAnimationFrame(animationFrameId)
    }
  }, [prefersReducedMotion])

  if (prefersReducedMotion) return null

  return (
    <canvas
      ref={canvasRef}
      className="pointer-events-none fixed inset-0"
      style={{ zIndex: 9999 }}
    />
  )
}
