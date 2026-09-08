// Codex choices match the local harness catalog; Claude aliases track CLI defaults.
// Claude aliases/efforts: https://code.claude.com/docs/en/model-config
const CODEX_MODELS = ['gpt-6-astra', 'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.5', 'gpt-5.4-mini']
const CLAUDE_MODELS = ['sonnet', 'opus', 'haiku']

export function modelChoices(driver?: string) {
  return driver === 'codex' ? CODEX_MODELS : driver === 'claude' ? CLAUDE_MODELS : []
}

export function effortChoices(driver?: string, model?: string) {
  if (driver === 'claude') return model === 'haiku' ? [] : ['low', 'medium', 'high', 'xhigh', 'max']
  if (driver !== 'codex') return []
  const levels = ['low', 'medium', 'high', 'xhigh']
  if (['gpt-6-astra', 'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna'].includes(model || '')) levels.push('max')
  if (['gpt-6-astra', 'gpt-5.6-sol', 'gpt-5.6-terra'].includes(model || '')) levels.push('ultra')
  return levels
}

export function workerOptions(choices: string[], current?: string, configured: string[] = []) {
  return [
    { value: '__harness_default__', label: 'Harness default' },
    ...[...new Set([...choices, ...configured.filter(Boolean), ...(current ? [current] : [])])].map(value => ({
      value, label: choices.includes(value) ? value : `${value} (configured)`,
    })),
  ]
}
