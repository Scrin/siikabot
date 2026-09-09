// TanStack Query hooks

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchHealthCheck,
  fetchMetrics,
  fetchReminders,
  fetchRooms,
  fetchAdminRooms,
  fetchRoomMembers,
  fetchAdminRoomMembers,
  fetchMemories,
  deleteMemory,
  deleteAllMemories,
  fetchChatUsage,
} from './client'
import { useAuth } from '../context/AuthContext'

/**
 * Query keys for cache management
 */
export const queryKeys = {
  healthCheck: ['healthCheck'] as const,
  metrics: ['metrics'] as const,
  reminders: ['reminders'] as const,
  rooms: ['rooms'] as const,
  adminRooms: ['adminRooms'] as const,
  roomMembers: (roomId: string) => ['roomMembers', roomId] as const,
  adminRoomMembers: (roomId: string) => ['adminRoomMembers', roomId] as const,
  memories: ['memories'] as const,
  chatUsage: (days: number) => ['chatUsage', days] as const,
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
 * Hook to fetch system metrics
 * Auto-refetches every 10 seconds (configured in QueryClient)
 */
export function useMetrics() {
  return useQuery({
    queryKey: queryKeys.metrics,
    queryFn: fetchMetrics,
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

/**
 * Hook to fetch all rooms known to the bot (admin only)
 * Only fetches when user is authenticated and has admin authorization
 */
export function useAdminRooms() {
  const { token, isAuthenticated, authorizations } = useAuth()

  return useQuery({
    queryKey: queryKeys.adminRooms,
    queryFn: () => fetchAdminRooms(token!),
    enabled: isAuthenticated && !!token && authorizations?.admin === true,
    refetchInterval: 30000,
  })
}

/**
 * Hook to fetch user's memories
 * Only fetches when user is authenticated
 */
export function useMemories() {
  const { token, isAuthenticated } = useAuth()

  return useQuery({
    queryKey: queryKeys.memories,
    queryFn: () => fetchMemories(token!),
    enabled: isAuthenticated && !!token,
    refetchInterval: 30000,
  })
}

/**
 * Hook to delete a specific memory
 */
export function useDeleteMemory() {
  const { token } = useAuth()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (memoryId: number) => deleteMemory(token!, memoryId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.memories })
    },
  })
}

/**
 * Hook to delete all memories
 */
export function useDeleteAllMemories() {
  const { token } = useAuth()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: () => deleteAllMemories(token!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.memories })
    },
  })
}

/**
 * Hook to fetch members of a specific room
 * Only fetches when enabled (room is expanded)
 */
export function useRoomMembers(roomId: string, enabled: boolean) {
  const { token, isAuthenticated } = useAuth()

  return useQuery({
    queryKey: queryKeys.roomMembers(roomId),
    queryFn: () => fetchRoomMembers(token!, roomId),
    enabled: enabled && isAuthenticated && !!token,
    staleTime: 30000,
  })
}

/**
 * Hook to fetch members of any room (admin only)
 * Only fetches when enabled (room is expanded)
 */
export function useAdminRoomMembers(roomId: string, enabled: boolean) {
  const { token, isAuthenticated, authorizations } = useAuth()

  return useQuery({
    queryKey: queryKeys.adminRoomMembers(roomId),
    queryFn: () => fetchAdminRoomMembers(token!, roomId),
    enabled: enabled && isAuthenticated && !!token && authorizations?.admin === true,
    staleTime: 30000,
  })
}

/**
 * Per-room chat usage over the given number of days (admin only)
 */
export function useChatUsage(days: number) {
  const { token, isAuthenticated, authorizations } = useAuth()

  return useQuery({
    queryKey: queryKeys.chatUsage(days),
    queryFn: () => fetchChatUsage(token!, days),
    enabled: isAuthenticated && !!token && authorizations?.admin === true,
    refetchInterval: 60000,
  })
}
