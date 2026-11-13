import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from './test/test-utils'
import App from './App'
import * as queries from './api/queries'
import * as auth from './context/AuthContext'

vi.mock('./api/queries', () => ({
  useHealthCheck: vi.fn(),
  useMetrics: vi.fn(),
}))

vi.mock('./context/AuthContext', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./context/AuthContext')>()),
  useAuth: vi.fn(),
}))

vi.mock('./hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

// The WebGL background and the cursor trail draw on canvases, which jsdom doesn't have
vi.mock('./components/RetroBackground', () => ({ RetroBackground: () => null }))
vi.mock('./components/ui/CursorTrail', () => ({ CursorTrail: () => null }))

// The cards have tests of their own, so here they only need to show where they are
vi.mock('./components/SystemStatusCard', () => ({
  SystemStatusCard: () => <div>system status card</div>,
}))
vi.mock('./components/AuthFlow', () => ({ AuthFlow: () => <div>auth flow</div> }))
vi.mock('./components/UserInfo', () => ({ UserInfo: () => <div>user info</div> }))
vi.mock('./components/RemindersCard', () => ({ RemindersCard: () => <div>reminders card</div> }))
vi.mock('./components/WebhooksCard', () => ({ WebhooksCard: () => <div>webhooks card</div> }))
vi.mock('./components/MemoriesCard', () => ({ MemoriesCard: () => <div>memories card</div> }))
vi.mock('./components/ChatUsageCard', () => ({ ChatUsageCard: () => <div>chat usage card</div> }))
vi.mock('./components/ChatConfigCard', () => ({
  ChatConfigCard: () => <div>chat config card</div>,
}))
vi.mock('./components/RoomsCard', () => ({ RoomsCard: () => <div>rooms card</div> }))
vi.mock('./components/MessageActivityCard', () => ({
  MessageActivityCard: () => <div>message activity card</div>,
}))

const userCards = ['reminders card', 'memories card', 'rooms card']
const adminCards = ['webhooks card', 'chat usage card', 'chat config card', 'message activity card']
const adminCardTitles = ['Webhooks', 'Usage', 'Configuration', 'Message activity']

function mockAuth({ isAuthenticated, isAdmin }: { isAuthenticated: boolean; isAdmin: boolean }) {
  vi.mocked(auth.useAuth).mockReturnValue({
    isAuthenticated,
    isLoading: false,
    isAdmin,
  } as ReturnType<typeof auth.useAuth>)
}

function sectionTitles() {
  return screen.getAllByRole('heading', { level: 2 }).map((heading) => heading.textContent)
}

describe('App', () => {
  beforeEach(() => {
    vi.mocked(queries.useHealthCheck).mockReturnValue({
      data: { status: 'ok', uptime: '1h2m3s' },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof queries.useHealthCheck>)
    vi.mocked(queries.useMetrics).mockReturnValue({
      data: undefined,
      isLoading: false,
    } as unknown as ReturnType<typeof queries.useMetrics>)
  })

  it('should show only the system status and the login when logged out', () => {
    mockAuth({ isAuthenticated: false, isAdmin: false })

    render(<App />)
    expect(sectionTitles()).toEqual(['Overview'])
    expect(screen.getByText('system status card')).toBeInTheDocument()
    expect(screen.getByText('auth flow')).toBeInTheDocument()
    for (const card of [...userCards, ...adminCards]) {
      expect(screen.queryByText(card)).not.toBeInTheDocument()
    }
  })

  it("should show a user their own cards by topic, and none of the admin's", () => {
    mockAuth({ isAuthenticated: true, isAdmin: false })

    render(<App />)
    expect(sectionTitles()).toEqual(['Overview', 'Reminders & webhooks', 'Chat', 'Rooms'])
    expect(screen.getByText('user info')).toBeInTheDocument()
    for (const card of userCards) {
      expect(screen.getByText(card)).toBeInTheDocument()
    }
    for (const card of adminCards) {
      expect(screen.queryByText(card)).not.toBeInTheDocument()
    }
    expect(screen.queryByText('Admin')).not.toBeInTheDocument()
  })

  it('should show an admin every card, with the admin-only ones tagged', () => {
    mockAuth({ isAuthenticated: true, isAdmin: true })

    render(<App />)
    expect(sectionTitles()).toEqual(['Overview', 'Reminders & webhooks', 'Chat', 'Rooms'])
    for (const card of [...userCards, ...adminCards]) {
      expect(screen.getByText(card)).toBeInTheDocument()
    }
    for (const title of adminCardTitles) {
      const card = screen.getByRole('region', { name: title })
      expect(within(card).getByText('Admin')).toBeInTheDocument()
    }
    for (const title of ['Reminders', 'Memories']) {
      const card = screen.getByRole('region', { name: title })
      expect(within(card).queryByText('Admin')).not.toBeInTheDocument()
    }
  })
})
