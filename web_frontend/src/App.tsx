import { useHealthCheck } from './api/queries'
import { useInterpolatedUptime } from './hooks/useInterpolatedUptime'
import { useAuth } from './context/AuthContext'
import { RetroBackground } from './components/RetroBackground'
import { PageHeader } from './components/PageHeader'
import { LoadingSpinner } from './components/LoadingSpinner'
import { ErrorMessage } from './components/ErrorMessage'
import { SystemStatusCard } from './components/SystemStatusCard'
import { AuthFlow } from './components/AuthFlow'
import { UserInfo } from './components/UserInfo'
import { RemindersCard } from './components/RemindersCard'

function App() {
  const { data: health, isLoading, error } = useHealthCheck()
  const interpolatedUptime = useInterpolatedUptime(health?.uptime)
  const { isAuthenticated, isLoading: authLoading } = useAuth()

  return (
    <div className="relative min-h-screen overflow-hidden bg-black">
      {/* WebGL Retro Background */}
      <RetroBackground />

      {/* Overlay gradient for depth */}
      <div
        className="pointer-events-none absolute inset-0 bg-gradient-to-b from-black/40 via-transparent to-black/60"
        style={{ zIndex: 1 }}
      />

      {/* Content */}
      <div
        className="relative flex min-h-screen items-center justify-center p-4"
        style={{ zIndex: 10 }}
      >
        <div className="w-full max-w-4xl">
          <PageHeader />

          {/* Main Content Card */}
          <div className="overflow-hidden border border-purple-500/30 bg-slate-900/90 shadow-2xl shadow-purple-900/30 backdrop-blur-xl">
            {/* Top accent bar */}
            <div className="h-px bg-gradient-to-r from-purple-500 via-blue-500 to-purple-500" />

            <div className="p-8 md:p-12">
              {isLoading && <LoadingSpinner />}

              {error && <ErrorMessage message={error.message} />}

              {health && !error && (
                <div className="animate-[fadeIn_0.5s_ease-in]">
                  <SystemStatusCard
                    status={health.status}
                    uptime={interpolatedUptime}
                  />
                </div>
              )}

              {/* Auth Section */}
              <div className="mt-8 border-t border-slate-700/50 pt-8">
                <h3 className="mb-4 font-mono text-sm uppercase tracking-wider text-slate-500">
                  Authentication
                </h3>
                {authLoading ? (
                  <div className="flex items-center gap-2 text-slate-400">
                    <div className="h-4 w-4 animate-spin rounded-full border-2 border-purple-500 border-t-transparent" />
                    <span className="font-mono text-sm">Checking auth status...</span>
                  </div>
                ) : isAuthenticated ? (
                  <UserInfo />
                ) : (
                  <AuthFlow />
                )}
              </div>

              {/* Reminders Section - only show when authenticated */}
              {isAuthenticated && (
                <div className="mt-8 border-t border-slate-700/50 pt-8">
                  <h3 className="mb-4 font-mono text-sm uppercase tracking-wider text-slate-500">
                    Active Reminders
                  </h3>
                  <RemindersCard />
                </div>
              )}
            </div>

            {/* Bottom accent bar */}
            <div className="h-px bg-gradient-to-r from-purple-500/50 via-blue-500/50 to-purple-500/50" />
          </div>
        </div>
      </div>

      {/* CSS for custom animations */}
      <style>{`
        @keyframes fadeIn {
          from { opacity: 0; transform: translateY(10px); }
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  )
}

export default App
