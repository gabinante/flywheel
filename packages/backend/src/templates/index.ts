/**
 * Email template renderer stub.
 *
 * Templates will be implemented in a future ticket. This stub satisfies
 * TypeScript module resolution so email.service.ts compiles cleanly.
 */

export function renderTemplate(
  templateId: string,
  variables: Record<string, unknown>
): string {
  // Minimal fallback — will be replaced with real template engine
  const varStr = Object.entries(variables)
    .map(([k, v]) => `<li><strong>${k}:</strong> ${String(v)}</li>`)
    .join('\n');
  return `<h2>${templateId}</h2><ul>${varStr}</ul>`;
}
