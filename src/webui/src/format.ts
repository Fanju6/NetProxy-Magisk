/** 将模块命令返回的 JSON 展开为便于终端阅读的格式。 */
export function formatCtlOutput(output: string): string {
  const trimmed = output.trim()
  if (!trimmed) return output

  try {
    const value: unknown = JSON.parse(trimmed)
    if (typeof value !== 'object' || value === null) return output
    return `${JSON.stringify(value, null, 2)}\n`
  } catch {
    return output
  }
}
