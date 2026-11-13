import { motion, AnimatePresence } from 'framer-motion'
import { useHealthCheck, useMetrics } from './api/queries'
import { useInterpolatedUptime } from './hooks/useInterpolatedUptime'
import { useAuth } from './context/AuthContext'
import { useReducedMotion } from './hooks/useReducedMotion'
import {
  ReducedEffectsProvider,
  useReducedEffects,
} from './context/ReducedEffectsContext'
import { RetroBackground } from './components/RetroBackground'
import { PageHeader } from './components/PageHeader'
import { LoadingSpinner } from './components/LoadingSpinner'
import { ErrorMessage } from './components/ErrorMessage'
import { SystemStatusCard } from './components/SystemStatusCard'
import { AuthFlow } from './components/AuthFlow'
import { UserInfo } from './components/UserInfo'
import { RemindersCard } from './components/RemindersCard'
import { MemoriesCard } from './components/MemoriesCard'
import { RoomsCard } from './components/RoomsCard'
import { ChatUsageCard } from './components/ChatUsageCard'
import { ChatConfigCard } from './components/ChatConfigCard'
import { MessageActivityCard } from './components/MessageActivityCard'
import { WebhooksCard } from './components/WebhooksCard'
import { DashboardSection } from './components/DashboardSection'
import { DashboardCard } from './components/DashboardCard'
import { ScanlineOverlay } from './components/ui/ScanlineOverlay'
import { TiltCard } from './components/ui/TiltCard'
import { CursorTrail } from './components/ui/CursorTrail'
import { CornerBrackets } from './components/ui/CornerBrackets'
import { Waveform } from './components/ui/Waveform'
import { ReducedEffectsToggle } from './components/ui/ReducedEffectsToggle'

