import { useHealthCheck } from './api/queries'
import { useInterpolatedUptime } from './hooks/useInterpolatedUptime'
import { RetroBackground } from './components/RetroBackground'
import { PageHeader } from './components/PageHeader'
import { LoadingSpinner } from './components/LoadingSpinner'
import { ErrorMessage } from './components/ErrorMessage'
import { SystemStatusCard } from './components/SystemStatusCard'

function App() {
  const { data: health, isLoading, error } = useHealthCheck()
  const interpolatedUptime = useInterpolatedUptime(health?.uptime)

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

              {health && (
                <div className="animate-[fadeIn_0.5s_ease-in]">
                  <SystemStatusCard
                    status={health.status}
                    uptime={interpolatedUptime}
                  />
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
