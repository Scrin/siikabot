export function LoadingSpinner() {
  return (
    <div className="flex flex-col items-center justify-center py-12">
      <div className="flex gap-1">
        <div className="h-8 w-2 animate-pulse bg-purple-500" style={{ animationDelay: '0ms' }} />
        <div className="h-8 w-2 animate-pulse bg-purple-400" style={{ animationDelay: '150ms' }} />
        <div className="h-8 w-2 animate-pulse bg-blue-500" style={{ animationDelay: '300ms' }} />
        <div className="h-8 w-2 animate-pulse bg-blue-400" style={{ animationDelay: '450ms' }} />
      </div>
      <p className="mt-6 text-sm tracking-wider text-blue-300/60">
        Initializing systems...
      </p>
    </div>
  )
}
