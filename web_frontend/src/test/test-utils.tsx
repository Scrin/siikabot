import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, type RenderOptions } from '@testing-library/react'
import type { ReactElement, ReactNode } from 'react'
import { AuthProvider } from '../context/AuthContext'
import { ReducedEffectsProvider } from '../context/ReducedEffectsContext'

// Create a fresh QueryClient for each test
export function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  })
}

interface WrapperProps {
  children: ReactNode
}

// Full provider wrapper for integration-style tests
function AllProviders({ children }: WrapperProps) {
  const queryClient = createTestQueryClient()
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <ReducedEffectsProvider>{children}</ReducedEffectsProvider>
      </AuthProvider>
    </QueryClientProvider>
  )
}

// Query-only wrapper for testing hooks without auth
export function QueryWrapper({ children }: WrapperProps) {
  const queryClient = createTestQueryClient()
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}

// Custom render with providers
function customRender(ui: ReactElement, options?: Omit<RenderOptions, 'wrapper'>) {
  return render(ui, { wrapper: AllProviders, ...options })
}

export * from '@testing-library/react'
export { customRender as render, AllProviders }
