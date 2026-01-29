interface ErrorMessageProps {
  message: string
}

export function ErrorMessage({ message }: ErrorMessageProps) {
  return (
    <div className="border border-rose-500/50 bg-rose-950/30 p-5">
      <div className="flex items-center gap-2">
        <div className="h-2 w-2 animate-pulse bg-rose-500" />
        <span className="text-sm font-semibold tracking-wide text-rose-400">
          System Error
        </span>
      </div>
      <p className="mt-2 text-sm text-rose-300/80">{message}</p>
    </div>
  )
}
