import { createContext, useContext, useState, useEffect, type ReactNode } from 'react'

export type EffectsLevel = 'full' | 'reduced' | 'minimal'

interface ReducedEffectsContextType {
  effectsLevel: EffectsLevel
  setEffectsLevel: (level: EffectsLevel) => void
  cycleEffectsLevel: () => void
  // Convenience booleans
  isReduced: boolean
  isMinimal: boolean
}

const ReducedEffectsContext = createContext<ReducedEffectsContextType | null>(null)

const STORAGE_KEY = 'siikabot_ui_effects'

function isValidEffectsLevel(value: string | null): value is EffectsLevel {
  return value === 'full' || value === 'reduced' || value === 'minimal'
}

export function ReducedEffectsProvider({ children }: { children: ReactNode }) {
  const [effectsLevel, setEffectsLevel] = useState<EffectsLevel>(() => {
    if (typeof window === 'undefined') return 'full'
    const stored = localStorage.getItem(STORAGE_KEY)
    return isValidEffectsLevel(stored) ? stored : 'full'
  })

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, effectsLevel)
  }, [effectsLevel])

  const cycleEffectsLevel = () => {
    setEffectsLevel((prev) => {
      if (prev === 'full') return 'reduced'
      if (prev === 'reduced') return 'minimal'
      return 'full'
    })
  }

  const isReduced = effectsLevel === 'reduced' || effectsLevel === 'minimal'
  const isMinimal = effectsLevel === 'minimal'

  return (
    <ReducedEffectsContext.Provider
      value={{ effectsLevel, setEffectsLevel, cycleEffectsLevel, isReduced, isMinimal }}
    >
      {children}
    </ReducedEffectsContext.Provider>
  )
}

export function useReducedEffects(): ReducedEffectsContextType {
  const context = useContext(ReducedEffectsContext)
  if (!context) {
    throw new Error('useReducedEffects must be used within a ReducedEffectsProvider')
  }
  return context
}
