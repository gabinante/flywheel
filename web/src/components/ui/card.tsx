import * as React from 'react'

import { cn } from '@/lib/utils'

function Card({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card"
      className={cn(
        'flex flex-col gap-4 rounded-xl border border-border bg-card py-4 text-card-foreground shadow-sm',
        'transition-all duration-200 ease-out',
        'hover:scale-[1.015] hover:shadow-md hover:shadow-primary/5 hover:border-primary/20',
        'focus-within:scale-[1.015] focus-within:shadow-md focus-within:border-primary/20',
        className,
      )}
      {...props}
    />
  )
}

function CardHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-header"
      className={cn(
        '@container/card-header grid auto-rows-min grid-rows-[auto_auto] items-start gap-1.5 px-4 has-data-[slot=card-action]:grid-cols-[1fr_auto] [.border-b]:pb-4',
        className,
      )}
      {...props}
    />
  )
}

function CardTitle({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-title"
      className={cn('font-medium text-sm', className)}
      {...props}
    />
  )
}

function CardDescription({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-description"
      className={cn('text-muted-foreground text-xs', className)}
      {...props}
    />
  )
}

function CardContent({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div data-slot="card-content" className={cn('px-4', className)} {...props} />
  )
}

export { Card, CardContent, CardDescription, CardHeader, CardTitle }
