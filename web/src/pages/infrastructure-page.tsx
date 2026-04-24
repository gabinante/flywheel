import { Server } from 'lucide-react'

import { CatalogEntityList } from '@/components/infrastructure/catalog-entity-list'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

export function InfrastructurePage() {
  return (
    <div className="mx-auto flex max-w-[1440px] flex-col gap-5 animate-in fade-in duration-300">
      {/* Header */}
      <div className="space-y-2">
        <div className="flex items-center gap-2">
          <Server className="size-5 text-muted-foreground" />
          <h1 className="text-2xl font-semibold tracking-tight">
            Infrastructure
          </h1>
        </div>
        <p className="max-w-3xl text-sm leading-relaxed text-muted-foreground">
          Catalog entities, deployment pipeline, and state health.
        </p>
      </div>

      {/* Catalog */}
      <CatalogEntityList />

      {/* Pipeline */}
      <Card className="border-white/10 bg-white/5 backdrop-blur-md">
        <CardHeader className="border-b border-white/10 pb-4">
          <CardTitle className="text-sm">Pipeline</CardTitle>
          <CardDescription>
            Deployment pipeline stages and current run status.
          </CardDescription>
        </CardHeader>
        <CardContent className="py-8 text-center">
          <p className="text-sm text-muted-foreground">
            Pipeline data will appear here.
          </p>
        </CardContent>
      </Card>

      {/* State Health */}
      <Card className="border-white/10 bg-white/5 backdrop-blur-md">
        <CardHeader className="border-b border-white/10 pb-4">
          <CardTitle className="text-sm">State Health</CardTitle>
          <CardDescription>
            Drift detection and resource state overview.
          </CardDescription>
        </CardHeader>
        <CardContent className="py-8 text-center">
          <p className="text-sm text-muted-foreground">
            State health data will appear here.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
