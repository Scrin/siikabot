import { useMemo } from 'react'
import { sanitizeHtml } from '../utils/htmlSanitizer'

interface HtmlPreviewProps {
  html: string
  isLoading?: boolean
  error?: Error | null
}

export function HtmlPreview({ html, isLoading, error }: HtmlPreviewProps) {
  const sanitizedHtml = useMemo(() => sanitizeHtml(html), [html])

  if (isLoading) {
    return (
      <div className="html-preview-container flex items-center justify-center py-8">
        <div className="flex items-center gap-2 text-slate-400">
          <svg
            className="h-4 w-4 animate-spin"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
          >
            <circle
              className="opacity-25"
              cx="12"
              cy="12"
              r="10"
              strokeWidth="4"
            />
            <path
              className="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            />
          </svg>
          <span className="font-mono text-xs">Loading preview...</span>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="html-preview-container border-rose-500/30 bg-rose-500/5">
        <div className="flex items-center gap-2 text-rose-400">
          <span className="font-mono text-xs">Error: {error.message}</span>
        </div>
      </div>
    )
  }

  if (!html || !sanitizedHtml) {
    return (
      <div className="html-preview-container">
        <span className="font-mono text-xs text-slate-500">No preview available</span>
      </div>
    )
  }

  return (
    <div
      className="html-preview-container"
      dangerouslySetInnerHTML={{ __html: sanitizedHtml }}
    />
  )
}
