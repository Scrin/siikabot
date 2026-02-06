import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  type ReactNode,
} from 'react'
import { fetchCurrentUser, logout as apiLogout, AuthError } from '../api/client'
import type { Authorizations } from '../api/types'

const AUTH_TOKEN_KEY = 'siikabot_auth_token'
const ADMIN_MODE_KEY = 'siikabot_admin_mode'

interface AuthState {
  isAuthenticated: boolean
  isLoading: boolean
  userId: string | null
  token: string | null
  authorizations: Authorizations | null
  adminMode: boolean
  isAdmin: boolean
}

interface AuthContextValue extends AuthState {
  login: (token: string, userId: string) => void
  logout: () => void
  validateToken: () => Promise<boolean>
  setAdminMode: (enabled: boolean) => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({
    isAuthenticated: false,
    isLoading: true,
    userId: null,
    token: null,
    authorizations: null,
    adminMode: false,
    isAdmin: false,
  })

  // Load admin mode from localStorage when admin status changes
  useEffect(() => {
    const storedAdminMode = localStorage.getItem(ADMIN_MODE_KEY)
    if (storedAdminMode === 'true' && state.isAdmin) {
      setState((prev) => ({ ...prev, adminMode: true }))
    }
  }, [state.isAdmin])

  // Validate token on mount
  useEffect(() => {
    const storedToken = localStorage.getItem(AUTH_TOKEN_KEY)

    if (storedToken) {
      // Validate the token and fetch user ID from API
      fetchCurrentUser(storedToken)
        .then((response) => {
          const storedAdminMode = localStorage.getItem(ADMIN_MODE_KEY)
          setState({
            isAuthenticated: true,
            isLoading: false,
            userId: response.user_id,
            token: storedToken,
            authorizations: response.authorizations,
            adminMode: storedAdminMode === 'true' && response.authorizations.admin,
            isAdmin: response.authorizations.admin,
          })
        })
        .catch((error) => {
          // Only clear token if it's definitively invalid (401)
          // Don't clear on server errors (500) or network issues
          if (error instanceof AuthError) {
            localStorage.removeItem(AUTH_TOKEN_KEY)
            setState({
              isAuthenticated: false,
              isLoading: false,
              userId: null,
              token: null,
              authorizations: null,
              adminMode: false,
              isAdmin: false,
            })
          } else {
            // Server error or network issue - keep token, assume authenticated
            // User can retry or the next request will validate
            setState({
              isAuthenticated: true,
              isLoading: false,
              userId: null, // Unknown due to error
              token: storedToken,
              authorizations: null,
              adminMode: false,
              isAdmin: false,
            })
          }
        })
    } else {
      setState({
        isAuthenticated: false,
        isLoading: false,
        userId: null,
        token: null,
        authorizations: null,
        adminMode: false,
        isAdmin: false,
      })
    }
  }, [])

  const login = useCallback((token: string, userId: string) => {
    localStorage.setItem(AUTH_TOKEN_KEY, token)
    setState({
      isAuthenticated: true,
      isLoading: false,
      userId,
      token,
      authorizations: null,
      adminMode: false,
      isAdmin: false,
    })
    // Fetch authorizations after login
    fetchCurrentUser(token)
      .then((response) => {
        const storedAdminMode = localStorage.getItem(ADMIN_MODE_KEY)
        setState((prev) => ({
          ...prev,
          authorizations: response.authorizations,
          isAdmin: response.authorizations.admin,
          adminMode: storedAdminMode === 'true' && response.authorizations.admin,
        }))
      })
      .catch(() => {
        // Ignore errors - user is already logged in, authorizations can be fetched later
      })
  }, [])

  const logout = useCallback(async () => {
    const token = localStorage.getItem(AUTH_TOKEN_KEY)

    // Clear local state first for immediate UI feedback
    localStorage.removeItem(AUTH_TOKEN_KEY)
    localStorage.removeItem(ADMIN_MODE_KEY)
    setState({
      isAuthenticated: false,
      isLoading: false,
      userId: null,
      token: null,
      authorizations: null,
      adminMode: false,
      isAdmin: false,
    })

    // Then clear the token from the server (fire and forget)
    if (token) {
      apiLogout(token).catch(() => {
        // Ignore errors - local logout already succeeded
      })
    }
  }, [])

  const validateToken = useCallback(async (): Promise<boolean> => {
    const storedToken = localStorage.getItem(AUTH_TOKEN_KEY)
    if (!storedToken) {
      return false
    }

    try {
      const response = await fetchCurrentUser(storedToken)
      const storedAdminMode = localStorage.getItem(ADMIN_MODE_KEY)
      setState((prev) => ({
        ...prev,
        isAuthenticated: true,
        userId: response.user_id,
        token: storedToken,
        authorizations: response.authorizations,
        adminMode: storedAdminMode === 'true' && response.authorizations.admin,
        isAdmin: response.authorizations.admin,
      }))
      return true
    } catch (error) {
      // Only logout if token is definitively invalid (401)
      if (error instanceof AuthError) {
        logout()
        return false
      }
      // Server error - don't clear token, return true to keep user logged in
      return true
    }
  }, [logout])

  const setAdminMode = useCallback(
    (enabled: boolean) => {
      if (!state.isAdmin && enabled) {
        // Can't enable admin mode if not admin
        return
      }
      localStorage.setItem(ADMIN_MODE_KEY, enabled.toString())
      setState((prev) => ({ ...prev, adminMode: enabled }))
    },
    [state.isAdmin]
  )

  return (
    <AuthContext.Provider
      value={{
        ...state,
        login,
        logout,
        validateToken,
        setAdminMode,
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
}
