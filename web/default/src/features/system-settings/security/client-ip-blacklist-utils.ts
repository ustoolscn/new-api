export function splitIPRules(value: string): string[] {
  const seen = new Set<string>()
  const rules: string[] = []

  for (const line of value.split('\n')) {
    const rule = line.trim()
    if (!rule || seen.has(rule)) continue
    seen.add(rule)
    rules.push(rule)
  }

  return rules
}
