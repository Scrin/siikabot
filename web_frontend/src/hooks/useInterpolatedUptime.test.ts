import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useInterpolatedUptime } from './useInterpolatedUptime'

describe('useInterpolatedUptime', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  describe('parsing uptime strings', () => {
    it('should parse hours, minutes, and seconds', () => {
      const { result } = renderHook(() => useInterpolatedUptime('1h30m45s'))
      expect(result.current).toBe('1h30m45s')
    })

    it('should parse hours only', () => {
      const { result } = renderHook(() => useInterpolatedUptime('2h'))
      // 2h = 7200 seconds, which formats back as "2h0s" (0 minutes omitted)
      expect(result.current).toBe('2h0s')
    })

    it('should parse minutes only', () => {
      const { result } = renderHook(() => useInterpolatedUptime('45m'))
      expect(result.current).toBe('45m0s')
    })

    it('should parse seconds only', () => {
      const { result } = renderHook(() => useInterpolatedUptime('30s'))
      expect(result.current).toBe('30s')
    })

    it('should parse hours and minutes without seconds', () => {
      const { result } = renderHook(() => useInterpolatedUptime('5h20m'))
      expect(result.current).toBe('5h20m0s')
    })

    it('should parse hours and seconds without minutes', () => {
      const { result } = renderHook(() => useInterpolatedUptime('2h15s'))
      expect(result.current).toBe('2h15s')
    })

    it('should handle undefined uptime', () => {
      const { result } = renderHook(() => useInterpolatedUptime(undefined))
      expect(result.current).toBe('0s')
    })
  })

  describe('interpolation', () => {
    it('should increment seconds every second', () => {
      const { result } = renderHook(() => useInterpolatedUptime('1h30m45s'))

      expect(result.current).toBe('1h30m45s')

      act(() => {
        vi.advanceTimersByTime(1000)
      })

      expect(result.current).toBe('1h30m46s')
    })

    it('should increment multiple seconds', () => {
      const { result } = renderHook(() => useInterpolatedUptime('1h30m45s'))

      act(() => {
        vi.advanceTimersByTime(5000)
      })

      expect(result.current).toBe('1h30m50s')
    })

    it('should roll over seconds to minutes', () => {
      const { result } = renderHook(() => useInterpolatedUptime('1h30m59s'))

      act(() => {
        vi.advanceTimersByTime(1000)
      })

      expect(result.current).toBe('1h31m0s')
    })

    it('should roll over minutes to hours', () => {
      const { result } = renderHook(() => useInterpolatedUptime('1h59m59s'))

      act(() => {
        vi.advanceTimersByTime(1000)
      })

      expect(result.current).toBe('2h0s')
    })

    it('should roll over from 59m59s to 1h', () => {
      const { result } = renderHook(() => useInterpolatedUptime('59m59s'))

      act(() => {
        vi.advanceTimersByTime(1000)
      })

      expect(result.current).toBe('1h0s')
    })
  })

  describe('syncing with backend', () => {
    it('should sync with backend when uptime changes', () => {
      const { result, rerender } = renderHook(({ uptime }) => useInterpolatedUptime(uptime), {
        initialProps: { uptime: '1h0m0s' },
      })

      // Advance some time
      act(() => {
        vi.advanceTimersByTime(5000)
      })
      expect(result.current).toBe('1h5s')

      // Backend provides new value (resync)
      rerender({ uptime: '1h0m10s' })
      expect(result.current).toBe('1h10s')
    })

    it('should reset when backend value changes significantly', () => {
      const { result, rerender } = renderHook(({ uptime }) => useInterpolatedUptime(uptime), {
        initialProps: { uptime: '1h0m0s' },
      })

      // Advance time locally
      act(() => {
        vi.advanceTimersByTime(30000)
      })
      expect(result.current).toBe('1h30s')

      // Backend says only 5 seconds have passed (server restart?)
      rerender({ uptime: '5s' })
      expect(result.current).toBe('5s')
    })
  })

  describe('formatting output', () => {
    it('should not show hours when 0', () => {
      const { result } = renderHook(() => useInterpolatedUptime('30m15s'))
      expect(result.current).toBe('30m15s')
      expect(result.current).not.toMatch(/^\d+h/)
    })

    it('should not show minutes when 0 but has hours', () => {
      const { result } = renderHook(() => useInterpolatedUptime('2h0m30s'))
      // Format shows 2h30s (no 0m)
      expect(result.current).toBe('2h30s')
    })

    it('should always show seconds', () => {
      const { result } = renderHook(() => useInterpolatedUptime('1h30m0s'))
      expect(result.current).toBe('1h30m0s')
    })

    it('should show 0s for zero total seconds', () => {
      const { result } = renderHook(() => useInterpolatedUptime('0s'))
      expect(result.current).toBe('0s')
    })
  })

  describe('interval cleanup', () => {
    it('should clear interval on unmount', () => {
      const clearIntervalSpy = vi.spyOn(global, 'clearInterval')
      const { unmount } = renderHook(() => useInterpolatedUptime('1h0m0s'))

      unmount()

      expect(clearIntervalSpy).toHaveBeenCalled()
    })
  })
})
