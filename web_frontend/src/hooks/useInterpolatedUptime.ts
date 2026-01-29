import { useEffect, useState } from 'react'

/**
 * Parses an uptime string like "1h30m45s" or "45s" or "2m30s" into total seconds
 */
function parseUptimeToSeconds(uptime: string): number {
  let totalSeconds = 0

  // Match hours
  const hoursMatch = uptime.match(/(\d+)h/)
  if (hoursMatch) {
    totalSeconds += parseInt(hoursMatch[1]) * 3600
  }

  // Match minutes
  const minutesMatch = uptime.match(/(\d+)m/)
  if (minutesMatch) {
    totalSeconds += parseInt(minutesMatch[1]) * 60
  }

  // Match seconds
  const secondsMatch = uptime.match(/(\d+)s/)
  if (secondsMatch) {
    totalSeconds += parseInt(secondsMatch[1])
  }

  return totalSeconds
}

/**
 * Formats seconds into an uptime string like "1h30m45s"
 */
function formatSecondsToUptime(totalSeconds: number): string {
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60

  const parts: string[] = []
  if (hours > 0) parts.push(`${hours}h`)
  if (minutes > 0) parts.push(`${minutes}m`)
  parts.push(`${seconds}s`)

  return parts.join('')
}

/**
 * Hook that takes the backend uptime and interpolates it locally every second
 * Syncs with the real backend value when it changes
 */
export function useInterpolatedUptime(
  backendUptime: string | undefined,
): string {
  const [displayedSeconds, setDisplayedSeconds] = useState<number>(0)

  // Sync with backend when it changes
  useEffect(() => {
    if (backendUptime) {
      setDisplayedSeconds(parseUptimeToSeconds(backendUptime))
    }
  }, [backendUptime])

  // Increment locally every second
  useEffect(() => {
    const interval = setInterval(() => {
      setDisplayedSeconds((prev) => prev + 1)
    }, 1000)

    return () => clearInterval(interval)
  }, [])

  return formatSecondsToUptime(displayedSeconds)
}
