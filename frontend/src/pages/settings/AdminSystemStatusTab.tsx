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
  const [featureSearch, setFeatureSearch] = useState('')
  const [featureFilter, setFeatureFilter] = useState<'all' | 'attention' | 'disabled'>('all')
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
  const featureRows = data.feature_matrix
    ? Object.values(data.feature_matrix)
      .filter((component) => matchesFeatureSearch(component, featureSearch))
      .filter((component) => matchesFeatureFilter(component, featureFilter))
      .sort((a, b) => featureLabel(a.key, a.label).localeCompare(featureLabel(b.key, b.label), 'de'))
    : []

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
            <div className="flex items-center gap-2">
              <span className="text-neutral-600 dark:text-neutral-400">Gesamtstatus</span>
              <StatusBadge state={data.overall} />
            </div>
            <BuildTable build={build} />
          </div>
          <button type="button" onClick={handleExport} disabled={exporting} className="rounded border border-neutral-300 px-3 py-1.5 text-sm hover:bg-neutral-50 disabled:cursor-wait disabled:opacity-60 dark:border-neutral-700 dark:hover:bg-neutral-900">
            {exporting ? 'Export läuft…' : 'Diagnose exportieren'}
          </button>
        </div>
      </SettingsSection>
      <SettingsSection title="Datenebene" description="Datenbank, Speicher, Jobverarbeitung und DSGVO-Artefakte.">
        <StatusGrid items={[
          ['Datenbank', data.components.db],
          ['TimescaleDB-Kompression', data.components.timescale_compression],
          ['Speicher', data.components.storage],
          ['Jobs', data.components.jobs],
          ['DSGVO-Artefakte', data.components.gdpr_artifacts],
        ]} />
      </SettingsSection>
      <SettingsSection title="Dienste und Nachrichten" description="API-nahe Dienste, ML und Queue-Verarbeitung.">
        <StatusGrid items={[
          ['API', data.components.api],
          ['ML-Dienst', data.components.ml],
          ['NATS / JetStream', data.components.nats],
        ]} />
      </SettingsSection>
      <SettingsSection title="Routing und Geodaten" description="Routing-Engines, lokale Geodaten, Höhenmodell, Coverage und Kartendateien.">
        <StatusGrid items={[
          ['BRouter', data.components.brouter],
          ['BadRouter-Routinggraph', data.components.badrouter],
          ['OSM-Geodaten', data.components.osm],
          ['Höhendaten', data.components.dem],
          ['Coverage', data.components.coverage],
          ['Offline-Karte (PMTiles)', data.components.tiles],
        ]} />
      </SettingsSection>
      {data.feature_matrix && (
        <SettingsSection title="Feature-Matrix" description="Konfigurierte und aktuell verfügbare Funktionsbereiche.">
          <div className="mb-4 flex flex-col gap-3 rounded-lg border border-neutral-200 bg-neutral-50 p-3 dark:border-neutral-800 dark:bg-neutral-900/40 lg:flex-row lg:items-center">
            <input
              type="search"
              value={featureSearch}
              onChange={(event) => setFeatureSearch(event.target.value)}
              placeholder="Feature suchen..."
              className="min-h-10 flex-1 rounded-md border border-neutral-300 bg-white px-3 text-sm outline-none focus:border-accent-DEFAULT dark:border-neutral-700 dark:bg-neutral-950"
            />
            <select
              value={featureFilter}
              onChange={(event) => setFeatureFilter(event.target.value as 'all' | 'attention' | 'disabled')}
              className="min-h-10 rounded-md border border-neutral-300 bg-white px-3 text-sm outline-none focus:border-accent-DEFAULT dark:border-neutral-700 dark:bg-neutral-950"
            >
              <option value="all">Alle Status</option>
              <option value="attention">Aufmerksamkeit</option>
              <option value="disabled">Deaktiviert</option>
            </select>
          </div>
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {featureRows.map((component) => (
              <StatusCard key={component.key} title={featureLabel(component.key, component.label)} component={component} compact />
            ))}
          </div>
        </SettingsSection>
      )}
    </div>
  )
}

function StatusGrid({ items }: { items: Array<[string, ComponentStatus | undefined]> }) {
  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {items.map(([title, component]) => (
        <StatusCard key={title} title={title} component={component} />
      ))}
    </div>
  )
}

function StatusCard({ title, component, compact = false }: { title: string; component?: ComponentStatus; compact?: boolean }) {
  const state = component?.state ?? 'unknown'
  const reason = component?.reason_code
  const details = component?.details ? Object.entries(component.details).slice(0, compact ? 2 : 3) : []
  return (
    <div className={`rounded-lg border border-neutral-200 bg-white p-4 dark:border-neutral-800 dark:bg-neutral-950/40 ${compact ? 'min-h-[7.25rem]' : 'min-h-[8.5rem]'}`}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">{title}</h3>
          {reason && <p className="mt-1 text-xs leading-5 text-neutral-500 dark:text-neutral-400">{reasonLabel(reason)}</p>}
        </div>
        <StatusBadge state={state} />
      </div>
      {details.length > 0 && (
        <dl className="mt-4 grid gap-1 text-xs text-neutral-500 dark:text-neutral-400">
          {details.map(([key, value]) => (
            <div key={key} className="flex min-w-0 justify-between gap-3">
              <dt className="truncate">{detailLabel(key)}</dt>
              <dd className="truncate text-right font-mono text-neutral-700 dark:text-neutral-300">{detailValue(value)}</dd>
            </div>
          ))}
        </dl>
      )}
    </div>
  )
}

