import { useId, type ReactNode } from 'react'
import { useAuth } from '../context/AuthContext'
import { FadeInSection } from './ui/FadeInSection'

interface DashboardCardProps {
  /** The card's heading. A card whose content has a header of its own leaves it out */
  title?: string
  /** Shown only to admins, with a tag that says so. Its data comes from the admin API */
  adminOnly?: boolean
  /** Classes for the card's place in its section's grid, such as a column span */
  className?: string
  children: ReactNode
}

/**
 * The frame of one card on the dashboard.
 *
 * It is also where the rule for admin-only cards lives: such a card renders nothing for anyone but
 * an admin, so it simply leaves its section's row to the cards beside it.
 *
 * The frame is a size container, so the card's content can lay itself out by the card's width
 * (`@md:` and the like) rather than the window's.
 */
export function DashboardCard({
  title,
  adminOnly = false,
  className = '',
  children,
}: DashboardCardProps) {
  const { isAdmin } = useAuth()
  const titleId = useId()

  if (adminOnly && !isAdmin) return null

  return (
    <FadeInSection className={className}>
      <section
        aria-labelledby={title ? titleId : undefined}
        className="@container h-full border border-slate-700/50 bg-slate-950/40 p-5"
      >
        {title && (
          <div className="mb-4 flex items-center gap-2">
            <h3
              id={titleId}
              className="font-mono text-sm tracking-wider text-slate-400 uppercase"
            >
              {title}
            </h3>
            {adminOnly && (
              <span className="border border-purple-500/40 bg-purple-900/30 px-1.5 py-px font-mono text-[10px] tracking-widest text-purple-300 uppercase">
                Admin
              </span>
            )}
          </div>
        )}
        {children}
      </section>
    </FadeInSection>
  )
}
