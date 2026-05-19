import { useEffect, useState } from 'react'
import { Info, Minus, Plus, Save } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']

interface KeyFile {
  path: string
  snippet: string
}

export function ProjectBasePromptCard({
  projectId,
  project,
  onProjectChange,
}: {
  projectId: string
  project: Project
  onProjectChange: (project: Project) => void
}) {
  const { client } = useAuth()
  const cp = project.context_pack as
    | { system_prompt?: string; conventions?: string; key_files?: KeyFile[] }
    | undefined

  const [systemPrompt, setSystemPrompt] = useState(cp?.system_prompt ?? '')
  const [conventions, setConventions] = useState(cp?.conventions ?? '')
  const [keyFiles, setKeyFiles] = useState<KeyFile[]>(cp?.key_files ?? [])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [savedAt, setSavedAt] = useState<number | null>(null)

  useEffect(() => {
    const pack = project.context_pack as typeof cp | undefined
    setSystemPrompt(pack?.system_prompt ?? '')
    setConventions(pack?.conventions ?? '')
    setKeyFiles(pack?.key_files ?? [])
  }, [project.context_pack])

  async function save() {
    setSaving(true)
    setError(null)
    setSavedAt(null)
    const { data, error: apiError, response } = await client.PATCH(
      '/projects/{projectID}',
      {
        params: { path: { projectID: projectId } },
        body: {
          context_pack: {
            system_prompt: systemPrompt,
            conventions,
            key_files: keyFiles.filter((f) => f.path.trim() !== ''),
          },
        },
      },
    )
    if (!response.ok || !data) {
      setError(formatApiError(apiError))
      setSaving(false)
      return
    }
    onProjectChange(data)
    setSavedAt(Date.now())
    setSaving(false)
  }

  function addKeyFile() {
    setKeyFiles([...keyFiles, { path: '', snippet: '' }])
  }

  function removeKeyFile(index: number) {
    setKeyFiles(keyFiles.filter((_, i) => i !== index))
  }

  function updateKeyFile(index: number, field: keyof KeyFile, value: string) {
    setKeyFiles(keyFiles.map((f, i) => (i === index ? { ...f, [field]: value } : f)))
  }

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader>
        <CardTitle className="text-sm">Base prompt</CardTitle>
        <CardDescription>
          Project-wide instructions injected into all worker prompts. These apply before
          role-specific or workflow phase instructions.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <section className="space-y-2">
          <Label htmlFor="system-prompt">System prompt</Label>
          <Textarea
            id="system-prompt"
            value={systemPrompt}
            onChange={(e) => setSystemPrompt(e.target.value)}
            rows={6}
            placeholder="Project-wide behavioral rules for all workers (e.g. coding standards, security policies, response style)."
          />
        </section>

        <section className="space-y-2">
          <Label htmlFor="conventions">Conventions</Label>
          <Textarea
            id="conventions"
            value={conventions}
            onChange={(e) => setConventions(e.target.value)}
            rows={4}
            placeholder="Coding standards, naming conventions, architectural patterns to follow."
          />
        </section>

        <section className="space-y-3">
          <div className="flex items-center justify-between">
            <Label>Key files</Label>
            <Button type="button" size="xs" variant="ghost" onClick={addKeyFile}>
              <Plus className="size-3.5" />
              Add file
            </Button>
          </div>
          {keyFiles.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              No key files configured. Add file paths and optional snippets that workers should reference.
            </p>
          ) : (
            <div className="space-y-2">
              {keyFiles.map((file, i) => (
                <div key={i} className="flex items-start gap-2">
                  <div className="grid flex-1 gap-2 sm:grid-cols-2">
                    <Input
                      value={file.path}
                      onChange={(e) => updateKeyFile(i, 'path', e.target.value)}
                      placeholder="path/to/file.ts"
                    />
                    <Input
                      value={file.snippet}
                      onChange={(e) => updateKeyFile(i, 'snippet', e.target.value)}
                      placeholder="Snippet or description (optional)"
                    />
                  </div>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="size-8 shrink-0"
                    onClick={() => removeKeyFile(i)}
                  >
                    <Minus className="size-3.5" />
                  </Button>
                </div>
              ))}
            </div>
          )}
        </section>

        <div className="flex items-center gap-2">
          <Button size="xs" onClick={() => void save()} disabled={saving}>
            <Save className="size-3.5" />
            {saving ? 'Saving…' : 'Save'}
          </Button>
          {savedAt ? (
            <span className="text-xs text-emerald-400">Saved</span>
          ) : null}
          {error ? (
            <span className="text-xs text-destructive">{error}</span>
          ) : null}
        </div>

        <section className="rounded-xl border border-white/10 bg-black/10 p-4">
          <div className="mb-2 flex items-center gap-2">
            <Info className="size-3.5 text-muted-foreground" />
            <span className="text-xs font-medium text-muted-foreground">
              Prompt composition order
            </span>
          </div>
          <ol className="list-inside list-decimal space-y-0.5 text-xs text-muted-foreground">
            <li>Worker type preamble (built-in)</li>
            <li>System prompt (this page)</li>
            <li>Conventions (this page)</li>
            <li>Key files (this page)</li>
            <li>Ticket details (automatic)</li>
            <li>Role instructions (Workers &amp; roles settings)</li>
            <li>Phase prompt (Workflow stages, if set)</li>
          </ol>
        </section>
      </CardContent>
    </Card>
  )
}