function StatusBadge({ state }: { state: SystemStatusState }) {
  const color = stateTone(state)
  return (
    <span className={`inline-flex shrink-0 items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium tabular-nums ${color.container}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${color.dot}`} />
      {stateLabel(state)}
    </span>
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

const FEATURE_LABELS: Record<string, string> = {
  activity_track_summary_geo: 'Geodaten in Aktivitäten',
  activity_track_summary_surface: 'Wege und Oberflächen',
  ai_agent: 'KI-Assistent',
  badrouter: 'BadRouter-Routinggraph',
  brouter: 'BRouter',
  brouter_segment_build: 'BRouter-Segmente erzeugen',
  brouter_segment_download: 'BRouter-Segmente herunterladen',
  coverage: 'Coverage',
  coverage_tiles: 'Coverage-Kacheln',
  dem_correction: 'Höhenkorrektur',
  deployment_profile: 'Installationsprofil',
  direct_upload_lifecycle: 'Direkter Upload',
  document_ml: 'Dokumentverarbeitung',
  gdpr_artifacts: 'DSGVO-Transferdateien',
  geo_poi_enrichment: 'Geo-/POI-Anreicherung',
  import_path_enabled: 'Import aus Serverpfaden',
  isochrone: 'Isochronen',
  library_paths: 'Dateipfade in der Bibliothek',
  library_schemas: 'Dateischemata',
  library_sync: 'Bibliotheks-Synchronisierung',
  local_poi_search: 'Lokale Ortssuche',
  ml_service: 'ML-Dienst',
  offline_map: 'Offline-Karte',
  osm_enrichment: 'OSM-Anreicherung',
  osm_import: 'OSM-Import',
  osm_lift_snap: 'Lift-Snap über OSM',
  photo_embedding_storage_access: 'Foto-Speicherzugriff',
  photo_embeddings: 'Foto-Indexierung',
  route_photo_media: 'Routenfotos',
  route_suggestion: 'Routenvorschläge',
  routing: 'Lokales Routing',
  routing_tiles: 'Routing-Kacheln',
  segment_enrichment_suggest: 'Segmentvorschläge',
  segment_osm_lift_import: 'Lift-Segmente aus OSM',
  segment_publish: 'Segment-Veröffentlichung',
  segment_recompute: 'Segment-Neuberechnung',
  segments: 'Segmente',
  storage_backend: 'Speicher-Backend',
  storage_mode: 'Speichermodus',
  surface_enrichment: 'Oberflächen-Anreicherung',
  system_layers_coverage: 'Coverage-Systemlayer',
  system_layers_vector: 'Vektor-Systemlayer',
}

const REASON_LABELS: Record<string, string> = {
  artifact_store_probe_failed: 'Artefakt-Speicher konnte nicht geprüft werden.',
  brouter_segments_missing: 'BRouter-Segmente fehlen.',
  brouter_unreachable: 'BRouter ist nicht erreichbar.',
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
  missing_profiles: 'Erforderliche Profile sind nicht aktiv.',
  missing_service: 'Erforderlicher Dienst läuft nicht.',
  ml_disabled: 'ML-Dienst ist deaktiviert.',
  ml_not_ready: 'ML-Dienst ist noch nicht bereit.',
  ml_roles_unavailable: 'ML-Rollen sind nicht vollständig verfügbar.',
  ml_status_unavailable: 'Interner ML-Status ist nicht erreichbar.',
  ml_unreachable: 'ML-Dienst ist nicht erreichbar.',
  nats_not_configured: 'NATS ist nicht konfiguriert.',
  nats_unavailable: 'NATS ist nicht erreichbar.',
  not_configured: 'Nicht konfiguriert.',
  osm_regions_failed: 'Mindestens eine OSM-Region ist fehlgeschlagen.',
  osm_regions_importing: 'Mindestens eine OSM-Region importiert noch.',
  osm_regions_need_redownload: 'Mindestens eine OSM-Region muss erneut geladen werden.',
  pmtiles_missing: 'Kartendateien fehlen.',
  pmtiles_dir_missing: 'PMTiles-Verzeichnis ist nicht eingebunden.',
  postgis_missing: 'PostGIS ist nicht verfügbar.',
  replica_count_unknown: 'Replikationssicherheit konnte nicht bewertet werden.',
  routing_tiles_missing: 'Routing-Kacheln fehlen.',
  s3_storage: 'Bei S3-Speicher nicht verfügbar.',
  s3_bucket_unreachable: 'S3-Bucket ist nicht erreichbar.',
  s3_list_prefix_probe_failed: 'S3-Listenprüfung ist fehlgeschlagen.',
  s3_multipart_abort_probe_failed: 'S3-Multipart-Prüfung ist fehlgeschlagen.',
  s3_write_delete_probe_failed: 'S3-Schreib-/Löschprüfung ist fehlgeschlagen.',
  stuck_jobs: 'Jobs hängen fest.',
  storage_unavailable: 'Speicher ist nicht verfügbar.',
  timescale_compression_disabled: 'TimescaleDB-Kompression ist deaktiviert.',
  timescale_compression_option_drift: 'TimescaleDB-Kompression weicht von der Soll-Konfiguration ab.',
  timescale_compression_policy_missing: 'TimescaleDB-Kompressionsrichtlinie fehlt.',
  timescale_compression_setting_drift: 'TimescaleDB-Kompression und Einstellung widersprechen sich.',
  timescale_compression_toggle_failed: 'Letzte Kompressionsumschaltung ist fehlgeschlagen.',
}

function featureLabel(key: string, backendLabel?: string): string {
  if (FEATURE_LABELS[key]) return FEATURE_LABELS[key]
  if (backendLabel && backendLabel !== key && !backendLabel.includes('_')) return backendLabel
  return key
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function matchesFeatureSearch(component: ComponentStatus, query: string): boolean {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return true
  return [
    featureLabel(component.key, component.label),
    reasonLabel(component.reason_code ?? ''),
  ].some((value) => value.toLowerCase().includes(normalized))
}

function matchesFeatureFilter(component: ComponentStatus, filter: 'all' | 'attention' | 'disabled'): boolean {
  if (filter === 'all') return true
  if (filter === 'disabled') return component.state === 'disabled' || component.state === 'not_configured'
  return component.state !== 'ok' && component.state !== 'disabled' && component.state !== 'not_configured'
}

function stateTone(state: SystemStatusState): { container: string; dot: string } {
  switch (state) {
    case 'ok':
      return {
        container: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
        dot: 'bg-emerald-500',
      }
    case 'degraded':
    case 'missing_artifacts':
      return {
        container: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
        dot: 'bg-amber-500',
      }
    case 'unavailable':
    case 'blocked':
      return {
        container: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
        dot: 'bg-red-500',
      }
    case 'disabled':
    case 'not_configured':
      return {
        container: 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400',
        dot: 'bg-neutral-400',
      }
    default:
      return {
        container: 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-400',
        dot: 'bg-sky-500',
      }
  }
}

function stateLabel(state: SystemStatusState): string {
  return STATE_LABELS[state] ?? state
}

function reasonLabel(reason: string): string {
  return REASON_LABELS[reason] ?? reason
}

const DETAIL_LABELS: Record<string, string> = {
  active_exports_count: 'Aktive Exporte',
  active_imports_count: 'Aktive Importe',
  artifact_counter_error_count: 'Artefakt-Zählfehler',
  available: 'Verfügbar',
  backend: 'Backend',
  compressed_chunks_count: 'Komprimierte Chunks',
  configured: 'Konfiguriert',
  corrupt_cell_count: 'Beschädigte Zellen',
  directory_present: 'Verzeichnis eingebunden',
  disabled_consumer_count: 'Deaktivierte Consumer',
  extension_fuzzystrmatch: 'Erweiterung fuzzystrmatch',
  extension_pg_trgm: 'Erweiterung pg_trgm',
  extension_pgcrypto: 'Erweiterung pgcrypto',
  failed_last_24h_count: 'Fehler letzte 24 h',
  failed_region_count: 'Fehlgeschlagene Regionen',
  imported_region_count: 'Importierte Regionen',
  importing_region_count: 'Laufende Importe',
  layer_count: 'Layer',
  llm_broker_status: 'LLM-Broker',
  metrics_available: 'Metriken verfügbar',
  missing_role_count: 'Fehlende Rollen',
  mode: 'Modus',
  oldest_pending_age_seconds: 'Ältester Rückstand (s)',
  pending_message_count: 'Offene Nachrichten',
  pmtiles_file_count: 'PMTiles-Dateien',
  profile_count: 'Profile',
  redelivered_count: 'Erneute Zustellungen',
  region_count: 'Regionen',
  replica_count: 'Replikas',
  running_count: 'Offene Jobs',
  segment_count: 'Segmente',
  state: 'Status',
  status_accessible: 'Status erreichbar',
  stale_region_count: 'Neu zu ladende Regionen',
  tile_count: 'Tiles',
  uncompressed_chunks_count: 'Unkomprimierte Chunks',
  version: 'Version',
}

function detailLabel(key: string): string {
  if (DETAIL_LABELS[key]) return DETAIL_LABELS[key]
  return key
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function detailValue(value: string): string {
  if (value === 'true') return 'Ja'
  if (value === 'false') return 'Nein'
  return value
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
