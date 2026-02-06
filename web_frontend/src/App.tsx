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
import { AdminRoomsCard } from './components/AdminRoomsCard'
import { GrafanaCard } from './components/GrafanaCard'
import { ScanlineOverlay } from './components/ui/ScanlineOverlay'
import { TiltCard } from './components/ui/TiltCard'
import { FadeInSection } from './components/ui/FadeInSection'
import { CursorTrail } from './components/ui/CursorTrail'
import { CornerBrackets } from './components/ui/CornerBrackets'
import { Waveform } from './components/ui/Waveform'
import { ReducedEffectsToggle } from './components/ui/ReducedEffectsToggle'

function AppContent() {
  const { data: health, isLoading: healthLoading, error } = useHealthCheck()
  const { data: metrics, isLoading: metricsLoading } = useMetrics()
  const isLoading = healthLoading || metricsLoading
  const interpolatedUptime = useInterpolatedUptime(health?.uptime)
  const { isAuthenticated, isLoading: authLoading, authorizations, adminMode, isAdmin } =
    useAuth()
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
          className={`w-full max-w-4xl ${prefersReducedMotion || isReduced ? '' : 'float-subtle'}`}
        >
          <PageHeader />

          {/* Admin mode indicator banner */}
          {isAdmin && adminMode && (
            <motion.div
              className="mb-4 border border-purple-500/50 bg-purple-900/20 px-4 py-2 text-center"
              initial={prefersReducedMotion ? {} : { opacity: 0, y: -10 }}
              animate={{ opacity: 1, y: 0 }}
            >
              <span className="font-mono text-sm text-purple-300 uppercase tracking-wider">
                Admin Mode Active
              </span>
            </motion.div>
          )}

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

                {/* Waveform divider */}
                <div className="my-6 opacity-40">
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

                {/* Auth Section */}
                <FadeInSection delay={0.3}>
                  <div className="border-t border-slate-700/50 pt-6">
                    <motion.h3
                      className="mb-4 font-mono text-sm tracking-wider text-slate-500 uppercase"
                      initial={
                        prefersReducedMotion ? {} : { opacity: 0, x: -10 }
                      }
                      animate={{ opacity: 1, x: 0 }}
                      transition={{ delay: 0.4 }}
                    >
                      Authentication
                    </motion.h3>
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
                  </div>
                </FadeInSection>

                {/* NORMAL MODE SECTIONS - Only show when NOT in admin mode */}
                <AnimatePresence>
                  {isAuthenticated && !adminMode && (
                    <motion.div
                      initial={prefersReducedMotion ? {} : { opacity: 0 }}
                      animate={{ opacity: 1 }}
                      exit={prefersReducedMotion ? {} : { opacity: 0 }}
                      transition={{ duration: 0.3 }}
                    >
                      {/* Reminders Section */}
                      <FadeInSection delay={0.4}>
                        <div className="mt-6 border-t border-slate-700/50 pt-6">
                          <motion.h3
                            className="mb-4 font-mono text-sm tracking-wider text-slate-500 uppercase"
                            initial={
                              prefersReducedMotion ? {} : { opacity: 0, x: -10 }
                            }
                            animate={{ opacity: 1, x: 0 }}
                            transition={{ delay: 0.5 }}
                          >
                            Active Reminders
                          </motion.h3>
                          <RemindersCard />
                        </div>
                      </FadeInSection>

                      {/* Memories Section */}
                      <FadeInSection delay={0.45}>
                        <div className="mt-6 border-t border-slate-700/50 pt-6">
                          <motion.h3
                            className="mb-4 font-mono text-sm tracking-wider text-slate-500 uppercase"
                            initial={
                              prefersReducedMotion ? {} : { opacity: 0, x: -10 }
                            }
                            animate={{ opacity: 1, x: 0 }}
                            transition={{ delay: 0.55 }}
                          >
                            Chat Memories
                          </motion.h3>
                          <MemoriesCard />
                        </div>
                      </FadeInSection>

                      {/* Grafana Section - only if authorized */}
                      {authorizations?.grafana && (
                        <FadeInSection delay={0.6}>
                          <div className="mt-6 border-t border-slate-700/50 pt-6">
                            <motion.h3
                              className="mb-4 font-mono text-sm tracking-wider text-slate-500 uppercase"
                              initial={
                                prefersReducedMotion ? {} : { opacity: 0, x: -10 }
                              }
                              animate={{ opacity: 1, x: 0 }}
                              transition={{ delay: 0.7 }}
                            >
                              Grafana Templates
                            </motion.h3>
                            <GrafanaCard />
                          </div>
                        </FadeInSection>
                      )}

                      {/* Known Rooms Section */}
                      <FadeInSection delay={0.65}>
                        <div className="mt-6 border-t border-slate-700/50 pt-6">
                          <motion.h3
                            className="mb-4 font-mono text-sm tracking-wider text-slate-500 uppercase"
                            initial={
                              prefersReducedMotion ? {} : { opacity: 0, x: -10 }
                            }
                            animate={{ opacity: 1, x: 0 }}
                            transition={{ delay: 0.75 }}
                          >
                            Known rooms
                          </motion.h3>
                          <RoomsCard />
                        </div>
                      </FadeInSection>
                    </motion.div>
                  )}
                </AnimatePresence>

                {/* ADMIN MODE SECTIONS - Only show when in admin mode */}
                <AnimatePresence>
                  {isAuthenticated && adminMode && (
                    <motion.div
                      initial={prefersReducedMotion ? {} : { opacity: 0 }}
                      animate={{ opacity: 1 }}
                      exit={prefersReducedMotion ? {} : { opacity: 0 }}
                      transition={{ duration: 0.3 }}
                    >
                      {/* All Known Rooms Section */}
                      <FadeInSection delay={0.4}>
                        <div className="mt-6 border-t border-slate-700/50 pt-6">
                          <motion.h3
                            className="mb-4 font-mono text-sm tracking-wider text-purple-400 uppercase"
                            initial={
                              prefersReducedMotion ? {} : { opacity: 0, x: -10 }
                            }
                            animate={{ opacity: 1, x: 0 }}
                            transition={{ delay: 0.5 }}
                          >
                            All Known Rooms (Admin)
                          </motion.h3>
                          <AdminRoomsCard />
                        </div>
                      </FadeInSection>

                      {/* Future admin sections can be added here */}
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
