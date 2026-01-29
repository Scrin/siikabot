import { useAuth } from '../context/AuthContext'

export function UserInfo() {
  const { userId, logout } = useAuth()

  if (!userId) return null

  return (
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
  )
}
