import { cn } from '@/lib/utils'

/** Minimal accessible pagination bar used by the account tables. */
interface PaginationProps {
  page: number
  pageCount: number
  onChange: (page: number) => void
  className?: string
}

function pageList(page: number, pageCount: number): (number | 'gap')[] {
  if (pageCount <= 7) return Array.from({ length: pageCount }, (_, i) => i + 1)
  const out: (number | 'gap')[] = [1]
  const start = Math.max(2, page - 1)
  const end = Math.min(pageCount - 1, page + 1)
  if (start > 2) out.push('gap')
  for (let i = start; i <= end; i++) out.push(i)
  if (end < pageCount - 1) out.push('gap')
  out.push(pageCount)
  return out
}

export function Pagination({ page, pageCount, onChange, className }: PaginationProps) {
  const items = pageList(page, pageCount)
  const base =
    'inline-flex h-6 min-w-6 items-center justify-center rounded px-1.5 text-[11.5px] font-medium transition-colors disabled:pointer-events-none disabled:opacity-40'

  return (
    <nav className={cn('flex items-center gap-1', className)} aria-label="分页">
      <button
        type="button"
        className={cn(base, 'text-muted-foreground hover:bg-accent')}
        onClick={() => onChange(page - 1)}
        disabled={page <= 1}
        aria-label="上一页"
      >
        上一页
      </button>
      {items.map((it, i) =>
        it === 'gap' ? (
          <span key={`gap-${i}`} className={cn(base, 'text-muted-foreground')}>
            …
          </span>
        ) : (
          <button
            key={it}
            type="button"
            onClick={() => onChange(it)}
            aria-current={it === page ? 'page' : undefined}
            className={cn(
              base,
              it === page
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-accent',
            )}
          >
            {it}
          </button>
        ),
      )}
      <button
        type="button"
        className={cn(base, 'text-muted-foreground hover:bg-accent')}
        onClick={() => onChange(page + 1)}
        disabled={page >= pageCount}
        aria-label="下一页"
      >
        下一页
      </button>
    </nav>
  )
}
