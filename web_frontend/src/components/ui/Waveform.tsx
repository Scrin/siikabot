import { motion, useAnimationFrame } from 'framer-motion'
import { useRef, useState } from 'react'
import { useReducedMotion } from '../../hooks/useReducedMotion'

interface WaveformProps {
  width?: number | string
  height?: number
  color?: string
  strokeWidth?: number
  amplitude?: number
  frequency?: number
  speed?: number
  status?: 'ok' | 'degraded' | 'error'
  className?: string
}

export function Waveform({
  width = '100%',
  height = 40,
  color = '#a855f7',
  strokeWidth = 2,
  amplitude = 15,
  frequency = 2,
  speed = 1,
  status = 'ok',
  className = '',
}: WaveformProps) {
  const [time, setTime] = useState(0)
  const prefersReducedMotion = useReducedMotion()

  // Adjust waveform based on status
  const statusConfig = {
    ok: { amplitude: amplitude, frequency: frequency, noise: 0 },
    degraded: { amplitude: amplitude * 1.2, frequency: frequency * 1.5, noise: 3 },
    error: { amplitude: amplitude * 1.5, frequency: frequency * 2.5, noise: 8 },
  }

  const config = statusConfig[status]

  useAnimationFrame((t) => {
    if (!prefersReducedMotion) {
      setTime(t * 0.001 * speed)
    }
  })

  const generatePath = () => {
    const points: string[] = []
    const numPoints = 100
    const actualWidth = typeof width === 'number' ? width : 300

    for (let i = 0; i <= numPoints; i++) {
      const x = (i / numPoints) * actualWidth
      const noise =
        status !== 'ok' ? Math.sin(i * 0.5 + time * 3) * config.noise : 0

      const y =
        height / 2 +
        Math.sin((i / numPoints) * Math.PI * 2 * config.frequency + time * 2) *
          config.amplitude +
        noise

      points.push(`${i === 0 ? 'M' : 'L'} ${x} ${y}`)
    }

    return points.join(' ')
  }

  if (prefersReducedMotion) {
    return (
      <svg
        width={width}
        height={height}
        className={className}
        viewBox={`0 0 ${typeof width === 'number' ? width : 300} ${height}`}
        preserveAspectRatio="none"
      >
        <line
          x1="0"
          y1={height / 2}
          x2={typeof width === 'number' ? width : 300}
          y2={height / 2}
          stroke={color}
          strokeWidth={strokeWidth}
          opacity={0.5}
        />
      </svg>
    )
  }

  return (
    <svg
      width={width}
      height={height}
      className={className}
      viewBox={`0 0 ${typeof width === 'number' ? width : 300} ${height}`}
      preserveAspectRatio="none"
      style={{ overflow: 'visible' }}
    >
      {/* Glow layer */}
      <path
        d={generatePath()}
        fill="none"
        stroke={color}
        strokeWidth={strokeWidth * 3}
        strokeLinecap="round"
        opacity={0.3}
        style={{ filter: `blur(4px)` }}
      />

      {/* Main wave */}
      <path
        d={generatePath()}
        fill="none"
        stroke={color}
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        style={{
          filter: `drop-shadow(0 0 3px ${color})`,
        }}
      />

      {/* Bright core */}
      <path
        d={generatePath()}
        fill="none"
        stroke="white"
        strokeWidth={strokeWidth * 0.5}
        strokeLinecap="round"
        opacity={0.5}
      />
    </svg>
  )
}

// Multi-wave version for more complex effects
export function MultiWaveform({
  width = '100%',
  height = 60,
  waves = 3,
  className = '',
}: {
  width?: number | string
  height?: number
  waves?: number
  className?: string
}) {
  const prefersReducedMotion = useReducedMotion()
  const timeRef = useRef(0)
  const [, forceUpdate] = useState(0)

  useAnimationFrame((t) => {
    if (!prefersReducedMotion) {
      timeRef.current = t * 0.001
      forceUpdate((n) => n + 1)
    }
  })

  const colors = ['#a855f7', '#3b82f6', '#06b6d4']
  const time = timeRef.current

  const generatePath = (waveIndex: number) => {
    const points: string[] = []
    const numPoints = 100
    const actualWidth = typeof width === 'number' ? width : 300
    const offset = waveIndex * 0.5
    const freq = 2 + waveIndex * 0.5
    const amp = 10 - waveIndex * 2

    for (let i = 0; i <= numPoints; i++) {
      const x = (i / numPoints) * actualWidth
      const y =
        height / 2 +
        Math.sin((i / numPoints) * Math.PI * 2 * freq + time * 2 + offset) * amp

      points.push(`${i === 0 ? 'M' : 'L'} ${x} ${y}`)
    }

    return points.join(' ')
  }

  if (prefersReducedMotion) {
    return (
      <svg
        width={width}
        height={height}
        className={className}
        viewBox={`0 0 ${typeof width === 'number' ? width : 300} ${height}`}
        preserveAspectRatio="none"
      >
        <line
          x1="0"
          y1={height / 2}
          x2={typeof width === 'number' ? width : 300}
          y2={height / 2}
          stroke="#a855f7"
          strokeWidth={2}
          opacity={0.5}
        />
      </svg>
    )
  }

  return (
    <svg
      width={width}
      height={height}
      className={className}
      viewBox={`0 0 ${typeof width === 'number' ? width : 300} ${height}`}
      preserveAspectRatio="none"
      style={{ overflow: 'visible' }}
    >
      {Array.from({ length: waves }).map((_, i) => (
        <motion.path
          key={i}
          d={generatePath(i)}
          fill="none"
          stroke={colors[i % colors.length]}
          strokeWidth={2 - i * 0.3}
          strokeLinecap="round"
          opacity={0.6 - i * 0.15}
          style={{
            filter: `drop-shadow(0 0 3px ${colors[i % colors.length]})`,
          }}
        />
      ))}
    </svg>
  )
}
