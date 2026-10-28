import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useWebhooks, useDeleteWebhook } from '../api/queries'
import type { WebhookResponse } from '../api/types'
import { formatDateTime } from '../utils/dateTime'
import { AnimatedList } from './ui/AnimatedList'
import { CopyButton } from './CopyButton'
import { WebhookForm } from './WebhookForm'
import { useReducedMotion } from '../hooks/useReducedMotion'

/** What `editing` holds while the form that creates a webhook is open */
const NEW_WEBHOOK = 'new'

/** A token just saved. It's shown until dismissed, since only its hash is kept. */
interface SavedToken {
  webhook: WebhookResponse
  token: string
}

export function WebhooksCard() {
  const { data, isLoading, error } = useWebhooks()
  const prefersReducedMotion = useReducedMotion()
  // The webhook whose form is open, NEW_WEBHOOK for the one that creates a webhook, or null
  const [editing, setEditing] = useState<string | null>(null)
  const [savedToken, setSavedToken] = useState<SavedToken | null>(null)

  const handleSaved = (webhook: WebhookResponse, token: string | null) => {
    setEditing(null)
    if (token !== null) {
      setSavedToken({ webhook, token })
    }
  }

  if (isLoading) {
    return (
      <motion.div
        className="flex items-center gap-2 text-slate-400"
        initial={prefersReducedMotion ? {} : { opacity: 0 }}
        animate={{ opacity: 1 }}
      >
        <motion.div
          className="h-4 w-4 rounded-full border-2 border-purple-500 border-t-transparent"
          animate={{ rotate: 360 }}
          transition={{ duration: 1, repeat: Infinity, ease: 'linear' }}
        />
        <span className="font-mono text-sm">Loading webhooks...</span>
      </motion.div>
    )
  }

  if (error) {
    return (
      <motion.div
        className="border border-rose-500/50 bg-rose-950/30 p-4"
        initial={prefersReducedMotion ? {} : { opacity: 0, x: -10 }}
        animate={{ opacity: 1, x: 0 }}
      >
        <span className="text-sm text-rose-400">Failed to load webhooks</span>
      </motion.div>
    )
  }

  const webhooks = data?.webhooks ?? []

  return (
    <div className="space-y-4">
      {/* Header with count and new webhook button */}
      <div className="flex items-center justify-between">
        <span className="font-mono text-xs text-slate-500">
          {webhooks.length} {webhooks.length === 1 ? 'webhook' : 'webhooks'}
        </span>

        {editing !== NEW_WEBHOOK && (
          <motion.button
            className="border border-purple-500/30 bg-purple-500/10 px-3 py-1 font-mono text-xs text-purple-300 transition-all hover:border-purple-500/50 hover:bg-purple-500/20"
            onClick={() => setEditing(NEW_WEBHOOK)}
            whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
            whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
          >
            New webhook
          </motion.button>
        )}
      </div>

      <p className="text-xs text-slate-500">
        A webhook posts what is sent to its URL into its room, as [name]: message, with Markdown
        formatting. Requests carry the webhook's token as a bearer token.
      </p>

      <AnimatePresence>
        {savedToken && (
          <TokenPanel
            key={savedToken.token}
            savedToken={savedToken}
            onDismiss={() => setSavedToken(null)}
          />
        )}
      </AnimatePresence>

      {editing === NEW_WEBHOOK && (
        <WebhookForm onSaved={handleSaved} onCancel={() => setEditing(null)} />
      )}

      {webhooks.length === 0 ? (
        <motion.div
          className="border border-slate-700/50 bg-black/20 p-4"
          initial={prefersReducedMotion ? {} : { opacity: 0 }}
          animate={{ opacity: 1 }}
        >
          <span className="font-mono text-sm text-slate-500">No webhooks yet</span>
        </motion.div>
      ) : (
        <AnimatedList
          items={webhooks}
          keyExtractor={(webhook) => webhook.id}
          className="space-y-3"
          renderItem={(webhook) =>
            editing === webhook.id ? (
              <WebhookForm
                webhook={webhook}
                onSaved={handleSaved}
                onCancel={() => setEditing(null)}
              />
            ) : (
              <WebhookItem webhook={webhook} onEdit={() => setEditing(webhook.id)} />
            )
          }
        />
      )}
    </div>
  )
}

/**
 * Quotes text for a POSIX shell, so a token with characters the shell treats specially still
 * reaches curl as it is
 */
