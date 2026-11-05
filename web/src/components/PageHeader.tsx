import type { ReactNode } from 'react'
import { motion, useReducedMotion } from 'motion/react'
import { cn } from '@/lib/utils'

interface PageHeaderProps {
  title: string
  subtitle?: string
  actions?: ReactNode
  meta?: ReactNode
  className?: string
}

/** Page title row: stacks vertically on phones, side by side from sm up. */
export function PageHeader({ title, subtitle, actions, meta, className }: PageHeaderProps) {
  const reduce = useReducedMotion()
  return (
    <motion.div
      initial={reduce ? false : { opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.28, ease: [0.23, 1, 0.32, 1] }}
      className={cn(
        'flex flex-col gap-2.5 border-b border-border pb-3 sm:flex-row sm:flex-wrap sm:items-end sm:justify-between sm:gap-3',
        className,
      )}
    >
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-[15px] leading-tight font-semibold tracking-tight">{title}</h1>
          {meta}
        </div>
        {subtitle && (
          <p className="mt-1 text-[11.5px] leading-snug text-muted-foreground">{subtitle}</p>
        )}
      </div>
      {actions && (
        <div className="flex flex-wrap items-center gap-2 [&>*]:flex-1 sm:[&>*]:flex-none">
          {actions}
        </div>
      )}
    </motion.div>
  )
}

/** Section label strip for the top of a card. */
export function SectionLabel({ children, right }: { children: ReactNode; right?: ReactNode }) {
  return (
    <div className="flex items-center justify-between border-b border-border bg-muted/40 px-3 py-1.5">
      <span className="text-[10px] font-semibold tracking-[0.07em] text-muted-foreground uppercase">
        {children}
      </span>
      {right}
    </div>
  )
}

/** Label/value pair used in detail panes. */
export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 items-baseline gap-2">
      <span className="min-w-11 shrink-0 text-[10.5px] text-muted-foreground">{label}</span>
      <span className="min-w-0 text-[11.5px] [overflow-wrap:anywhere]">{children}</span>
    </div>
  )
}

/** Empty placeholder inside tables and panels. */
export function EmptyState({
  title,
  hint,
  action,
  icon,
}: {
  title: string
  hint?: string
  action?: ReactNode
  icon?: ReactNode
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-1.5 px-4 py-10 text-center">
      {icon && <div className="mb-1 text-muted-foreground/45">{icon}</div>}
      <p className="text-[12.5px] font-medium text-foreground/80">{title}</p>
      {hint && <p className="max-w-sm text-[11px] leading-relaxed text-muted-foreground">{hint}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  )
}
