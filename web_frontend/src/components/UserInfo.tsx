import { useAuth } from '../context/AuthContext'
import type { Authorizations } from '../api/types'

export function UserInfo() {
  const { userId, authorizations, logout } = useAuth()

  if (!userId) return null

  // Define all available permissions
  const allPermissions: Array<{ key: keyof Authorizations; label: string }> = [
    { key: 'grafana', label: 'Grafana' },
  ]

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2">
          <div className="h-2 w-2 rounded-full bg-green-500" />
          <span className="font-mono text-sm text-slate-300">{userId}</span>
        </div>
        <button
          onClick={logout}
          className="font-mono text-xs text-slate-500 transition-colors hover:text-red-400"
        >
          Logout
        </button>
      </div>

      {/* Permissions section */}
      <div className="flex items-start gap-2 text-slate-400">
        <span className="font-mono text-xs">Permissions:</span>
        <div className="flex flex-wrap gap-2">
          {allPermissions.map(({ key, label }) => {
            const isGranted = authorizations?.[key] ?? false
            return (
              <div
                key={key}
                className="flex items-center gap-1.5 rounded bg-slate-800/50 px-2 py-1"
              >
                <div
                  className={`h-1.5 w-1.5 rounded-full ${
                    isGranted ? 'bg-green-500' : 'bg-red-500'
                  }`}
                />
                <span
                  className={`font-mono text-xs ${
                    isGranted ? 'text-slate-300' : 'text-slate-500'
                  }`}
                >
                  {label}
                </span>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