function AppContent() {
  const { data: health, isLoading: healthLoading, error } = useHealthCheck()
  const { data: metrics, isLoading: metricsLoading } = useMetrics()
  const isLoading = healthLoading || metricsLoading
  const interpolatedUptime = useInterpolatedUptime(health?.uptime)
  const { isAuthenticated, isLoading: authLoading } = useAuth()
  const prefersReducedMotion = useReducedMotion()
  const { isReduced, isMinimal } = useReducedEffects()

  return (
    <div className="aurora-bg plasma-bg relative min-h-screen overflow-hidden bg-black">
      {/* WebGL Retro Background */}
      {!isMinimal && <RetroBackground reducedEffects={isReduced} />}

      {/* Reduced effects toggle button */}
      <ReducedEffectsToggle />

      {/* Cursor particle trail effect */}
      {!isMinimal && <CursorTrail />}

      {/* Scanline CRT effect overlay */}
      <ScanlineOverlay intensity="light" />

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
        <div
          className={`w-full max-w-[1800px] ${prefersReducedMotion || isReduced ? '' : 'float-subtle'}`}
        >
          <PageHeader />

          {/* Main Content Card with TiltCard effect and holographic overlay */}
          <TiltCard glowColor="rgba(168, 85, 247, 0.2)" holographic>
            <motion.div
              className="relative overflow-hidden border border-purple-500/30 bg-slate-900/90 shadow-2xl shadow-purple-900/30 backdrop-blur-xl"
              initial={
                prefersReducedMotion ? {} : { opacity: 0, y: 30, scale: 0.98 }
              }
              animate={{ opacity: 1, y: 0, scale: 1 }}
              transition={{
                duration: 0.6,
                delay: 0.2,
                ease: [0.25, 0.1, 0.25, 1],
              }}
            >
              {/* Animated corner brackets */}
              <CornerBrackets
                size={24}
                color="#a855f7"
                glowColor="rgba(168, 85, 247, 0.5)"
              />

              {/* Top accent bar with animation */}
              <motion.div
                className="h-px bg-gradient-to-r from-purple-500 via-blue-500 to-purple-500"
                initial={prefersReducedMotion ? {} : { scaleX: 0 }}
                animate={{ scaleX: 1 }}
                transition={{ duration: 0.8, delay: 0.4 }}
              />

              <div className="p-8 md:p-12">
                {/* Admin-only cards sit with the rest of their topic, and DashboardCard leaves them
                    out for anyone but an admin */}
                <DashboardSection
                  title="Overview"
                  gridClassName="grid-cols-1 xl:grid-cols-3"
                >
                  <DashboardCard className="xl:col-span-2">
                    <AnimatePresence mode="wait">
                      {isLoading && (
                        <motion.div
                          key="loading"
                          initial={prefersReducedMotion ? {} : { opacity: 0 }}
                          animate={{ opacity: 1 }}
                          exit={prefersReducedMotion ? {} : { opacity: 0 }}
                        >
                          <LoadingSpinner />
                        </motion.div>
                      )}

                      {error && (
                        <motion.div
                          key="error"
                          initial={prefersReducedMotion ? {} : { opacity: 0 }}
                          animate={{ opacity: 1 }}
                          exit={prefersReducedMotion ? {} : { opacity: 0 }}
                        >
                          <ErrorMessage message={error.message} />
                        </motion.div>
                      )}

                      {health && !error && (
                        <motion.div
                          key="content"
                          initial={prefersReducedMotion ? {} : { opacity: 0 }}
                          animate={{ opacity: 1 }}
                          exit={prefersReducedMotion ? {} : { opacity: 0 }}
                        >
                          <SystemStatusCard
                            status={health.status}
                            uptime={interpolatedUptime}
                            metrics={metrics}
                          />
                        </motion.div>
                      )}
                    </AnimatePresence>
                  </DashboardCard>

                  <DashboardCard title="Account">
                    <AnimatePresence mode="wait">
                      {authLoading ? (
                        <motion.div
                          key="auth-loading"
                          className="flex items-center gap-2 text-slate-400"
                          initial={prefersReducedMotion ? {} : { opacity: 0 }}
                          animate={{ opacity: 1 }}
                          exit={prefersReducedMotion ? {} : { opacity: 0 }}
                        >
                          <motion.div
                            className="h-4 w-4 rounded-full border-2 border-purple-500 border-t-transparent"
                            animate={{ rotate: 360 }}
                            transition={{
                              duration: 1,
                              repeat: Infinity,
                              ease: 'linear',
                            }}
                          />
                          <span className="font-mono text-sm">
                            Checking auth status...
                          </span>
                        </motion.div>
                      ) : isAuthenticated ? (
                        <motion.div
                          key="user-info"
                          initial={
                            prefersReducedMotion ? {} : { opacity: 0, y: 10 }
                          }
                          animate={{ opacity: 1, y: 0 }}
                          exit={
                            prefersReducedMotion ? {} : { opacity: 0, y: -10 }
                          }
                        >
                          <UserInfo />
                        </motion.div>
                      ) : (
                        <motion.div
                          key="auth-flow"
                          initial={
                            prefersReducedMotion ? {} : { opacity: 0, y: 10 }
                          }
                          animate={{ opacity: 1, y: 0 }}
                          exit={
                            prefersReducedMotion ? {} : { opacity: 0, y: -10 }
                          }
                        >
                          <AuthFlow />
                        </motion.div>
                      )}
                    </AnimatePresence>
                  </DashboardCard>
                </DashboardSection>

                {/* Waveform divider */}
                <div className="my-8 opacity-40">
                  <Waveform
                    height={30}
                    color="#a855f7"
                    status={
                      health?.status === 'ok'
                        ? 'ok'
                        : health?.status === 'degraded'
                          ? 'degraded'
                          : 'error'
                    }
                  />
                </div>

                <AnimatePresence>
                  {isAuthenticated && (
                    <motion.div
                      key="sections"
                      className="space-y-10"
                      initial={prefersReducedMotion ? {} : { opacity: 0 }}
                      animate={{ opacity: 1 }}
                      exit={prefersReducedMotion ? {} : { opacity: 0 }}
                      transition={{ duration: 0.3 }}
                    >
                      <DashboardSection title="Reminders & webhooks">
                        <DashboardCard title="Reminders">
                          <RemindersCard />
                        </DashboardCard>
                        <DashboardCard title="Webhooks" adminOnly>
                          <WebhooksCard />
                        </DashboardCard>
                      </DashboardSection>

                      <DashboardSection title="Chat">
                        <DashboardCard title="Memories">
                          <MemoriesCard />
                        </DashboardCard>
                        <DashboardCard title="Usage" adminOnly>
                          <ChatUsageCard />
                        </DashboardCard>
                        <DashboardCard title="Configuration" adminOnly>
                          <ChatConfigCard />
                        </DashboardCard>
                      </DashboardSection>

                      {/* Aligned to the top, since expanding a room makes its card much taller */}
                      <DashboardSection title="Rooms" alignTop>
                        <DashboardCard title="Rooms">
                          <RoomsCard />
                        </DashboardCard>
                        <DashboardCard title="Message activity" adminOnly>
                          <MessageActivityCard />
                        </DashboardCard>
                      </DashboardSection>
                    </motion.div>
                  )}
                </AnimatePresence>
              </div>

              {/* Bottom accent bar with animation */}
              <motion.div
                className="h-px bg-gradient-to-r from-purple-500/50 via-blue-500/50 to-purple-500/50"
                initial={prefersReducedMotion ? {} : { scaleX: 0 }}
                animate={{ scaleX: 1 }}
                transition={{ duration: 0.8, delay: 0.6 }}
              />
            </motion.div>
          </TiltCard>
        </div>
      </div>
    </div>
  )
}

function App() {
  return (
    <ReducedEffectsProvider>
      <AppContent />
    </ReducedEffectsProvider>
  )
}

export default App
