import * as React from 'react'
import { Select } from 'radix-ui'
import { ChevronDown, Check } from 'lucide-react'
import { cn } from '@/lib/utils'

interface StyledSelectOption {
  value: string
  label: string
  icon?: React.ReactNode
}

interface StyledSelectProps {
  id?: string
  value: string
  onValueChange: (value: string) => void
  options: StyledSelectOption[]
  placeholder?: string
  disabled?: boolean
  className?: string
  'aria-label'?: string
}

export function StyledSelect({
  id,
  value,
  onValueChange,
  options,
  placeholder = 'Select…',
  disabled = false,
  className,
  'aria-label': ariaLabel,
}: StyledSelectProps) {
  return (
    <Select.Root value={value} onValueChange={onValueChange} disabled={disabled}>
      <Select.Trigger
        id={id}
        aria-label={ariaLabel}
        className={cn(
          'inline-flex h-8 min-w-[12rem] items-center justify-between gap-2 rounded-lg',
          'border border-white/10 bg-white/5 px-3 text-sm text-foreground backdrop-blur-md',
          'transition-all duration-200',
          'hover:bg-white/10 hover:border-white/20',
          'focus:outline-none focus:ring-2 focus:ring-green-500/40 focus:border-green-500/40',
          'disabled:cursor-not-allowed disabled:opacity-50',
          'data-[placeholder]:text-muted-foreground',
          className,
        )}
      >
        <Select.Value placeholder={placeholder} />
        <Select.Icon>
          <ChevronDown className="size-3.5 text-muted-foreground" />
        </Select.Icon>
      </Select.Trigger>

      <Select.Portal>
        <Select.Content
          position="popper"
          sideOffset={4}
          className={cn(
            'z-50 min-w-[var(--radix-select-trigger-width)] overflow-hidden rounded-lg',
            'border border-white/10 bg-card/90 backdrop-blur-xl',
            'animate-in fade-in-0 zoom-in-95',
            'data-[side=bottom]:slide-in-from-top-2',
            'data-[side=top]:slide-in-from-bottom-2',
          )}
        >
          <Select.Viewport className="p-1">
            {options.map((opt) => (
              <Select.Item
                key={opt.value}
                value={opt.value}
                className={cn(
                  'relative flex h-8 cursor-pointer select-none items-center gap-2 rounded-md px-2 pr-8 text-sm',
                  'outline-none transition-colors duration-150',
                  'data-[highlighted]:bg-white/10 data-[highlighted]:text-foreground',
                  'data-[state=checked]:text-green-400',
                  'focus:bg-white/10',
                )}
              >
                {opt.icon ? (
                  <span className="flex size-4 shrink-0 items-center justify-center">
                    {opt.icon}
                  </span>
                ) : null}
                <Select.ItemText>{opt.label}</Select.ItemText>
                <Select.ItemIndicator className="absolute right-2 flex items-center">
                  <Check className="size-3.5 text-green-400" />
                </Select.ItemIndicator>
              </Select.Item>
            ))}
          </Select.Viewport>
        </Select.Content>
      </Select.Portal>
    </Select.Root>
  )
}
