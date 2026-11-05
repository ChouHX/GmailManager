import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { AlertCircle, CheckCircle2, Info, TriangleAlert, X } from 'lucide-react'
import { cn } from '@/lib/utils'

type Severity = 'success' | 'error' | 'info' | 'warning'

interface Toast {
  id: number
  message: string
  severity: Severity
}

interface ToastApi {
  toast: (message: string, severity?: Severity) => void
}

const ToastContext = createContext<ToastApi>({ toast: () => undefined })

const ICONS = {
  success: CheckCircle2,
  error: AlertCircle,
  info: Info,
  warning: TriangleAlert,
} as const

const TONES: Record<Severity, string> = {
  success: 'text-emerald-600 dark:text-emerald-400',
  error: 'text-destructive',
  info: 'text-primary',
  warning: 'text-amber-600 dark:text-amber-400',
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([])
  const reduce = useReducedMotion()

  const toast = useCallback((message: string, severity: Severity = 'info') => {
    const id = Date.now() + Math.random()
    setItems((prev) => [...prev.slice(-2), { id, message, severity }])
    window.setTimeout(() => {
      setItems((prev) => prev.filter((t) => t.id !== id))
    }, severity === 'error' ? 6000 : 3400)
  }, [])

  const dismiss = useCallback((id: number) => {
    setItems((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const value = useMemo(() => ({ toast }), [toast])

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed inset-x-0 bottom-5 z-[60] flex flex-col items-center gap-2 px-4">
        <AnimatePresence initial={false}>
          {items.map((item) => {
            const Icon = ICONS[item.severity]
            return (
              <motion.div
                key={item.id}
                layout
                initial={reduce ? { opacity: 0 } : { opacity: 0, y: 12, scale: 0.97 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                exit={reduce ? { opacity: 0 } : { opacity: 0, y: 6, scale: 0.98 }}
                transition={{ type: 'spring', duration: 0.32, bounce: 0.14 }}
                className={cn(
                  'pointer-events-auto flex w-fit max-w-[min(92vw,30rem)] items-start gap-2.5 rounded-xl border border-border bg-popover px-3 py-2 shadow-lg',
                )}
                role="status"
              >
                <Icon className={cn('mt-px size-3.5 shrink-0', TONES[item.severity])} />
                <span className="text-[12px] leading-relaxed text-popover-foreground">
                  {item.message}
                </span>
                <button
                  type="button"
                  onClick={() => dismiss(item.id)}
                  className="ml-1 shrink-0 rounded p-0.5 text-muted-foreground transition hover:bg-accent hover:text-foreground"
                  aria-label="关闭提示"
                >
                  <X className="size-3" />
                </button>
              </motion.div>
            )
          })}
        </AnimatePresence>
      </div>
    </ToastContext.Provider>
  )
}

export function useToast() {
  return useContext(ToastContext)
}
