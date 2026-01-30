import { motion, AnimatePresence } from 'framer-motion'
import type { ReactNode } from 'react'
import { useReducedMotion } from '../../hooks/useReducedMotion'
import { staggerContainer, listItem } from '../../utils/animations'

interface AnimatedListProps<T> {
  items: T[]
  keyExtractor: (item: T) => string | number
  renderItem: (item: T, index: number) => ReactNode
  className?: string
  itemClassName?: string
}

export function AnimatedList<T>({
  items,
  keyExtractor,
  renderItem,
  className = '',
  itemClassName = '',
}: AnimatedListProps<T>) {
  const prefersReducedMotion = useReducedMotion()

  if (prefersReducedMotion) {
    return (
      <div className={className}>
        {items.map((item, index) => (
          <div key={keyExtractor(item)} className={itemClassName}>
            {renderItem(item, index)}
          </div>
        ))}
      </div>
    )
  }

  return (
    <motion.div
      className={className}
      variants={staggerContainer}
      initial="hidden"
      animate="visible"
    >
      <AnimatePresence mode="popLayout">
        {items.map((item, index) => (
          <motion.div
            key={keyExtractor(item)}
            variants={listItem}
            initial="hidden"
            animate="visible"
            exit="exit"
            layout
            className={itemClassName}
          >
            {renderItem(item, index)}
          </motion.div>
        ))}
      </AnimatePresence>
    </motion.div>
  )
}
