import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select'

export function Choice({
  label,
  value,
  items,
  onChange,
  disabled = false,
  variant = 'outline',
}: {
  label: string
  value: string
  items: { value: string; label: string }[]
  onChange: (value: string) => void
  disabled?: boolean
  variant?: 'outline' | 'ghost'
}) {
  return (
    <Select
      items={items}
      value={value}
      onValueChange={(next) => {
        if (next !== null) onChange(next)
      }}
      disabled={disabled}
    >
      <SelectTrigger
        aria-label={label}
        className={
          variant === 'ghost'
            ? 'border-transparent bg-transparent shadow-none dark:bg-transparent'
            : undefined
        }
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
