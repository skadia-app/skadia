import { useQuery } from '@tanstack/react-query'
import type { ComponentStatus } from '@/api/systemStatus'
import { fetchSystemStatus } from '@/api/systemStatus'
import { SettingsSection } from '@/components/ui/SettingsSection'
import { frontendBuildInfo } from '@/lib/buildInfo'

export default function AdminSystemStatusTab() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['admin-system-status'],
    queryFn: fetchSystemStatus,
    staleTime: 15_000,
  })

  if (isLoading) return <div className="text-sm text-neutral-500">Status wird geladen…</div>
  if (isError || !data) return <div className="text-sm text-red-500">Systemstatus konnte nicht geladen werden.</div>

  const build = {
    ...data.build,
    frontend: data.build.frontend ?? frontendBuildInfo(),
  }

  return (
    <div className="space-y-6">
      <SettingsSection title="Systemstatus" description="Laufzeitstatus, Build-Herkunft und Diagnosehinweise.">
        <div className="space-y-2 text-sm">
          <div>Gesamtstatus: {data.overall}</div>
          <div className="text-xs text-neutral-500 dark:text-neutral-400">
            Frontend-Build: {build.frontend.git_commit} · Commit-Zeit: {build.frontend.commit_time}
          </div>
        </div>
      </SettingsSection>
      <SettingsSection title="Datenebene" description="Datenbank, Speicher, Jobverarbeitung und DSGVO-Artefakte.">
        <div>
          <StatusRow title="Datenbank" component={data.components.db} />
          <StatusRow title="TimescaleDB-Kompression" component={data.components.timescale_compression} />
          <StatusRow title="Speicher" component={data.components.storage} />
          <StatusRow title="Jobs" component={data.components.jobs} />
          <StatusRow title="DSGVO-Artefakte" component={data.components.gdpr_artifacts} />
        </div>
      </SettingsSection>
      <SettingsSection title="Dienste und Nachrichten" description="API-nahe Dienste, ML und Queue-Verarbeitung.">
        <div>
          <StatusRow title="API" component={data.components.api} />
          <StatusRow title="ML-Dienst" component={data.components.ml} />
          <StatusRow title="NATS / JetStream" component={data.components.nats} />
        </div>
      </SettingsSection>
      <SettingsSection title="Routing und Geodaten" description="Routing-Engines, lokale Geodaten, Höhenmodell, Coverage und Kartendateien.">
        <div>
          <StatusRow title="BRouter" component={data.components.brouter} />
          <StatusRow title="Skadia-Routinggraph" component={data.components.badrouter} />
          <StatusRow title="OSM-Geodaten" component={data.components.osm} />
          <StatusRow title="Höhendaten" component={data.components.dem} />
          <StatusRow title="Coverage" component={data.components.coverage} />
          <StatusRow title="Kartendateien" component={data.components.tiles} />
        </div>
      </SettingsSection>
      {data.feature_matrix && (
        <SettingsSection title="Feature-Matrix" description="Konfigurierte und aktuell verfügbare Funktionsbereiche.">
          <div>
            {Object.values(data.feature_matrix).map((component) => (
              <StatusRow key={component.key} title={component.label || component.key} component={component} />
            ))}
          </div>
        </SettingsSection>
      )}
    </div>
  )
}

function StatusRow({ title, component }: { title: string; component?: ComponentStatus }) {
  const state = component?.state ?? 'unknown'
  const reason = component?.reason_code
  return (
    <div className="flex items-start justify-between gap-4 border-b border-neutral-200 py-3 last:border-b-0 dark:border-neutral-800">
      <div className="min-w-0">
        <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{title}</h3>
        {reason && <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">{reason}</p>}
      </div>
      <span className="shrink-0 text-xs tabular-nums">{state}</span>
    </div>
  )
}
