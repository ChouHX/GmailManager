import * as React from 'react'
import { cn } from '@/lib/utils'

function Input({ className, type, ...props }: React.ComponentProps<'input'>) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        'flex h-8 w-full min-w-0 rounded-md border border-input bg-card px-2.5 py-1 text-[13px] shadow-xs transition-[color,box-shadow] outline-none',
        'placeholder:text-muted-foreground/70',
        'file:inline-flex file:h-6 file:border-0 file:bg-transparent file:text-xs file:font-medium',
        'focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/25',
        'aria-invalid:border-destructive aria-invalid:ring-2 aria-invalid:ring-destructive/20',
        'disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
        className,
      )}
      {...props}
    />
  )
}

export { Input }
