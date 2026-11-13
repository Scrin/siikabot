import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '../test/test-utils'
import { DashboardCard } from './DashboardCard'
import * as auth from '../context/AuthContext'

vi.mock('../context/AuthContext', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../context/AuthContext')>()),
  useAuth: vi.fn(),
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

function mockAdmin(isAdmin: boolean) {
  vi.mocked(auth.useAuth).mockReturnValue({ isAdmin } as ReturnType<typeof auth.useAuth>)
}

describe('DashboardCard', () => {
  it('should show a card to everyone, titled and without the admin tag', () => {
    mockAdmin(false)

    render(<DashboardCard title="Reminders">reminder list</DashboardCard>)
    const card = screen.getByRole('region', { name: 'Reminders' })
    expect(within(card).getByText('reminder list')).toBeInTheDocument()
    expect(within(card).queryByText('Admin')).not.toBeInTheDocument()
  })

  it('should show a card without a title, for content with a header of its own', () => {
    mockAdmin(false)

    render(<DashboardCard>system status</DashboardCard>)
    expect(screen.getByText('system status')).toBeInTheDocument()
    expect(screen.queryByRole('heading')).not.toBeInTheDocument()
  })

  it('should leave an admin-only card out for anyone but an admin', () => {
    mockAdmin(false)

    render(
      <DashboardCard title="Webhooks" adminOnly>
        webhook list
      </DashboardCard>,
    )
    expect(screen.queryByText('Webhooks')).not.toBeInTheDocument()
    expect(screen.queryByText('webhook list')).not.toBeInTheDocument()
  })

  it('should show an admin-only card to an admin, tagged as admin-only', () => {
    mockAdmin(true)

    render(
      <DashboardCard title="Webhooks" adminOnly>
        webhook list
      </DashboardCard>,
    )
    const card = screen.getByRole('region', { name: 'Webhooks' })
    expect(within(card).getByText('webhook list')).toBeInTheDocument()
    expect(within(card).getByText('Admin')).toBeInTheDocument()
  })
})
