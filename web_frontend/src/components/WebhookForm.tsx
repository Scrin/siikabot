import { useId, useState, type FormEvent } from 'react'
import { motion } from 'framer-motion'
import { useAdminRooms, useCreateWebhook, useUpdateWebhook } from '../api/queries'
import type { UpdateWebhookRequest, WebhookResponse } from '../api/types'
import {
  WEBHOOK_TOKEN_MIN_LENGTH,
  generateWebhookToken,
  validateWebhookName,
  validateWebhookToken,
} from '../utils/webhookToken'
import { CopyButton } from './CopyButton'
import { useReducedMotion } from '../hooks/useReducedMotion'

interface WebhookFormProps {
  /** The webhook to edit, or undefined for the form that creates one */
  webhook?: WebhookResponse
  /** Called once the webhook is saved, with the token if one was set */
  onSaved: (webhook: WebhookResponse, token: string | null) => void
  onCancel: () => void
}

const labelClass = 'block font-mono text-xs text-slate-400'
const inputClass =
  'w-full border border-slate-600/50 bg-slate-900/80 px-3 py-2 font-mono text-sm text-slate-200 placeholder:text-slate-600 focus:border-purple-500 focus:outline-none'

function roomLabel(room: { room_id: string; room_name?: string }): string {
  return room.room_name ? `${room.room_name} (${room.room_id})` : room.room_id
}

/**
 * The form that creates a webhook, or edits one. Editing sends only what changed, so a webhook
 * keeps its token unless a new one is given, and one whose room the bot has left can be renamed
 * without being moved.
 */
