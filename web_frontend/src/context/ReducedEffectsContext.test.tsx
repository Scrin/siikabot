import { describe, it, expect, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import type { ReactNode } from 'react'
import { ReducedEffectsProvider, useReducedEffects } from './ReducedEffectsContext'

const wrapper = ({ children }: { children: ReactNode }) => (
  <ReducedEffectsProvider>{children}</ReducedEffectsProvider>
)

describe('ReducedEffectsContext', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  describe('initial state', () => {
    it('should default to full effects', () => {
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      expect(result.current.effectsLevel).toBe('full')
      expect(result.current.isReduced).toBe(false)
      expect(result.current.isMinimal).toBe(false)
    })

    it('should restore reduced from localStorage', () => {
      localStorage.setItem('siikabot_ui_effects', 'reduced')

      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      expect(result.current.effectsLevel).toBe('reduced')
      expect(result.current.isReduced).toBe(true)
    })

    it('should restore minimal from localStorage', () => {
      localStorage.setItem('siikabot_ui_effects', 'minimal')

      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      expect(result.current.effectsLevel).toBe('minimal')
      expect(result.current.isMinimal).toBe(true)
    })

    it('should ignore invalid localStorage values', () => {
      localStorage.setItem('siikabot_ui_effects', 'invalid-value')

      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      expect(result.current.effectsLevel).toBe('full')
    })

    it('should ignore empty localStorage', () => {
      localStorage.setItem('siikabot_ui_effects', '')

      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      expect(result.current.effectsLevel).toBe('full')
    })
  })

  describe('setEffectsLevel', () => {
    it('should update to reduced', () => {
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      act(() => {
        result.current.setEffectsLevel('reduced')
      })

      expect(result.current.effectsLevel).toBe('reduced')
    })

    it('should update to minimal', () => {
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      act(() => {
        result.current.setEffectsLevel('minimal')
      })

      expect(result.current.effectsLevel).toBe('minimal')
      expect(result.current.isMinimal).toBe(true)
    })

    it('should update to full', () => {
      localStorage.setItem('siikabot_ui_effects', 'minimal')
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      act(() => {
        result.current.setEffectsLevel('full')
      })

      expect(result.current.effectsLevel).toBe('full')
    })

    it('should persist to localStorage', () => {
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      act(() => {
        result.current.setEffectsLevel('reduced')
      })

      expect(localStorage.getItem('siikabot_ui_effects')).toBe('reduced')
    })
  })

  describe('cycleEffectsLevel', () => {
    it('should cycle from full to reduced', () => {
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      expect(result.current.effectsLevel).toBe('full')

      act(() => {
        result.current.cycleEffectsLevel()
      })

      expect(result.current.effectsLevel).toBe('reduced')
    })

    it('should cycle from reduced to minimal', () => {
      localStorage.setItem('siikabot_ui_effects', 'reduced')
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      act(() => {
        result.current.cycleEffectsLevel()
      })

      expect(result.current.effectsLevel).toBe('minimal')
    })

    it('should cycle from minimal back to full', () => {
      localStorage.setItem('siikabot_ui_effects', 'minimal')
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      act(() => {
        result.current.cycleEffectsLevel()
      })

      expect(result.current.effectsLevel).toBe('full')
    })

    it('should complete a full cycle', () => {
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      expect(result.current.effectsLevel).toBe('full')

      act(() => {
        result.current.cycleEffectsLevel()
      })
      expect(result.current.effectsLevel).toBe('reduced')

      act(() => {
        result.current.cycleEffectsLevel()
      })
      expect(result.current.effectsLevel).toBe('minimal')

      act(() => {
        result.current.cycleEffectsLevel()
      })
      expect(result.current.effectsLevel).toBe('full')
    })
  })

  describe('convenience booleans', () => {
    it('isReduced should be false for full', () => {
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      expect(result.current.effectsLevel).toBe('full')
      expect(result.current.isReduced).toBe(false)
    })

    it('isReduced should be true for reduced', () => {
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      act(() => {
        result.current.setEffectsLevel('reduced')
      })

      expect(result.current.isReduced).toBe(true)
    })

    it('isReduced should be true for minimal', () => {
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      act(() => {
        result.current.setEffectsLevel('minimal')
      })

      expect(result.current.isReduced).toBe(true)
    })

    it('isMinimal should be false for full', () => {
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      expect(result.current.isMinimal).toBe(false)
    })

    it('isMinimal should be false for reduced', () => {
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      act(() => {
        result.current.setEffectsLevel('reduced')
      })

      expect(result.current.isMinimal).toBe(false)
    })

    it('isMinimal should be true only for minimal', () => {
      const { result } = renderHook(() => useReducedEffects(), { wrapper })

      act(() => {
        result.current.setEffectsLevel('minimal')
      })

      expect(result.current.isMinimal).toBe(true)
    })
  })

  describe('useReducedEffects hook', () => {
    it('should throw error when used outside provider', () => {
      expect(() => {
        renderHook(() => useReducedEffects())
      }).toThrow('useReducedEffects must be used within a ReducedEffectsProvider')
    })
  })
})
