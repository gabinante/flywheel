import type React from 'react'

import { cn } from '@/lib/utils'

/**
 * Base shimmer skeleton element with glassmorphism-style gradient sweep.
 * Use `className` to control size (height/width) and shape (rounded-full, etc.).
 */
export function Skeleton({
  className,
  style,
}: {
  className?: string
  style?: React.CSSProperties
}) {
  return (
    <div
      data-slot="skeleton"
      className={cn(
        'relative overflow-hidden rounded-md bg-white/[0.06] backdrop-blur-sm',
        className,
      )}
      style={style}
    >
      <div className="absolute inset-0 animate-shimmer bg-gradient-to-r from-transparent via-white/[0.08] to-transparent" />
    </div>
  )
}

/** Skeleton that mimics a Card with header content. */
export function CardSkeleton({
  lines = 2,
  className,
}: {
  lines?: number
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex flex-col gap-4 rounded-xl border border-white/10 bg-white/[0.03] py-4 backdrop-blur-md',
        className,
      )}
    >
      <div className="flex flex-col gap-1.5 px-4">
        <Skeleton className="h-4 w-2/5" />
        <Skeleton className="h-3 w-1/3" />
      </div>
      {lines > 0 && (
        <div className="flex flex-col gap-2 px-4">
          {Array.from({ length: lines }).map((_, i) => (
            <Skeleton
              key={i}
              className="h-3"
              style={{ width: `${70 + ((i * 17) % 31)}%` }}
            />
          ))}
        </div>
      )}
    </div>
  )
}

/** Skeleton that mimics the ticket list page layout. */
export function TicketsPageSkeleton() {
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <Skeleton className="h-3 w-48" />
        <Skeleton className="h-6 w-24" />
      </div>
      <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-end">
        <div className="flex flex-col gap-1">
          <Skeleton className="h-3 w-20" />
          <Skeleton className="h-8 w-48 rounded-md" />
        </div>
        <div className="flex flex-col gap-1">
          <Skeleton className="h-3 w-12" />
          <Skeleton className="h-8 w-48 rounded-md" />
        </div>
        <div className="flex flex-col gap-1">
          <Skeleton className="h-3 w-24" />
          <Skeleton className="h-8 w-48 rounded-md" />
        </div>
      </div>
      <ul className="flex flex-col gap-3">
        {Array.from({ length: 5 }).map((_, i) => (
          <li key={i}>
            <CardSkeleton lines={0} />
          </li>
        ))}
      </ul>
    </div>
  )
}

/** Skeleton that mimics the project page layout. */
export function ProjectPageSkeleton() {
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <Skeleton className="h-3 w-40" />
        <div className="flex flex-wrap items-center gap-2">
          <Skeleton className="h-6 w-48" />
          <Skeleton className="h-5 w-16 rounded-full" />
        </div>
        <Skeleton className="h-3 w-64" />
      </div>
      <div className="flex flex-wrap gap-2">
        <Skeleton className="h-9 w-20 rounded-md" />
        <Skeleton className="h-9 w-32 rounded-md" />
        <Skeleton className="h-9 w-28 rounded-md" />
      </div>
      <div className="flex flex-col gap-4 rounded-xl border border-white/10 bg-white/[0.03] py-4 backdrop-blur-md">
        <div className="flex flex-col gap-3 px-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="space-y-1.5">
            <Skeleton className="h-4 w-28" />
            <Skeleton className="h-3 w-64" />
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            <Skeleton className="h-8 w-24 rounded-md" />
            <Skeleton className="h-8 w-32 rounded-md" />
          </div>
        </div>
        <div className="flex flex-col gap-2 px-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-14 w-full rounded-lg" />
          ))}
        </div>
      </div>
    </div>
  )
}

/** Skeleton for org/project list pages (list of cards). */
export function ListPageSkeleton({
  title,
  count = 3,
}: {
  title?: boolean
  count?: number
}) {
  return (
    <div className="flex flex-col gap-4">
      {title !== false && (
        <div className="flex flex-col gap-1">
          <Skeleton className="h-3 w-32" />
          <Skeleton className="h-6 w-40" />
        </div>
      )}
      <ul className="flex flex-col gap-3">
        {Array.from({ length: count }).map((_, i) => (
          <li key={i}>
            <CardSkeleton lines={0} />
          </li>
        ))}
      </ul>
    </div>
  )
}

