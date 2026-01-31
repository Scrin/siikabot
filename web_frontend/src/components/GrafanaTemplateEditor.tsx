import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import {
  useUpdateGrafanaTemplate,
  useDeleteGrafanaTemplate,
  useSetGrafanaDatasource,
  useDeleteGrafanaDatasource,
  useRenderGrafanaTemplate,
} from '../api/queries'
import type { GrafanaTemplate, GrafanaDatasource } from '../api/types'
import { useReducedMotion } from '../hooks/useReducedMotion'
import { HtmlPreview } from './HtmlPreview'

interface GrafanaTemplateEditorProps {
  template: GrafanaTemplate
}

export function GrafanaTemplateEditor({
  template,
}: GrafanaTemplateEditorProps) {
  const [isExpanded, setIsExpanded] = useState(false)
  const [editedTemplate, setEditedTemplate] = useState(template.template)
  const [isEditing, setIsEditing] = useState(false)
  const updateTemplate = useUpdateGrafanaTemplate()
  const deleteTemplate = useDeleteGrafanaTemplate()
  const renderQuery = useRenderGrafanaTemplate(template.name, isExpanded)
  const prefersReducedMotion = useReducedMotion()

  const handleSave = async () => {
    try {
      await updateTemplate.mutateAsync({
        name: template.name,
        template: editedTemplate,
      })
      setIsEditing(false)
      // Refresh preview with new template
      renderQuery.refetch()
    } catch {
      // Error is handled by the mutation
    }
  }

  const handleDelete = async () => {
    if (
      !confirm(
        `Delete template "${template.name}"? This will also delete all its datasources.`,
      )
    ) {
      return
    }
    try {
      await deleteTemplate.mutateAsync(template.name)
    } catch {
      // Error is handled by the mutation
    }
  }

  return (
    <motion.div
      className="glow-border-hover border border-purple-500/20 bg-black/30 transition-all duration-300"
      whileHover={prefersReducedMotion ? {} : { scale: 1.005, x: 2 }}
      transition={{ type: 'spring', stiffness: 400, damping: 17 }}
    >
      {/* Header - clickable to expand */}
      <div
        className="flex cursor-pointer items-center justify-between p-4"
        onClick={() => setIsExpanded(!isExpanded)}
      >
        <span className="font-mono text-sm text-slate-200">
          {template.name}
        </span>
        <div className="flex items-center gap-3">
          <span className="text-xs text-slate-500">
            {template.datasources.length} datasource
            {template.datasources.length !== 1 ? 's' : ''}
          </span>
          <motion.span
            className="text-slate-400"
            animate={{ rotate: isExpanded ? 180 : 0 }}
            transition={{ duration: 0.2 }}
          >
            ▼
          </motion.span>
        </div>
      </div>

      {/* Expanded content */}
      <AnimatePresence>
        {isExpanded && (
          <motion.div
            initial={prefersReducedMotion ? {} : { height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={prefersReducedMotion ? {} : { height: 0, opacity: 0 }}
            transition={{ duration: 0.2 }}
            className="overflow-hidden border-t border-slate-700/50"
          >
            <div className="space-y-4 p-4">
              {/* Template text area */}
              <div>
                <label className="mb-2 block font-mono text-xs text-slate-500">
                  Template String (Go template syntax)
                </label>
                <textarea
                  value={editedTemplate}
                  onChange={(e) => {
                    setEditedTemplate(e.target.value)
                    if (e.target.value !== template.template) {
                      setIsEditing(true)
                    }
                  }}
                  className="h-32 w-full resize-y border border-slate-700 bg-black/50 p-3 font-mono text-sm text-slate-300 placeholder-slate-600 focus:border-purple-500 focus:outline-none"
                  placeholder="Enter Go template string, e.g., Temperature: {{.temp}}°C"
                />
              </div>

              {/* Save/Cancel buttons */}
              <AnimatePresence>
                {isEditing && (
                  <motion.div
                    className="flex gap-2"
                    initial={prefersReducedMotion ? {} : { opacity: 0, y: -10 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={prefersReducedMotion ? {} : { opacity: 0, y: -10 }}
                  >
                    <motion.button
                      className="border border-green-500/50 bg-green-500/20 px-4 py-2 font-mono text-sm text-green-300 transition-all hover:bg-green-500/30 disabled:cursor-not-allowed disabled:opacity-50"
                      onClick={handleSave}
                      disabled={updateTemplate.isPending}
                      whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
                      whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
                    >
                      {updateTemplate.isPending ? 'Saving...' : 'Save Template'}
                    </motion.button>
                    <motion.button
                      className="border border-slate-500/50 bg-slate-500/20 px-4 py-2 font-mono text-sm text-slate-300 transition-all hover:bg-slate-500/30"
                      onClick={() => {
                        setEditedTemplate(template.template)
                        setIsEditing(false)
                      }}
                      whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
                      whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
                    >
                      Cancel
                    </motion.button>
                  </motion.div>
                )}
              </AnimatePresence>
              {updateTemplate.isError && (
                <p className="text-xs text-rose-400">
                  {updateTemplate.error?.message || 'Failed to update template'}
                </p>
              )}

              {/* Preview panel */}
              <div>
                <label className="mb-2 block font-mono text-xs text-slate-500">
                  Rendered Preview
                </label>
                <HtmlPreview
                  html={renderQuery.data?.rendered || ''}
                  isLoading={renderQuery.isLoading}
                  error={renderQuery.error}
                />
              </div>

              {/* Datasources section */}
              <DatasourcesEditor
                templateName={template.name}
                datasources={template.datasources}
              />

              {/* Delete template button */}
              <div className="border-t border-slate-700/50 pt-4">
                <motion.button
                  className="border border-rose-500/50 bg-rose-500/20 px-4 py-2 font-mono text-sm text-rose-300 transition-all hover:bg-rose-500/30 disabled:cursor-not-allowed disabled:opacity-50"
                  onClick={handleDelete}
                  disabled={deleteTemplate.isPending}
                  whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
                  whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
                >
                  {deleteTemplate.isPending ? 'Deleting...' : 'Delete Template'}
                </motion.button>
                {deleteTemplate.isError && (
                  <p className="mt-2 text-xs text-rose-400">
                    {deleteTemplate.error?.message ||
                      'Failed to delete template'}
                  </p>
                )}
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </motion.div>
  )
}

interface DatasourcesEditorProps {
  templateName: string
  datasources: GrafanaDatasource[]
}

function DatasourcesEditor({
  templateName,
  datasources,
}: DatasourcesEditorProps) {
  const [showAddForm, setShowAddForm] = useState(false)
  const [newName, setNewName] = useState('')
  const [newUrl, setNewUrl] = useState('')
  const setDatasource = useSetGrafanaDatasource()
  const prefersReducedMotion = useReducedMotion()

  const handleAdd = async () => {
    if (!newName.trim() || !newUrl.trim()) return
    try {
      await setDatasource.mutateAsync({
        templateName,
        datasourceName: newName.trim(),
        url: newUrl.trim(),
      })
      setNewName('')
      setNewUrl('')
      setShowAddForm(false)
    } catch {
      // Error is handled by the mutation
    }
  }

  return (
    <div>
      <div className="mb-2 flex items-center justify-between">
        <label className="font-mono text-xs text-slate-500">Datasources</label>
        <motion.button
          className="border border-purple-500/30 bg-purple-500/10 px-3 py-1 font-mono text-xs text-purple-400 transition-all hover:border-purple-500/50 hover:bg-purple-500/20"
          onClick={() => setShowAddForm(!showAddForm)}
          whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
          whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
        >
          {showAddForm ? 'Cancel' : '+ Add'}
        </motion.button>
      </div>

      {/* Add datasource form */}
      <AnimatePresence>
        {showAddForm && (
          <motion.div
            className="mb-3 space-y-2 border border-slate-700 bg-black/30 p-3"
            initial={prefersReducedMotion ? {} : { opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={prefersReducedMotion ? {} : { opacity: 0, height: 0 }}
          >
            <input
              type="text"
              placeholder="Datasource name (e.g., temp)"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              className="w-full border border-slate-700 bg-black/50 p-2 font-mono text-sm text-slate-300 placeholder-slate-600 focus:border-purple-500 focus:outline-none"
            />
            <input
              type="text"
              placeholder="Grafana query URL"
              value={newUrl}
              onChange={(e) => setNewUrl(e.target.value)}
              className="w-full border border-slate-700 bg-black/50 p-2 font-mono text-sm text-slate-300 placeholder-slate-600 focus:border-purple-500 focus:outline-none"
            />
            <div className="flex gap-2">
              <motion.button
                className="border border-green-500/50 bg-green-500/20 px-3 py-1 font-mono text-sm text-green-300 transition-all hover:bg-green-500/30 disabled:cursor-not-allowed disabled:opacity-50"
                onClick={handleAdd}
                disabled={
                  setDatasource.isPending || !newName.trim() || !newUrl.trim()
                }
                whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
                whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
              >
                {setDatasource.isPending ? 'Adding...' : 'Add'}
              </motion.button>
            </div>
            {setDatasource.isError && (
              <p className="text-xs text-rose-400">
                {setDatasource.error?.message || 'Failed to add datasource'}
              </p>
            )}
          </motion.div>
        )}
      </AnimatePresence>

      {/* Datasources list */}
      {datasources.length === 0 ? (
        <div className="text-xs text-slate-500">No datasources configured</div>
      ) : (
        <div className="space-y-2">
          {datasources.map((ds) => (
            <DatasourceItem
              key={ds.name}
              templateName={templateName}
              datasource={ds}
            />
          ))}
        </div>
      )}
    </div>
  )
}

interface DatasourceItemProps {
  templateName: string
  datasource: GrafanaDatasource
}

function DatasourceItem({ templateName, datasource }: DatasourceItemProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [editedUrl, setEditedUrl] = useState(datasource.url)
  const setDatasource = useSetGrafanaDatasource()
  const deleteDatasource = useDeleteGrafanaDatasource()
  const prefersReducedMotion = useReducedMotion()

  const handleSave = async () => {
    if (!editedUrl.trim()) return
    try {
      await setDatasource.mutateAsync({
        templateName,
        datasourceName: datasource.name,
        url: editedUrl.trim(),
      })
      setIsEditing(false)
    } catch {
      // Error is handled by the mutation
    }
  }

  const handleDelete = async () => {
    try {
      await deleteDatasource.mutateAsync({
        templateName,
        datasourceName: datasource.name,
      })
    } catch {
      // Error is handled by the mutation
    }
  }

  return (
    <motion.div
      className="flex items-center gap-2 border border-slate-700/30 bg-black/20 p-2"
      whileHover={prefersReducedMotion ? {} : { x: 2 }}
    >
      <span className="shrink-0 font-mono text-xs text-purple-400">
        {datasource.name}
      </span>
      <span className="text-slate-600">=</span>
      {isEditing ? (
        <>
          <input
            value={editedUrl}
            onChange={(e) => setEditedUrl(e.target.value)}
            className="min-w-0 flex-1 border border-slate-700 bg-black/50 p-1 font-mono text-xs text-slate-300 focus:border-purple-500 focus:outline-none"
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleSave()
              if (e.key === 'Escape') {
                setEditedUrl(datasource.url)
                setIsEditing(false)
              }
            }}
            autoFocus
          />
          <motion.button
            className="shrink-0 text-xs text-green-400 hover:text-green-300 disabled:opacity-50"
            onClick={handleSave}
            disabled={setDatasource.isPending}
            whileHover={prefersReducedMotion ? {} : { scale: 1.1 }}
          >
            {setDatasource.isPending ? '...' : 'Save'}
          </motion.button>
          <motion.button
            className="shrink-0 text-xs text-slate-400 hover:text-slate-300"
            onClick={() => {
              setEditedUrl(datasource.url)
              setIsEditing(false)
            }}
            whileHover={prefersReducedMotion ? {} : { scale: 1.1 }}
          >
            Cancel
          </motion.button>
        </>
      ) : (
        <>
          <span
            className="min-w-0 flex-1 truncate font-mono text-xs text-slate-400"
            title={datasource.url}
          >
            {datasource.url}
          </span>
          <motion.button
            className="shrink-0 text-xs text-blue-400 hover:text-blue-300"
            onClick={() => setIsEditing(true)}
            whileHover={prefersReducedMotion ? {} : { scale: 1.1 }}
          >
            Edit
          </motion.button>
          <motion.button
            className="shrink-0 text-xs text-rose-400 hover:text-rose-300 disabled:opacity-50"
            onClick={handleDelete}
            disabled={deleteDatasource.isPending}
            whileHover={prefersReducedMotion ? {} : { scale: 1.1 }}
          >
            {deleteDatasource.isPending ? '...' : 'Delete'}
          </motion.button>
        </>
      )}
    </motion.div>
  )
}
