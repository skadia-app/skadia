import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import type { BuildInfo, ComponentStatus, SystemStatusState } from '@/api/systemStatus'
import { downloadSystemStatusExport, fetchSystemStatus } from '@/api/systemStatus'
import { SettingsSection } from '@/components/ui/SettingsSection'
import { frontendBuildInfo } from '@/lib/buildInfo'
import { useToast } from '@/lib/toast'
import { triggerBlobDownload } from '@/lib/utils'

export default function AdminSystemStatusTab() {
  const { addToast } = useToast()
  const [exporting, setExporting] = useState(false)
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

  async function handleExport() {
    if (exporting) return
    setExporting(true)
    try {
      const { blob, filename } = await downloadSystemStatusExport()
      triggerBlobDownload(blob, filename || 'skadia-system-status.json')
      addToast({ type: 'success', title: 'Diagnose exportiert' })
    } catch {
      addToast({
        type: 'error',
        title: 'Diagnose-Export fehlgeschlagen',
        message: 'Der Systemstatus konnte nicht exportiert werden.',
        duration: 0,
      })
    } finally {
      setExporting(false)
    }
  }

  return (
    <div className="space-y-6">
      <SettingsSection title="Systemstatus" description="Laufzeitstatus, Build-Herkunft und Diagnosehinweise.">
        <div className="flex flex-col gap-3 text-sm sm:flex-row sm:items-start sm:justify-between">
          <div className="space-y-2">
            <div>Gesamtstatus: {stateLabel(data.overall)}</div>
            <BuildTable build={build} />
          </div>
          <button type="button" onClick={handleExport} disabled={exporting} className="rounded border border-neutral-300 px-3 py-1.5 text-sm hover:bg-neutral-50 disabled:cursor-wait disabled:opacity-60 dark:border-neutral-700 dark:hover:bg-neutral-900">
            {exporting ? 'Export läuft…' : 'Diagnose exportieren'}
          </button>
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
        {reason && <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">{reasonLabel(reason)}</p>}
      </div>
      <span className="shrink-0 text-xs tabular-nums">{stateLabel(state)}</span>
    </div>
  )
}

function BuildTable({ build }: { build: Record<string, BuildInfo> }) {
  const rows = ['api', 'frontend', 'ml']
    .map((key) => build[key])
    .filter((row): row is BuildInfo => Boolean(row))
  if (rows.length === 0) return null
  return (
    <div className="overflow-x-auto text-xs text-neutral-600 dark:text-neutral-400">
      <table className="min-w-full border-separate border-spacing-x-4 border-spacing-y-1">
        <thead>
          <tr className="text-left">
            <th>Dienst</th>
            <th>Commit</th>
            <th>Commit-Zeit</th>
            <th>Build-Zeit</th>
            <th>Konfiguration</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.service}>
              <td>{serviceLabel(row.service)}</td>
              <td className="font-mono">{shortValue(row.git_commit)}</td>
              <td>{fallbackValue(row.commit_time)}</td>
              <td>{fallbackValue(row.build_time)}</td>
              <td>{fallbackValue(row.config_version)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

const STATE_LABELS: Record<SystemStatusState, string> = {
  ok: 'OK',
  degraded: 'Eingeschränkt',
  unavailable: 'Nicht erreichbar',
  disabled: 'Deaktiviert',
  not_configured: 'Nicht konfiguriert',
  missing_artifacts: 'Daten fehlen',
  blocked: 'Blockiert',
  unknown: 'Unbekannt',
}

const REASON_LABELS: Record<string, string> = {
  artifact_store_probe_failed: 'Artefakt-Speicher konnte nicht geprüft werden.',
  brouter_segments_missing: 'BRouter-Segmente fehlen.',
  consumer_lag: 'Queue-Verarbeitung hängt zurück.',
  core_profile: 'Im Core-Profil deaktiviert.',
  coverage_corrupt_cells: 'Coverage-Daten enthalten beschädigte Zellen.',
  coverage_missing: 'Coverage-Daten fehlen.',
  db_extension_missing: 'Eine erforderliche Datenbankerweiterung fehlt.',
  db_not_configured: 'Datenbank ist nicht konfiguriert.',
  db_unreachable: 'Datenbank ist nicht erreichbar.',
  dem_tiles_missing: 'Höhendaten fehlen.',
  expired_artifact_backlog: 'Abgelaufene DSGVO-Artefakte warten auf Bereinigung.',
  file_presence_only: 'Nur Dateipräsenz geprüft.',
  incomplete_multipart_backlog: 'Unvollständige Multipart-Uploads vorhanden.',
  jetstream_consumers_missing: 'JetStream-Consumer fehlen.',
  jetstream_introspection_unavailable: 'JetStream-Details konnten nicht gelesen werden.',
  jetstream_stream_missing: 'JetStream-Stream fehlt.',
  local_storage_not_replica_safe: 'Lokaler Speicher ist für mehrere API-Replikas nicht freigegeben.',
  migration_dirty: 'Datenbankmigration ist als dirty markiert.',
  migration_status_unreadable: 'Migrationsstatus konnte nicht gelesen werden.',
  missing_artifacts: 'Erforderliche lokale Daten fehlen.',
  ml_disabled: 'ML-Dienst ist deaktiviert.',
  ml_not_ready: 'ML-Dienst ist noch nicht bereit.',
  ml_roles_unavailable: 'ML-Rollen sind nicht vollständig verfügbar.',
  ml_status_unavailable: 'Interner ML-Status ist nicht erreichbar.',
  ml_unreachable: 'ML-Dienst ist nicht erreichbar.',
  nats_not_configured: 'NATS ist nicht konfiguriert.',
  nats_unavailable: 'NATS ist nicht erreichbar.',
  not_configured: 'Nicht konfiguriert.',
  pmtiles_missing: 'Kartendateien fehlen.',
  postgis_missing: 'PostGIS ist nicht verfügbar.',
  replica_count_unknown: 'Replikationssicherheit konnte nicht bewertet werden.',
  routing_tiles_missing: 'Routing-Kacheln fehlen.',
  s3_bucket_unreachable: 'S3-Bucket ist nicht erreichbar.',
  s3_list_prefix_probe_failed: 'S3-Listenprüfung ist fehlgeschlagen.',
  s3_multipart_abort_probe_failed: 'S3-Multipart-Prüfung ist fehlgeschlagen.',
  s3_write_delete_probe_failed: 'S3-Schreib-/Löschprüfung ist fehlgeschlagen.',
  stuck_jobs: 'Jobs hängen fest.',
  timescale_compression_disabled: 'TimescaleDB-Kompression ist deaktiviert.',
  timescale_compression_option_drift: 'TimescaleDB-Kompression weicht von der Soll-Konfiguration ab.',
  timescale_compression_policy_missing: 'TimescaleDB-Kompressionsrichtlinie fehlt.',
  timescale_compression_setting_drift: 'TimescaleDB-Kompression und Einstellung widersprechen sich.',
  timescale_compression_toggle_failed: 'Letzte Kompressionsumschaltung ist fehlgeschlagen.',
}

function stateLabel(state: SystemStatusState): string {
  return STATE_LABELS[state] ?? state
}

function reasonLabel(reason: string): string {
  return REASON_LABELS[reason] ?? reason
}

function serviceLabel(service: string): string {
  if (service === 'api') return 'API'
  if (service === 'frontend') return 'Frontend'
  if (service === 'ml') return 'ML'
  return service
}

function shortValue(value?: string): string {
  const raw = fallbackValue(value)
  if (raw === 'unbekannt') return raw
  return raw.length > 12 ? raw.slice(0, 12) : raw
}

function fallbackValue(value?: string): string {
  if (!value || value === 'unknown') return 'unbekannt'
  return value
}
