// TanStack Query hooks

import { useQuery } from '@tanstack/react-query'
import { fetchHealthCheck, fetchReminders, fetchRooms } from './client'
import { useAuth } from '../context/AuthContext'

/**
 * Query keys for cache management
 */
export const queryKeys = {
  healthCheck: ['healthCheck'] as const,
  reminders: ['reminders'] as const,
  rooms: ['rooms'] as const,
}

/**
 * Hook to fetch health check status
 * Auto-refetches every 10 seconds (configured in QueryClient)
 */
export function useHealthCheck() {
  return useQuery({
    queryKey: queryKeys.healthCheck,
    queryFn: fetchHealthCheck,
  })
}

/**
 * Hook to fetch user's active reminders
 * Only fetches when user is authenticated
 */
export function useReminders() {
  const { token, isAuthenticated } = useAuth()

  return useQuery({
    queryKey: queryKeys.reminders,
    queryFn: () => fetchReminders(token!),
    enabled: isAuthenticated && !!token,
    refetchInterval: 30000,
  })
}

/**
 * Hook to fetch rooms shared between the bot and the current user
 * Only fetches when user is authenticated
 */
export function useRooms() {
  const { token, isAuthenticated } = useAuth()

  return useQuery({
    queryKey: queryKeys.rooms,
    queryFn: () => fetchRooms(token!),
    enabled: isAuthenticated && !!token,
    refetchInterval: 30000,
  })
}
