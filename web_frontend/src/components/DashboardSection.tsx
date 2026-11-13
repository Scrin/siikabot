import type { ReactNode } from 'react'

/**
 * Each card gets an equal share of the row and is at least 32rem wide, so the row holds as many
 * cards as fit. A card that isn't rendered leaves no hole, and on a narrow window the cards fall
 * into a single column.
 */
const evenGrid = 'grid-cols-[repeat(auto-fit,minmax(min(100%,32rem),1fr))]'

interface DashboardSectionProps {
  title: string
  /** Replaces the even grid, for a section whose cards need set widths */
  gridClassName?: string
  /**
   * Aligns the cards to the top of their row instead of stretching them to one height, for a
   * section with a card that grows when it is opened
   */
  alignTop?: boolean
  children: ReactNode
}

/** One topic of the dashboard: a label above a row of cards */
export function DashboardSection({
  title,
  gridClassName = evenGrid,
  alignTop = false,
  children,
}: DashboardSectionProps) {
  return (
    <section aria-label={title}>
      <div className="mb-4 flex items-center gap-4">
        <h2 className="font-mono text-xs tracking-[0.3em] text-purple-400/70 uppercase">{title}</h2>
        <div className="h-px flex-1 bg-gradient-to-r from-purple-500/30 to-transparent" />
      </div>
      <div className={`grid gap-6 ${gridClassName} ${alignTop ? 'items-start' : ''}`}>
        {children}
      </div>
    </section>
  )
}
