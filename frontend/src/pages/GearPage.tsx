import { useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Bike, CircleGauge, Footprints, Mountain, Package, Radio, Shirt, Smartphone, Snowflake, Waves } from 'lucide-react'
import { fetchGear, createGear, fetchGearDocuments, downloadGearDocumentBlob } from '@/api/gear'
import { fetchDevices } from '@/api/hardware'
import type { Gear, GearStatus } from '@/api/types'
import { sportLabel } from '@/lib/sportTaxonomy'
import { getActiveSchema } from '@/lib/gearSchema'
import { gearMatchesCategoryScope, gearTypeLabel } from '@/lib/gearSchemaHelpers'
import {
  gearCategoryLabel,
  isGearCategoryScope,
  type GearCategoryScope,
} from '@/lib/gearSchemaTypes'
import { supportedLanguageFrom } from '@/i18n/localizedString'
import type { SupportedLanguage } from '@/i18n/languages'
import GearForm from '@/components/GearForm'
import MaintenanceAlerts from '@/components/MaintenanceAlerts'
import GearMergePanel from '@/components/GearMergePanel'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

function parseGearScope(raw: string | null): GearCategoryScope | null {
  return isGearCategoryScope(raw) ? raw : null
}

export default function GearPage() {
  const queryClient = useQueryClient()
  const [searchParams] = useSearchParams()
  const { t, i18n } = useTranslation()
  const language = supportedLanguageFrom(i18n.language)
  const statusChips: { value: GearStatus | undefined; label: string }[] = [
    { value: undefined, label: t('gearPages.status.all') },
    { value: 'active', label: t('gearPages.status.active') },
    { value: 'stored', label: t('gearPages.status.stored') },
    { value: 'retired', label: t('gearPages.status.retired') },
  ]
  const typeFilter = searchParams.get('type')
  const [statusFilter, setStatusFilter] = useState<GearStatus | undefined>('active')
  const [showForm, setShowForm] = useState(false)
  const [showMerge, setShowMerge] = useState(false)

  const schema = getActiveSchema()

  const { data: gear, isLoading: gearLoading } = useQuery({
    queryKey: ['gear', statusFilter],
    queryFn: () => fetchGear(statusFilter),
  })

  const { data: recorders } = useQuery({
    queryKey: ['devices', 'recorder'],
    queryFn: () => fetchDevices('recorder'),
    enabled: !typeFilter,
  })

  const { data: sensors } = useQuery({
    queryKey: ['devices', 'sensor'],
    queryFn: () => fetchDevices('sensor'),
    enabled: !typeFilter,
  })

  const createMutation = useMutation({
    mutationFn: (data: Partial<Gear>) => createGear(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gear'] })
      setShowForm(false)
    },
  })

  // Filtered view — when ?type= is set in the URL
  if (typeFilter) {
    const categoryScope = parseGearScope(typeFilter)
    const filtered = gear?.filter(g => {
      if (categoryScope) return gearMatchesCategoryScope(g, categoryScope, schema)
      return true
    }) ?? []

    const titleLabel = categoryScope
      ? gearCategoryLabel(categoryScope, language)
      : typeFilter

    if (gearLoading) {
      return <div className="flex items-center justify-center h-64 text-neutral-400">{t('gearPages.common.loadingGear')}</div>
    }

    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Link
              to="/gear"
              className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 transition-colors"
            >
              {t('gearPages.common.backToGear')}
            </Link>
            <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
              {titleLabel}
            </h1>
          </div>
          <div className="flex items-center gap-3">
            <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700">
              {statusChips.map(({ value, label }) => (
                <button
                  key={label}
                  onClick={() => setStatusFilter(value)}
                  className={`px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer ${
                    statusFilter === value
                      ? 'bg-accent-DEFAULT text-white'
                      : 'text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
            <button
              onClick={() => setShowMerge(s => !s)}
              className="text-sm px-3 py-2 rounded-lg border border-neutral-200 dark:border-neutral-800 text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 transition-colors cursor-pointer"
            >
              {t('gearPages.actions.mergeDuplicates')}
            </button>
            <button
              onClick={() => setShowForm(true)}
              className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors cursor-pointer"
            >
              {t('gearPages.actions.addGear')}
            </button>
          </div>
        </div>

        <MaintenanceAlerts />

        {showMerge && (
          <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-base font-medium text-neutral-900 dark:text-neutral-100">{t('gearPages.gearPage.duplicateTitle')}</h2>
            </div>
            <GearMergePanel onClose={() => setShowMerge(false)} />
          </div>
        )}

        {showForm && (
          <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
            <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">{t('gearPages.gearPage.newGear')}</h2>
            <GearForm
              categoryScope={categoryScope ?? 'other'}
              onSave={(data) => createMutation.mutate(data)}
              onCancel={() => setShowForm(false)}
              isSaving={createMutation.isPending}
            />
            {createMutation.isError && (
              <p className="text-sm text-rose-500 mt-2">
                {t('gearPages.errors.createFailed', { message: createMutation.error instanceof Error ? createMutation.error.message : t('gearPages.common.unknown') })}
              </p>
            )}
          </div>
        )}

        {filtered.length > 0 ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {filtered.map((item) => (
              <GearCard key={item.id} gear={item} language={language} />
            ))}
          </div>
        ) : (
          <div className="text-center py-16 text-neutral-400">
            {t('gearPages.gearPage.emptyCategory')}
          </div>
        )}
      </div>
    )
  }

  // Category tile overview — 8 gear scopes + hardware
  const scopeCount = (scope: GearCategoryScope) =>
    gear?.filter(g => gearMatchesCategoryScope(g, scope, schema)).length ?? 0

  const recorderCount = recorders?.length ?? 0
  const sensorCount = sensors?.length ?? 0

  const categories = [
    {
      to: '/gear?type=bike',
      title: gearCategoryLabel('bike', language),
      description: t('gearPages.gearPage.categories.bike'),
      icon: Bike,
      count: scopeCount('bike'),
    },
    {
      to: '/gear?type=shoes',
      title: gearCategoryLabel('shoes', language),
      description: t('gearPages.gearPage.categories.shoes'),
      icon: Footprints,
      count: scopeCount('shoes'),
    },
    {
      to: '/gear?type=winter',
      title: gearCategoryLabel('winter', language),
      description: t('gearPages.gearPage.categories.winter'),
      icon: Snowflake,
      count: scopeCount('winter'),
    },
    {
      to: '/gear?type=water',
      title: gearCategoryLabel('water', language),
      description: t('gearPages.gearPage.categories.water'),
      icon: Waves,
      count: scopeCount('water'),
    },
    {
      to: '/gear?type=diving',
      title: gearCategoryLabel('diving', language),
      description: t('gearPages.gearPage.categories.diving'),
      icon: CircleGauge,
      count: scopeCount('diving'),
    },
    {
      // climb scope — items added in Phase 9; tile visible now, count = 0 until then
      to: '/gear?type=climb',
      title: gearCategoryLabel('climb', language),
      description: t('gearPages.gearPage.categories.climb'),
      icon: Mountain,
      count: scopeCount('climb'),
    },
    {
      // apparel scope — items added in Phase 9; tile visible now, count = 0 until then
      to: '/gear?type=apparel',
      title: gearCategoryLabel('apparel', language),
      description: t('gearPages.gearPage.categories.apparel'),
      icon: Shirt,
      count: scopeCount('apparel'),
    },
    {
      to: '/gear?type=other',
      title: gearCategoryLabel('other', language),
      description: t('gearPages.gearPage.categories.other'),
      icon: Package,
      count: scopeCount('other'),
    },
    {
      to: '/hardware/recorder',
      title: t('gearPages.gearPage.recorders'),
      description: t('gearPages.gearPage.categories.recorders'),
      icon: Smartphone,
      count: recorderCount,
    },
    {
      to: '/hardware/sensor',
      title: t('gearPages.gearPage.sensors'),
      description: t('gearPages.gearPage.categories.sensors'),
      icon: Radio,
      count: sensorCount,
    },
  ]

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">{t('gearPages.gearPage.title')}</h1>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          {t('gearPages.gearPage.subtitle')}
        </p>
      </div>

      <MaintenanceAlerts />

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {categories.map(({ to, title, description, icon: Icon, count }) => (
          <Link
            key={title}
            to={to}
            className="group rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-3
                       hover:border-accent-400 dark:hover:border-accent-600 hover:shadow-sm transition-all"
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="flex items-center justify-center w-9 h-9 rounded-lg bg-accent-100 dark:bg-accent-100/20 text-accent-DEFAULT">
                  <Icon className="w-5 h-5" />
                </div>
                <h2 className="text-base font-medium text-neutral-900 dark:text-neutral-100 group-hover:text-accent-DEFAULT transition-colors">
                  {title}
                </h2>
              </div>
              {count > 0 && (
                <span className="text-sm font-medium text-neutral-500 dark:text-neutral-400 tabular-nums">
                  {count}
                </span>
              )}
            </div>
            <p className="text-sm text-neutral-500 dark:text-neutral-400">{description}</p>
          </Link>
        ))}
      </div>
    </div>
  )
}

function GearCardImage({ gearId }: { gearId: string }) {
  const { t } = useTranslation()
  const [blobUrl, setBlobUrl] = useState<string | null>(null)
  const [loadFailed, setLoadFailed] = useState(false)
  const { data: docs } = useQuery({
    queryKey: ['gear-documents', gearId],
    queryFn: () => fetchGearDocuments(gearId),
  })
  const firstPhoto = docs?.find(d => d.content_type.startsWith('image/'))

  useEffect(() => {
    setBlobUrl(null)
    setLoadFailed(false)
    if (!firstPhoto) return

    let url: string | null = null
    let cancelled = false
    downloadGearDocumentBlob(firstPhoto.id)
      .then((nextUrl) => {
        if (cancelled) {
          URL.revokeObjectURL(nextUrl)
          return
        }
        url = nextUrl
        setBlobUrl(nextUrl)
      })
      .catch(() => {
        if (!cancelled) setLoadFailed(true)
      })

    return () => {
      cancelled = true
      if (url) URL.revokeObjectURL(url)
    }
  }, [firstPhoto?.id])

  if (!firstPhoto) {
    return (
      <div className="w-full h-full flex items-center justify-center bg-neutral-100 dark:bg-neutral-800 text-neutral-400">
        <svg className="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M6.827 6.175A2.31 2.31 0 015.186 7.23c-.38.054-.757.112-1.134.175C2.999 7.58 2.25 8.507 2.25 9.574V18a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9.574c0-1.067-.75-1.994-1.802-2.169a47.865 47.865 0 00-1.134-.175 2.31 2.31 0 01-1.64-1.055l-.822-1.316a2.192 2.192 0 00-1.736-1.039 48.774 48.774 0 00-5.232 0 2.192 2.192 0 00-1.736 1.039l-.821 1.316z" />
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M16.5 12.75a4.5 4.5 0 11-9 0 4.5 4.5 0 019 0z" />
        </svg>
      </div>
    )
  }

  if (!blobUrl || loadFailed) {
    return (
      <div className="w-full h-full flex items-center justify-center bg-neutral-100 dark:bg-neutral-800 text-neutral-400">
        {loadFailed ? (
          <span className="text-xs">{t('gearPages.gearPage.imageLoadFailed')}</span>
        ) : (
          <svg className="animate-spin h-5 w-5 text-neutral-300" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
        )}
      </div>
    )
  }

  return (
    <img
      src={blobUrl}
      alt=""
      className="w-full h-full object-cover"
      loading="lazy"
    />
  )
}

function GearCard({ gear, language }: { gear: Gear; language?: SupportedLanguage }) {
  const { t } = useTranslation()
  const { formatDistanceCompact, formatElevationGain } = useUnitFormatters()
  const schema = getActiveSchema()
  const elevGain = formatElevationGain(gear.total_elevation_gain_m)

  // Use schema path label when available; fall back to hardcoded label.
  const typeDisplay = (() => {
    return gearTypeLabel(schema, gear.type, language)
  })()

  return (
    <Link
      to={`/gear/${gear.id}`}
      className="block rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden hover:border-orange-300 dark:hover:border-orange-700 transition-colors"
    >
      <div className="h-32 w-full">
        <GearCardImage gearId={gear.id} />
      </div>

      <div className="p-4">
        <div className="flex items-start justify-between mb-2">
          <div className="min-w-0">
            <h3 className="text-base font-medium text-neutral-900 dark:text-neutral-100 truncate">
              {gear.name}
            </h3>
            <p className="text-sm text-neutral-500 dark:text-neutral-400">
              {typeDisplay}
              {gear.brand && ` - ${gear.brand}`}
              {gear.model && ` ${gear.model}`}
            </p>
          </div>
          <StatusBadge status={gear.status} />
        </div>

        <div className="grid grid-cols-4 gap-2 mt-3">
          <div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400">{t('gearPages.metrics.distance')}</div>
            <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 tabular-nums">
              {formatDistanceCompact(gear.total_distance_m)}
            </div>
          </div>
          <div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400">{t('gearPages.metrics.elevationShort')}</div>
            <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 tabular-nums">
              {elevGain ?? '—'}
            </div>
          </div>
          <div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400">{t('gearPages.metrics.time')}</div>
            <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 tabular-nums">
              {formatGearCardDuration(gear.total_time_s)}
            </div>
          </div>
          <div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400">{t('gearPages.metrics.activities')}</div>
            <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 tabular-nums">
              {gear.total_activities}
            </div>
          </div>
        </div>

        <SportBadges predicates={gear.applicable_sport_predicates} language={language} />
      </div>
    </Link>
  )
}

// Schema covers all legacy gear type keys (bike, shoes, goggles, ski, paddle, other) — no fallback needed.
function SportBadges({ predicates, language }: { predicates: import('@/api/types').GearDefaultSportPredicate[]; language?: SupportedLanguage }) {
  if (!predicates || predicates.length === 0) return null

  // Deduplicate by sport_type; preserve insertion order.
  const seen = new Set<string>()
  const sports: string[] = []
  for (const p of predicates) {
    if (p.sport_type === 'generic') continue
    if (!seen.has(p.sport_type)) {
      seen.add(p.sport_type)
      sports.push(p.sport_type)
    }
  }

  const MAX_VISIBLE = 3
  const visible = sports.slice(0, MAX_VISIBLE)
  const overflow = sports.length - MAX_VISIBLE

  return (
    <div className="flex flex-wrap gap-1 mt-3">
      {visible.map((s) => (
        <span
          key={s}
          className="px-1.5 py-0.5 rounded text-xs bg-orange-50 text-accent-600 dark:bg-orange-900/20 dark:text-orange-400"
        >
          {sportLabel(s, undefined, { language })}
        </span>
      ))}
      {overflow > 0 && (
        <span className="px-1.5 py-0.5 rounded text-xs bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400">
          +{overflow}
        </span>
      )}
    </div>
  )
}

function formatGearCardDuration(seconds: number | null | undefined): string {
  if (seconds == null || isNaN(seconds)) return '—'
  if (seconds >= 3600) return `${Math.round(seconds / 3600)} h`
  return `${Math.floor(seconds / 60)} min`
}

function StatusBadge({ status }: { status: GearStatus }) {
  const { t } = useTranslation()
  if (status === 'active') return null
  const styles: Record<string, string> = {
    stored: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
    retired: 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400',
  }
  const labels: Record<string, string> = {
    stored: t('gearPages.status.stored'),
    retired: t('gearPages.status.retired'),
  }
  return (
    <span className={`flex-shrink-0 px-2 py-0.5 rounded text-xs font-medium ${styles[status] ?? 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400'}`}>
      {labels[status] ?? t('gearPages.status.unknown')}
    </span>
  )
}