export function WebhookForm({ webhook, onSaved, onCancel }: WebhookFormProps) {
  const fieldId = useId()
  const prefersReducedMotion = useReducedMotion()
  const { data: roomsData, isLoading: roomsLoading } = useAdminRooms()
  const createWebhook = useCreateWebhook()
  const updateWebhook = useUpdateWebhook()
  const mutation = webhook ? updateWebhook : createWebhook

  const [name, setName] = useState(webhook?.name ?? '')
  const [roomId, setRoomId] = useState(webhook?.room_id ?? '')
  const [token, setToken] = useState('')
  // The problems with the fields are shown once the admin has tried to save
  const [showErrors, setShowErrors] = useState(false)

  // The rooms the bot is in. The webhook's own room is offered too when it isn't among them, as
  // when the bot has left it, so that saving other changes doesn't move the webhook.
  const rooms = roomsData?.rooms ?? []
  const currentRoomMissing = webhook && !rooms.some((room) => room.room_id === webhook.room_id)

  const nameError = validateWebhookName(name)
  const roomError = roomId === '' ? 'The room is required' : null
  let tokenError: string | null = null
  if (token !== '') {
    tokenError = validateWebhookToken(token)
  } else if (!webhook) {
    tokenError = 'The token is required'
  }

  const changes: UpdateWebhookRequest = {}
  if (webhook) {
    if (name.trim() !== webhook.name) changes.name = name.trim()
    if (roomId !== webhook.room_id) changes.room_id = roomId
    if (token !== '') changes.token = token
  }
  const unchanged = webhook !== undefined && Object.keys(changes).length === 0

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (nameError || roomError || tokenError) {
      setShowErrors(true)
      return
    }
    try {
      if (webhook) {
        if (unchanged) return
        const saved = await updateWebhook.mutateAsync({
          id: webhook.id,
          changes,
        })
        onSaved(saved.webhook, changes.token ?? null)
      } else {
        const saved = await createWebhook.mutateAsync({
          name: name.trim(),
          room_id: roomId,
          token,
        })
        onSaved(saved.webhook, token)
      }
    } catch {
      // Error handled by mutation
    }
  }

  return (
    <motion.form
      className="space-y-4 border border-purple-500/30 bg-black/30 p-4"
      onSubmit={handleSubmit}
      noValidate
      initial={prefersReducedMotion ? {} : { opacity: 0, y: -10 }}
      animate={{ opacity: 1, y: 0 }}
    >
      <h4 className="font-mono text-xs tracking-wider text-purple-400 uppercase">
        {webhook ? 'Edit webhook' : 'New webhook'}
      </h4>

      <div className="space-y-1">
        <label htmlFor={`${fieldId}-name`} className={labelClass}>
          Name
        </label>
        <input
          id={`${fieldId}-name`}
          type="text"
          className={inputClass}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="hermes"
          autoComplete="off"
        />
        <p className="text-xs text-slate-500">
          Every message starts with it: [{name.trim() || 'name'}]: message
        </p>
        {showErrors && nameError && <p className="text-xs text-rose-400">{nameError}</p>}
      </div>

      <div className="space-y-1">
        <label htmlFor={`${fieldId}-room`} className={labelClass}>
          Room
        </label>
        <select
          id={`${fieldId}-room`}
          className={inputClass}
          value={roomId}
          onChange={(e) => setRoomId(e.target.value)}
        >
          {!webhook && (
            <option value="">{roomsLoading ? 'Loading rooms...' : 'Choose a room'}</option>
          )}
          {currentRoomMissing && (
            <option value={webhook.room_id}>
              {roomLabel(webhook)}
              {!webhook.bot_in_room && ' (the bot has left this room)'}
            </option>
          )}
          {rooms.map((room) => (
            <option key={room.room_id} value={room.room_id}>
              {roomLabel(room)}
            </option>
          ))}
        </select>
        {webhook && !webhook.bot_in_room && roomId === webhook.room_id && (
          <p className="text-xs text-amber-400">
            The bot has left this room. Nothing can be posted until it is back, or the webhook is
            moved to another room.
          </p>
        )}
        {showErrors && roomError && <p className="text-xs text-rose-400">{roomError}</p>}
      </div>

      <div className="space-y-1">
        <label htmlFor={`${fieldId}-token`} className={labelClass}>
          Token
        </label>
        <div className="flex items-center gap-2">
          <input
            id={`${fieldId}-token`}
            type="text"
            className={inputClass}
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder={webhook ? 'Leave empty to keep the current token' : 'Type or generate one'}
            autoComplete="off"
            spellCheck={false}
          />
          <motion.button
            type="button"
            className="shrink-0 border border-purple-500/50 bg-purple-500/20 px-3 py-1 font-mono text-xs text-purple-300 transition-all hover:bg-purple-500/30"
            onClick={() => setToken(generateWebhookToken())}
            whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
            whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
          >
            Generate
          </motion.button>
          <CopyButton text={token} label="token" disabled={token === ''} />
        </div>
        <p className="text-xs text-slate-500">
          {webhook
            ? 'Leave empty to keep the current token. A new one replaces it at once.'
            : `At least ${WEBHOOK_TOKEN_MIN_LENGTH} characters, without spaces.`}{' '}
          Only its hash is stored, so it can't be shown again once saved.
        </p>
        {showErrors && tokenError && <p className="text-xs text-rose-400">{tokenError}</p>}
      </div>

      {mutation.isError && (
        <p className="text-xs text-rose-400">
          {mutation.error?.message || 'Failed to save webhook'}
        </p>
      )}

      <div className="flex items-center gap-2">
        <motion.button
          type="submit"
          className="border border-purple-500/50 bg-purple-500/20 px-3 py-1 font-mono text-xs text-purple-300 transition-all hover:bg-purple-500/30 disabled:cursor-not-allowed disabled:opacity-50"
          disabled={mutation.isPending || unchanged}
          whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
          whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
        >
          {mutation.isPending ? 'Saving...' : webhook ? 'Save' : 'Create'}
        </motion.button>
        <motion.button
          type="button"
          className="border border-slate-500/50 bg-slate-500/20 px-3 py-1 font-mono text-xs text-slate-300 transition-all hover:bg-slate-500/30"
          onClick={onCancel}
          whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
          whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
        >
          Cancel
        </motion.button>
      </div>
    </motion.form>
  )
}