/** Skeleton for detail pages (ticket detail, work stream edit). */
export function DetailPageSkeleton() {
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <Skeleton className="h-3 w-56" />
        <div className="flex flex-wrap items-center gap-2">
          <Skeleton className="h-6 w-64" />
          <Skeleton className="h-5 w-20 rounded-full" />
        </div>
        <Skeleton className="h-3 w-40" />
      </div>
      <CardSkeleton lines={3} />
      <CardSkeleton lines={2} />
      <CardSkeleton lines={4} />
    </div>
  )
}

/** Skeleton for the reviews page. */
export function ReviewsPageSkeleton() {
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <Skeleton className="h-3 w-48" />
        <div className="flex items-center justify-between gap-4">
          <Skeleton className="h-6 w-36" />
          <Skeleton className="h-6 w-20" />
        </div>
      </div>
      <ul className="flex flex-col gap-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <li key={i}>
            <div className="flex flex-col gap-4 rounded-xl border border-white/10 bg-white/[0.03] py-4 backdrop-blur-md">
              <div className="flex items-center gap-2 px-4">
                <Skeleton className="size-5 rounded-full" />
                <Skeleton className="h-4 w-48" />
                <Skeleton className="ml-auto h-3 w-24" />
              </div>
            </div>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** Skeleton for the policy health page. */
export function PolicyHealthSkeleton() {
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <Skeleton className="h-3 w-48" />
        <div className="flex items-center justify-between">
          <Skeleton className="h-6 w-32" />
          <Skeleton className="h-8 w-32 rounded-md" />
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-4">
        <Skeleton className="h-4 w-32" />
        <Skeleton className="h-4 w-36" />
      </div>
      <div className="grid gap-4">
        {Array.from({ length: 2 }).map((_, i) => (
          <div
            key={i}
            className="flex flex-col gap-4 rounded-xl border border-white/10 bg-white/[0.03] py-4 backdrop-blur-md"
          >
            <div className="flex items-center justify-between px-4">
              <div className="flex items-center gap-2">
                <Skeleton className="h-5 w-40" />
                <Skeleton className="h-5 w-16 rounded-full" />
              </div>
              <Skeleton className="h-4 w-20" />
            </div>
            <div className="grid grid-cols-2 gap-3 px-4 sm:grid-cols-4">
              {Array.from({ length: 4 }).map((_, j) => (
                <Skeleton key={j} className="h-16 rounded-lg" />
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

/** Inline skeleton for small loading states within cards. */
export function InlineSkeleton({ className }: { className?: string }) {
  return (
    <div className={cn('flex flex-col gap-2', className)}>
      <Skeleton className="h-3 w-full" />
      <Skeleton className="h-3 w-3/4" />
    </div>
  )
}

/** Skeleton for work streams page. */
export function WorkStreamsPageSkeleton() {
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <Skeleton className="h-3 w-48" />
        <Skeleton className="h-6 w-32" />
        <Skeleton className="h-3 w-64" />
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Skeleton className="h-8 w-28 rounded-md" />
        <Skeleton className="h-8 w-32 rounded-md" />
      </div>
      <div className="flex flex-col gap-4 rounded-xl border border-white/10 bg-white/[0.03] py-4 backdrop-blur-md">
        <div className="flex flex-col gap-3 px-4">
          <Skeleton className="h-4 w-48" />
          <Skeleton className="h-3 w-72" />
          <div className="flex flex-wrap gap-2">
            <Skeleton className="h-8 w-16 rounded-md" />
            <Skeleton className="h-8 w-16 rounded-md" />
            <Skeleton className="h-8 w-12 rounded-md" />
          </div>
        </div>
        <div className="flex flex-col gap-2 px-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-14 w-full rounded-lg" />
          ))}
        </div>
      </div>
    </div>
  )
}

/** Skeleton card for the command center. */
export function SkeletonCard() {
  return (
    <div className="flex flex-col gap-3 rounded-xl border border-white/10 bg-white/[0.03] py-4 backdrop-blur-md p-4">
      <Skeleton className="h-4 w-32" />
      <Skeleton className="h-3 w-48" />
      <Skeleton className="h-12 w-full rounded-lg" />
    </div>
  )
}
