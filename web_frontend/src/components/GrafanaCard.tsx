import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useGrafanaTemplates, useCreateGrafanaTemplate } from '../api/queries'
import { AnimatedList } from './ui/AnimatedList'
import { useReducedMotion } from '../hooks/useReducedMotion'
import { GrafanaTemplateEditor } from './GrafanaTemplateEditor'

export function GrafanaCard() {
  const { data, isLoading, error } = useGrafanaTemplates()
  const createTemplate = useCreateGrafanaTemplate()
  const [showAddForm, setShowAddForm] = useState(false)
  const [newTemplateName, setNewTemplateName] = useState('')
  const prefersReducedMotion = useReducedMotion()

  const handleCreate = async () => {
    if (!newTemplateName.trim()) return
    try {
      await createTemplate.mutateAsync({ name: newTemplateName.trim(), template: '' })
      setNewTemplateName('')
      setShowAddForm(false)
    } catch {
      // Error is handled by the mutation
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
        <span className="font-mono text-sm">Loading templates...</span>
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
        <span className="text-sm text-rose-400">Failed to load templates</span>
      </motion.div>
    )
  }

  const templates = data?.templates ?? []

  return (
    <div className="space-y-4">
      {/* Add template button/form */}
      <div className="flex items-center gap-4">
        <motion.button
          className="border border-purple-500/30 bg-purple-500/10 px-4 py-2 font-mono text-sm text-purple-400 transition-all hover:border-purple-500/50 hover:bg-purple-500/20"
          onClick={() => setShowAddForm(!showAddForm)}
          whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
          whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
        >
          {showAddForm ? 'Cancel' : '+ Add Template'}
        </motion.button>
      </div>

      {/* New template form */}
      <AnimatePresence>
        {showAddForm && (
          <motion.div
            className="border border-slate-700/50 bg-black/30 p-4"
            initial={prefersReducedMotion ? {} : { opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={prefersReducedMotion ? {} : { opacity: 0, height: 0 }}
          >
            <div className="space-y-3">
              <div>
                <label className="mb-1 block font-mono text-xs text-slate-500">
                  Template Name
                </label>
                <input
                  type="text"
                  value={newTemplateName}
                  onChange={(e) => setNewTemplateName(e.target.value)}
                  placeholder="my-template"
                  className="w-full border border-slate-700 bg-black/50 p-2 font-mono text-sm text-slate-300 placeholder-slate-600 focus:border-purple-500 focus:outline-none"
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') handleCreate()
                  }}
                />
              </div>
              <motion.button
                className="border border-purple-500/50 bg-purple-500/20 px-4 py-2 font-mono text-sm text-purple-300 transition-all hover:bg-purple-500/30 disabled:cursor-not-allowed disabled:opacity-50"
                onClick={handleCreate}
                disabled={createTemplate.isPending || !newTemplateName.trim()}
                whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
                whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
              >
                {createTemplate.isPending ? 'Creating...' : 'Create Template'}
              </motion.button>
              {createTemplate.isError && (
                <p className="text-xs text-rose-400">
                  {createTemplate.error?.message || 'Failed to create template'}
                </p>
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Templates list */}
      {templates.length === 0 ? (
        <motion.div
          className="border border-slate-700/50 bg-black/20 p-4"
          initial={prefersReducedMotion ? {} : { opacity: 0 }}
          animate={{ opacity: 1 }}
        >
          <span className="font-mono text-sm text-slate-500">
            No Grafana templates configured
          </span>
        </motion.div>
      ) : (
        <AnimatedList
          items={templates}
          keyExtractor={(t) => t.name}
          className="space-y-3"
          renderItem={(template) => <GrafanaTemplateEditor template={template} />}
        />
      )}
    </div>
  )
}
