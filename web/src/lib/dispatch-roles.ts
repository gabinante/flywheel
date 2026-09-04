/** Built-in worker roles the dispatcher understands. */
export const ROLE_OPTIONS = [
  {
    key: 'orchestrator',
    label: 'Command Center',
    description: 'Conversation planning and ticket creation in the orchestrator.',
  },
  {
    key: 'planner',
    label: 'Planning',
    description: 'Ticket planning and investigation before implementation starts.',
  },
  {
    key: 'executor',
    label: 'Implementation',
    description: 'Main code-writing workers for claimed and executing tickets.',
  },
  {
    key: 'validator',
    label: 'Review',
    description: 'PR review, validation, and acceptance decisions.',
  },
  {
    key: 'deployer',
    label: 'Deployment',
    description: 'Deployment and post-merge operational work.',
  },
  {
    key: 'investigator',
    label: 'Investigation',
    description: 'Read-only investigation subagents.',
  },
  {
    key: 'conflict_resolver',
    label: 'Conflict Resolution',
    description: 'Merge/rebase resolution when auto-merge hits conflicts.',
  },
] as const

export type BuiltInRoleKey = (typeof ROLE_OPTIONS)[number]['key']
