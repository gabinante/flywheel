import * as React from 'react'

import { cn } from '@/lib/utils'

function Textarea({ className, ...props }: React.ComponentProps<'textarea'>) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        'min-h-[120px] w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-foreground',
        'placeholder:text-muted-foreground/60',
        'backdrop-blur-sm transition-colors duration-200',
        'focus:border-primary/50 focus:bg-white/[0.07] focus:outline-none focus:ring-1 focus:ring-primary/30',
        'disabled:cursor-not-allowed disabled:opacity-50',
        'resize-y',
        className,
      )}
      {...props}
    />
  )
}

export { Textarea }
