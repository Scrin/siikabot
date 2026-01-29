import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  type ReactNode,
} from 'react'
import { fetchCurrentUser, logout as apiLogout, AuthError } from '../api/client'

const AUTH_TOKEN_KEY = 'siikabot_auth_token'

interface AuthState {
  isAuthenticated: boolean
  isLoading: boolean
  userId: string | null
  token: string | null
}

interface AuthContextValue extends AuthState {
  login: (token: string, userId: string) => void
  logout: () => void
  validateToken: () => Promise<boolean>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({
    isAuthenticated: false,
    isLoading: true,
    userId: null,
    token: null,
  })

  // Validate token on mount
  useEffect(() => {
    const storedToken = localStorage.getItem(AUTH_TOKEN_KEY)

    if (storedToken) {
      // Validate the token and fetch user ID from API
      fetchCurrentUser(storedToken)
        .then((response) => {
          setState({
            isAuthenticated: true,
            isLoading: false,
            userId: response.user_id,
            token: storedToken,
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
            })
          } else {
            // Server error or network issue - keep token, assume authenticated
            // User can retry or the next request will validate
            setState({
              isAuthenticated: true,
              isLoading: false,
              userId: null, // Unknown due to error
              token: storedToken,
            })
          }
        })
    } else {
      setState({
        isAuthenticated: false,
        isLoading: false,
        userId: null,
        token: null,
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
    })
  }, [])

  const logout = useCallback(async () => {
    const token = localStorage.getItem(AUTH_TOKEN_KEY)

    // Clear local state first for immediate UI feedback
    localStorage.removeItem(AUTH_TOKEN_KEY)
    setState({
      isAuthenticated: false,
      isLoading: false,
      userId: null,
      token: null,
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
      setState((prev) => ({
        ...prev,
        isAuthenticated: true,
        userId: response.user_id,
        token: storedToken,
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

  return (
    <AuthContext.Provider
      value={{
        ...state,
        login,
        logout,
        validateToken,
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
