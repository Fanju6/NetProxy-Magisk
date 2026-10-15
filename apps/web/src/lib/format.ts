export function bytes(value: number, locale = 'en'): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = value > 0 ? Math.min(4, Math.floor(Math.log(value) / Math.log(1024))) : 0
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: index === 0 ? 0 : 1 }).format(value / 1024 ** index)} ${units[index]}`
}
export function displayNode(reference: string): string {
  const index = reference.indexOf('/')
  return index < 0 ? reference : reference.slice(index + 1)
}