function shellQuote(text: string): string {
  return `'${text.replace(/'/g, `'\\''`)}'`
}

interface TokenPanelProps {
  savedToken: SavedToken
  onDismiss: () => void
}

function TokenPanel({ savedToken, onDismiss }: TokenPanelProps) {
  const prefersReducedMotion = useReducedMotion()
  const { webhook, token } = savedToken
  const curl = `curl -H ${shellQuote(`Authorization: Bearer ${token}`)} --data-binary 'hello there' ${webhook.url}`

  return (
    <motion.div
      className="space-y-3 border border-green-500/30 bg-green-950/20 p-4"
      initial={prefersReducedMotion ? {} : { opacity: 0, y: -10 }}
      animate={{ opacity: 1, y: 0 }}
      exit={prefersReducedMotion ? {} : { opacity: 0, y: -10 }}
    >
      <p className="text-sm text-slate-200">
        Token saved for <span className="font-mono">[{webhook.name}]</span>
      </p>
      <p className="text-xs text-amber-400">
        Copy it now: only its hash is stored, so it can't be shown again.
      </p>

      <div className="flex items-center gap-2">
        <code className="flex-1 overflow-x-auto bg-slate-900/80 px-3 py-2 font-mono text-sm whitespace-nowrap text-green-400">
          {token}
        </code>
        <CopyButton text={token} label="token" />
      </div>

      <p className="text-xs text-slate-500">Send a message:</p>
      <div className="flex items-center gap-2">
        <code className="flex-1 overflow-x-auto bg-slate-900/80 px-3 py-2 font-mono text-xs break-all whitespace-pre-wrap text-green-400">
          {curl}
        </code>
        <CopyButton text={curl} label="curl command" />
      </div>

      <motion.button
        className="border border-slate-500/50 bg-slate-500/20 px-3 py-1 font-mono text-xs text-slate-300 transition-all hover:bg-slate-500/30"
        onClick={onDismiss}
        whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
        whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
      >
        Done
      </motion.button>
    </motion.div>
  )
}

interface WebhookItemProps {
  webhook: WebhookResponse
  onEdit: () => void
}

function WebhookItem({ webhook, onEdit }: WebhookItemProps) {
  const deleteWebhook = useDeleteWebhook()
  const prefersReducedMotion = useReducedMotion()
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)

  const handleDelete = async () => {
    try {
      await deleteWebhook.mutateAsync(webhook.id)
    } catch {
      // Error handled by mutation
    }
  }

  return (
    <motion.div
      className="glow-border-hover border border-purple-500/20 bg-black/30 p-4 transition-all duration-300"
      whileHover={prefersReducedMotion ? {} : { scale: 1.01, x: 4 }}
      transition={{ type: 'spring', stiffness: 400, damping: 17 }}
    >
      <div className="space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <span className="font-mono text-sm text-slate-200">[{webhook.name}]</span>

          <AnimatePresence mode="wait">
            {showDeleteConfirm ? (
              <motion.div
                key="confirm"
                className="flex items-center gap-2"
                initial={prefersReducedMotion ? {} : { opacity: 0, scale: 0.9 }}
                animate={{ opacity: 1, scale: 1 }}
                exit={prefersReducedMotion ? {} : { opacity: 0, scale: 0.9 }}
              >
                <span className="text-xs text-slate-400">Delete this webhook?</span>
                <motion.button
                  className="text-xs text-rose-400 hover:text-rose-300 disabled:opacity-50"
                  onClick={handleDelete}
                  disabled={deleteWebhook.isPending}
                  whileHover={prefersReducedMotion ? {} : { scale: 1.1 }}
                >
                  {deleteWebhook.isPending ? '...' : 'Confirm'}
                </motion.button>
                <motion.button
                  className="text-xs text-slate-400 hover:text-slate-300"
                  onClick={() => setShowDeleteConfirm(false)}
                  whileHover={prefersReducedMotion ? {} : { scale: 1.1 }}
                >
                  Cancel
                </motion.button>
              </motion.div>
            ) : (
              <motion.div
                key="actions"
                className="flex items-center gap-3"
                initial={prefersReducedMotion ? {} : { opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={prefersReducedMotion ? {} : { opacity: 0 }}
              >
                <motion.button
                  className="text-xs text-purple-400 hover:text-purple-300"
                  onClick={onEdit}
                  whileHover={prefersReducedMotion ? {} : { scale: 1.1 }}
                >
                  Edit
                </motion.button>
                <motion.button
                  className="text-xs text-rose-400 hover:text-rose-300"
                  onClick={() => setShowDeleteConfirm(true)}
                  whileHover={prefersReducedMotion ? {} : { scale: 1.1 }}
                >
                  Delete
                </motion.button>
              </motion.div>
            )}
          </AnimatePresence>
        </div>

        <div className="text-xs text-slate-500">
          Posts to:{' '}
          {webhook.room_name ? (
            <span className="text-slate-400">
              {webhook.room_name}{' '}
              <span className="font-mono text-slate-500">({webhook.room_id})</span>
            </span>
          ) : (
            <span className="font-mono text-slate-400">{webhook.room_id}</span>
          )}
        </div>

        {!webhook.bot_in_room && (
          <p className="text-xs text-amber-400">
            The bot has left this room. Nothing can be posted until it is back, or the webhook is
            moved to another room.
          </p>
        )}

        <div className="flex items-center gap-2">
          <code className="flex-1 overflow-x-auto bg-slate-900/80 px-3 py-1 font-mono text-xs whitespace-nowrap text-green-400">
            {webhook.url}
          </code>
          <CopyButton text={webhook.url} label="URL" />
        </div>

        <div className="flex flex-wrap gap-x-4 font-mono text-xs text-purple-400">
          <span>Created {formatDateTime(new Date(webhook.created_at))}</span>
          <span>
            Last used{' '}
            {webhook.last_used_at ? formatDateTime(new Date(webhook.last_used_at)) : 'never'}
          </span>
        </div>

        {deleteWebhook.isError && (
          <p className="text-xs text-rose-400">
            {deleteWebhook.error?.message || 'Failed to delete webhook'}
          </p>
        )}
      </div>
    </motion.div>
  )
}
