import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '../test/test-utils'
import { SystemStatusCard } from './SystemStatusCard'

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

describe('SystemStatusCard', () => {
  describe('status display', () => {
    it('should show "Operational" for ok status', () => {
      render(<SystemStatusCard status="ok" uptime="1h30m45s" />)

      expect(screen.getByText('Operational')).toBeInTheDocument()
    })

    it('should show "Degraded" for degraded status', () => {
      render(<SystemStatusCard status="degraded" uptime="1h30m45s" />)

      expect(screen.getByText('Degraded')).toBeInTheDocument()
    })

    it('should show "Error" for error status', () => {
      render(<SystemStatusCard status="error" uptime="1h30m45s" />)

      expect(screen.getByText('Error')).toBeInTheDocument()
    })

    it('should show "Unknown" for unknown status', () => {
      render(<SystemStatusCard status="unknown" uptime="1h30m45s" />)

      expect(screen.getByText('Unknown')).toBeInTheDocument()
    })

    it('should show "Unknown" for any other status', () => {
      render(<SystemStatusCard status="something-else" uptime="1h30m45s" />)

      expect(screen.getByText('Unknown')).toBeInTheDocument()
    })
  })

  describe('status colors', () => {
    it('should have green color for ok status', () => {
      render(<SystemStatusCard status="ok" uptime="1h30m45s" />)

      const healthValue = screen.getByText('Operational')
      expect(healthValue).toHaveClass('text-emerald-400')
    })

    it('should have amber color for degraded status', () => {
      render(<SystemStatusCard status="degraded" uptime="1h30m45s" />)

      const healthValue = screen.getByText('Degraded')
      expect(healthValue).toHaveClass('text-amber-400')
    })

    it('should have rose color for error status', () => {
      render(<SystemStatusCard status="error" uptime="1h30m45s" />)

      const healthValue = screen.getByText('Error')
      expect(healthValue).toHaveClass('text-rose-400')
    })

    it('should have slate color for unknown status', () => {
      render(<SystemStatusCard status="unknown" uptime="1h30m45s" />)

      const healthValue = screen.getByText('Unknown')
      expect(healthValue).toHaveClass('text-slate-400')
    })
  })

  describe('uptime display', () => {
    it('should display uptime value', () => {
      render(<SystemStatusCard status="ok" uptime="5h20m15s" />)
      expect(screen.getByText('5h20m15s')).toBeInTheDocument()
    })

    it('should display UPTIME label', () => {
      render(<SystemStatusCard status="ok" uptime="1h0m0s" />)
      expect(screen.getByText('UPTIME')).toBeInTheDocument()
    })
  })

  describe('metrics display', () => {
    const metrics = {
      memory: { resident_mb: 45.5 },
      runtime: { goroutines: 12 },
      database: { active_conns: 2, max_conns: 10, idle_conns: 8 },
      bot: { events_handled: 1234 },
    }

    it('should display memory in MB', () => {
      render(<SystemStatusCard status="ok" uptime="1h" metrics={metrics} />)

      expect(screen.getByText('45.5 MB')).toBeInTheDocument()
      expect(screen.getByText('MEMORY')).toBeInTheDocument()
    })

    it('should display goroutines count', () => {
      render(<SystemStatusCard status="ok" uptime="1h" metrics={metrics} />)

      expect(screen.getByText('12')).toBeInTheDocument()
      expect(screen.getByText('GOROUTINES')).toBeInTheDocument()
    })

    it('should display database pool as active/max', () => {
      render(<SystemStatusCard status="ok" uptime="1h" metrics={metrics} />)

      expect(screen.getByText('2/10')).toBeInTheDocument()
      expect(screen.getByText('DB POOL')).toBeInTheDocument()
    })

    it('should display events handled with locale formatting', () => {
      render(<SystemStatusCard status="ok" uptime="1h" metrics={metrics} />)

      expect(screen.getByText('1,234')).toBeInTheDocument()
      expect(screen.getByText('EVENTS')).toBeInTheDocument()
    })

    it('should not display metrics section when not provided', () => {
      render(<SystemStatusCard status="ok" uptime="1h" />)

      expect(screen.queryByText('MEMORY')).not.toBeInTheDocument()
      expect(screen.queryByText('GOROUTINES')).not.toBeInTheDocument()
      expect(screen.queryByText('DB POOL')).not.toBeInTheDocument()
      expect(screen.queryByText('EVENTS')).not.toBeInTheDocument()
    })
  })

  describe('header', () => {
    it('should display System Status title', () => {
      render(<SystemStatusCard status="ok" uptime="1h" />)

      expect(screen.getByText('System Status')).toBeInTheDocument()
    })
  })
})
