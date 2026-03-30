import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { createManualActivity } from '@/api/manualActivity'
import { fetchGear } from '@/api/gear'
import { fetchTags } from '@/api/tags'
import { CATEGORIES, SPORT_DEFS, sportsInCategory } from '@/lib/sportTaxonomy'
import { useToast } from '@/lib/toast'
import Toggle from '@/components/Toggle'
import type { Gear } from '@/api/types'
import type { Tag } from '@/api/tags'

// ── Helpers ──────────────────────────────────────────────────────────────────

function parseDuration(h: string, m: string, s: string): number {
  return (parseInt(h) || 0) * 3600 + (parseInt(m) || 0) * 60 + (parseInt(s) || 0)
}

const RPE_COLORS: Record<number, string> = {
  1:  '#22c55e',
  2:  '#4ade80',
  3:  '#86efac',
  4:  '#fde047',
  5:  '#facc15',
  6:  '#fb923c',
  7:  '#f97316',
  8:  '#ef4444',
  9:  '#dc2626',
  10: '#991b1b',
}

const FEEL_OPTIONS = [
  { value: 0, emoji: '😫', label: 'Erschöpft' },
  { value: 1, emoji: '😕', label: 'Schlecht' },
  { value: 2, emoji: '😐', label: 'Neutral' },
  { value: 3, emoji: '🙂', label: 'Gut' },
  { value: 4, emoji: '😄', label: 'Super' },
]

// ── Component ─────────────────────────────────────────────────────────────────

export default function ManualActivityPage() {
  const navigate = useNavigate()
  const { addToast } = useToast()

  // ── Form state ──────────────────────────────────────────────────────────────
  const [sportType, setSportType] = useState('running')
  const [subType, setSubType] = useState('')
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')

  // Datum/Uhrzeit: default = heute, aktuelle Stunde
  const [date, setDate] = useState(() => new Date().toISOString().slice(0, 10))
  const [time, setTime] = useState(() => {
    const now = new Date()
    return `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`
  })

  // Dauer in h / min / s
  const [durH, setDurH] = useState('0')
  const [durM, setDurM] = useState('30')
  const [durS, setDurS] = useState('0')

  const [distanceKm, setDistanceKm] = useState('')
  const [elevGain, setElevGain] = useState('')
  const [elevLoss, setElevLoss] = useState('')

  // Rechte Spalte
  const [selectedTags, setSelectedTags] = useState<string[]>([])
  const [gearId, setGearId] = useState('')
  const [isElectric, setIsElectric] = useState(false)
  const [isVirtual, setIsVirtual] = useState(false)
  const [rpe, setRpe] = useState<number | null>(null)
  const [feel, setFeel] = useState<number | null>(null)

  const [submitting, setSubmitting] = useState(false)

  // ── Data queries ────────────────────────────────────────────────────────────
  const { data: gearList = [] } = useQuery<Gear[]>({
    queryKey: ['gear', 'active'],
    queryFn: () => fetchGear('active'),
  })

  const { data: tagList = [] } = useQuery<Tag[]>({
    queryKey: ['tags'],
    queryFn: fetchTags,
  })

  // ── Derived ─────────────────────────────────────────────────────────────────
  const selectedSportDef = SPORT_DEFS.find(d => d.sportType === sportType)
  const isBike = selectedSportDef?.categoryKey === 'bike'

  function handleSportChange(newSport: string) {
    setSportType(newSport)
    setSubType('')
    if (SPORT_DEFS.find(d => d.sportType === newSport)?.categoryKey !== 'bike') {
      setIsElectric(false)
    }
  }

  function toggleTag(tagId: string) {
    setSelectedTags(prev =>
      prev.includes(tagId) ? prev.filter(id => id !== tagId) : [...prev, tagId],
    )
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()

    const elapsedTimeS = parseDuration(durH, durM, durS)
    if (elapsedTimeS <= 0) {
      addToast({ type: 'error', title: 'Dauer fehlt', message: 'Bitte gib eine Dauer ein.' })
      return
    }

    // Combine date + time → RFC3339 in local timezone
    const startedAt = new Date(`${date}T${time}:00`).toISOString()

    setSubmitting(true)
    try {
      const result = await createManualActivity({
        sport_type: sportType,
        sub_type: subType || undefined,
        title: title || undefined,
        description: description || undefined,
        started_at: startedAt,
        elapsed_time_s: elapsedTimeS,
        distance_m: distanceKm ? parseFloat(distanceKm) * 1000 : undefined,
        elevation_gain_m: elevGain ? parseFloat(elevGain) : undefined,
        elevation_loss_m: elevLoss ? parseFloat(elevLoss) : undefined,
        tags: selectedTags.length ? selectedTags : undefined,
        gear_id: gearId || undefined,
        is_electric: isElectric || undefined,
        is_virtual: isVirtual || undefined,
        workout_rpe: rpe ?? undefined,
        workout_feel: feel ?? undefined,
      })
      addToast({ type: 'success', title: 'Aktivität erstellt' })
      navigate(`/activities/${result.id}`)
    } catch {
      addToast({ type: 'error', title: 'Fehler', message: 'Aktivität konnte nicht erstellt werden.' })
    } finally {
      setSubmitting(false)
    }
  }

  // ── Render ──────────────────────────────────────────────────────────────────

  return (
    <div className="min-h-screen bg-white text-neutral-900 dark:bg-neutral-950 dark:text-neutral-100">
      {/* Header */}
      <div className="border-b border-neutral-200 bg-neutral-50 px-4 py-4 dark:border-neutral-800 dark:bg-neutral-900">
        <div className="mx-auto max-w-5xl flex items-center gap-4">
          <Link
            to="/activities"
            className="flex items-center gap-1.5 text-sm text-neutral-500 transition-colors hover:text-neutral-700 dark:text-neutral-400 dark:hover:text-neutral-200"
          >
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 19l-7-7 7-7" />
            </svg>
            Zurück
          </Link>
          <h1 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Aktivität erstellen</h1>
        </div>
      </div>

      {/* Form */}
      <form onSubmit={handleSubmit} className="mx-auto max-w-5xl px-4 py-6">
        <div className="grid grid-cols-1 gap-6 md:grid-cols-2">

          {/* ── Linke Spalte ─────────────────────────────────────────────────── */}
          <div className="flex flex-col gap-5">

            {/* Sportart — Kategorie-Auswahl */}
            <fieldset>
              <legend className="mb-2 text-sm font-medium text-neutral-700 dark:text-neutral-300">Sportart</legend>
              <div className="overflow-hidden rounded-lg border border-neutral-200 bg-neutral-50 dark:border-neutral-700 dark:bg-neutral-900">
                {CATEGORIES.map(cat => {
                  const sports = sportsInCategory(cat.key)
                  if (!sports.length) return null
                  return (
                    <details key={cat.key} open={sports.some(s => s.sportType === sportType)}>
                      <summary
                        className="flex cursor-pointer select-none items-center gap-2 px-3 py-2.5 text-sm font-medium text-neutral-700 transition-colors hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800"
                        style={{ borderLeft: `3px solid ${cat.color}` }}
                      >
                        <span className="flex-1">{cat.label}</span>
                        <svg className="h-4 w-4 text-neutral-400 transition-transform group-open:rotate-180 dark:text-neutral-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
                        </svg>
                      </summary>
                      <div className="flex flex-wrap gap-1.5 px-3 py-2 bg-neutral-100/80 dark:bg-neutral-950/50">
                        {sports.map(s => (
                          <button
                            key={s.sportType}
                            type="button"
                            onClick={() => handleSportChange(s.sportType)}
                            className={[
                              'cursor-pointer rounded-full px-3 py-1 text-xs font-medium transition-colors',
                              sportType === s.sportType
                                ? 'bg-accent-DEFAULT text-white'
                                : 'bg-neutral-200 text-neutral-700 hover:bg-neutral-300 dark:bg-neutral-800 dark:text-neutral-300 dark:hover:bg-neutral-700',
                            ].join(' ')}
                          >
                            {s.label}
                          </button>
                        ))}
                      </div>
                    </details>
                  )
                })}
              </div>
            </fieldset>

            {/* SubType — nur wenn SubTypes vorhanden */}
            {selectedSportDef && selectedSportDef.subTypes.length > 0 && (
              <div>
                <label className="mb-1.5 block text-sm font-medium text-neutral-600 dark:text-neutral-300">
                  Unterart
                </label>
                <select
                  value={subType}
                  onChange={e => setSubType(e.target.value)}
                  className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 focus:border-accent-DEFAULT focus:outline-none dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100"
                >
                  <option value="">— keine —</option>
                  {selectedSportDef.subTypes.map(st => (
                    <option key={st.subType} value={st.subType}>{st.label}</option>
                  ))}
                </select>
              </div>
            )}

            {/* Datum + Uhrzeit */}
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="mb-1.5 block text-sm font-medium text-neutral-600 dark:text-neutral-300">Datum</label>
                <input
                  type="date"
                  value={date}
                  onChange={e => setDate(e.target.value)}
                  required
                  className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 focus:border-accent-DEFAULT focus:outline-none dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100"
                />
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium text-neutral-600 dark:text-neutral-300">Uhrzeit</label>
                <input
                  type="time"
                  value={time}
                  onChange={e => setTime(e.target.value)}
                  required
                  className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 focus:border-accent-DEFAULT focus:outline-none dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100"
                />
              </div>
            </div>

            {/* Dauer */}
            <div>
              <label className="mb-1.5 block text-sm font-medium text-neutral-600 dark:text-neutral-300">Dauer</label>
              <div className="flex items-center gap-2">
                <div className="flex-1">
                  <input
                    type="number"
                    min="0"
                    value={durH}
                    onChange={e => setDurH(e.target.value)}
                    placeholder="0"
                    className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 focus:border-accent-DEFAULT focus:outline-none dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100"
                  />
                  <p className="mt-0.5 text-center text-xs text-neutral-500">h</p>
                </div>
                <span className="text-neutral-500 text-lg pb-4">:</span>
                <div className="flex-1">
                  <input
                    type="number"
                    min="0"
                    max="59"
                    value={durM}
                    onChange={e => setDurM(e.target.value)}
                    placeholder="0"
                    className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 focus:border-accent-DEFAULT focus:outline-none dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100"
                  />
                  <p className="mt-0.5 text-center text-xs text-neutral-500">min</p>
                </div>
                <span className="text-neutral-500 text-lg pb-4">:</span>
                <div className="flex-1">
                  <input
                    type="number"
                    min="0"
                    max="59"
                    value={durS}
                    onChange={e => setDurS(e.target.value)}
                    placeholder="0"
                    className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 focus:border-accent-DEFAULT focus:outline-none dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100"
                  />
                  <p className="mt-0.5 text-center text-xs text-neutral-500">sek</p>
                </div>
              </div>
            </div>

            {/* Distanz */}
            <div>
              <label className="mb-1.5 block text-sm font-medium text-neutral-600 dark:text-neutral-300">
                Distanz <span className="text-neutral-500 font-normal">(optional)</span>
              </label>
              <div className="relative">
                <input
                  type="number"
                  min="0"
                  step="0.01"
                  value={distanceKm}
                  onChange={e => setDistanceKm(e.target.value)}
                  placeholder="0.0"
                  className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 pr-12 text-sm text-neutral-900 focus:border-accent-DEFAULT focus:outline-none dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100"
                />
                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-neutral-500">km</span>
              </div>
            </div>

            {/* Höhenmeter */}
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="mb-1.5 block text-sm font-medium text-neutral-600 dark:text-neutral-300">
                  Anstieg <span className="text-neutral-500 font-normal">(optional)</span>
                </label>
                <div className="relative">
                  <input
                    type="number"
                    min="0"
                    value={elevGain}
                    onChange={e => setElevGain(e.target.value)}
                    placeholder="0"
                    className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 pr-8 text-sm text-neutral-900 focus:border-accent-DEFAULT focus:outline-none dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100"
                  />
                  <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-500">m</span>
                </div>
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium text-neutral-600 dark:text-neutral-300">
                  Abstieg <span className="text-neutral-500 font-normal">(optional)</span>
                </label>
                <div className="relative">
                  <input
                    type="number"
                    min="0"
                    value={elevLoss}
                    onChange={e => setElevLoss(e.target.value)}
                    placeholder="0"
                    className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 pr-8 text-sm text-neutral-900 focus:border-accent-DEFAULT focus:outline-none dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100"
                  />
                  <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-neutral-500">m</span>
                </div>
              </div>
            </div>

            {/* Titel */}
            <div>
              <label className="mb-1.5 block text-sm font-medium text-neutral-600 dark:text-neutral-300">
                Titel <span className="text-neutral-500 font-normal">(optional)</span>
              </label>
              <input
                type="text"
                value={title}
                onChange={e => setTitle(e.target.value)}
                placeholder="Aktivitätstitel"
                maxLength={200}
                className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 placeholder-neutral-400 focus:border-accent-DEFAULT focus:outline-none dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100 dark:placeholder-neutral-600"
              />
            </div>

            {/* Beschreibung */}
            <div>
              <label className="mb-1.5 block text-sm font-medium text-neutral-600 dark:text-neutral-300">
                Beschreibung <span className="text-neutral-500 font-normal">(optional)</span>
              </label>
              <textarea
                value={description}
                onChange={e => setDescription(e.target.value)}
                placeholder="Notizen zur Aktivität..."
                rows={4}
                className="w-full resize-none rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 placeholder-neutral-400 focus:border-accent-DEFAULT focus:outline-none dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100 dark:placeholder-neutral-600"
              />
            </div>
          </div>

          {/* ── Rechte Spalte ────────────────────────────────────────────────── */}
          <div className="flex flex-col gap-5">

            {/* Tags */}
            <div>
              <label className="mb-2 block text-sm font-medium text-neutral-600 dark:text-neutral-300">
                Tags <span className="text-neutral-500 font-normal">(optional)</span>
              </label>
              {tagList.length === 0 ? (
                <p className="text-sm text-neutral-500">Keine Tags vorhanden.</p>
              ) : (
                <div className="flex flex-wrap gap-1.5">
                  {tagList.map(tag => {
                    const selected = selectedTags.includes(tag.id)
                    return (
                      <button
                        key={tag.id}
                        type="button"
                        onClick={() => toggleTag(tag.id)}
                        className={[
                          'cursor-pointer rounded-full px-3 py-1 text-xs font-medium transition-all',
                          selected
                            ? 'ring-2 ring-white/30 ring-offset-1 ring-offset-white dark:ring-offset-neutral-950'
                            : 'opacity-70 hover:opacity-100',
                        ].join(' ')}
                        style={{
                          backgroundColor: tag.color,
                          color: tag.text_color,
                        }}
                      >
                        {tag.name}
                      </button>
                    )
                  })}
                </div>
              )}
            </div>

            {/* Ausrüstung */}
            <div>
              <label className="mb-1.5 block text-sm font-medium text-neutral-600 dark:text-neutral-300">
                Ausrüstung <span className="text-neutral-500 font-normal">(optional)</span>
              </label>
              <select
                value={gearId}
                onChange={e => setGearId(e.target.value)}
                className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 focus:border-accent-DEFAULT focus:outline-none dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100"
              >
                <option value="">— keine —</option>
                {gearList.map(gear => (
                  <option key={gear.id} value={gear.id}>{gear.name}</option>
                ))}
              </select>
            </div>

            {/* E-Bike Toggle — nur bei bike-Kategorie */}
            {isBike && (
              <div className="flex items-center justify-between rounded-lg border border-neutral-200 bg-neutral-50 px-4 py-3 dark:border-neutral-700 dark:bg-neutral-900">
                <div>
                  <p className="text-sm font-medium text-neutral-800 dark:text-neutral-200">E-Bike</p>
                  <p className="text-xs text-neutral-500">Elektrisch unterstützt</p>
                </div>
                <Toggle
                  checked={isElectric}
                  onChange={(v) => setIsElectric(v)}
                  aria-label="E-Bike"
                />
              </div>
            )}

            {/* Virtuell Toggle */}
            <div className="flex items-center justify-between rounded-lg border border-neutral-200 bg-neutral-50 px-4 py-3 dark:border-neutral-700 dark:bg-neutral-900">
              <div>
                <p className="text-sm font-medium text-neutral-800 dark:text-neutral-200">Virtuell</p>
                <p className="text-xs text-neutral-500">Simulierte GPS-Strecke</p>
              </div>
              <Toggle
                checked={isVirtual}
                onChange={(v) => setIsVirtual(v)}
                aria-label="Virtuell"
              />
            </div>

            {/* RPE Slider */}
            <div>
              <div className="mb-2 flex items-center justify-between">
                <label className="text-sm font-medium text-neutral-600 dark:text-neutral-300">
                  Belastung (RPE) <span className="text-neutral-500 font-normal">(optional)</span>
                </label>
                {rpe !== null && (
                  <span
                    className="text-sm font-bold tabular-nums"
                    style={{ color: RPE_COLORS[rpe] }}
                  >
                    {rpe} / 10
                  </span>
                )}
              </div>
              <div className="rounded-lg border border-neutral-200 bg-neutral-50 px-4 py-3 dark:border-neutral-700 dark:bg-neutral-900">
                <input
                  type="range"
                  min={1}
                  max={10}
                  step={1}
                  value={rpe ?? 5}
                  onChange={e => setRpe(parseInt(e.target.value))}
                  className="w-full cursor-pointer"
                  style={{ accentColor: rpe !== null ? RPE_COLORS[rpe] : 'var(--color-accent-DEFAULT, #f97316)' }}
                />
                <div className="mt-1.5 flex justify-between text-xs text-neutral-600">
                  <span>1</span>
                  <span>5</span>
                  <span>10</span>
                </div>
                {rpe === null && (
                  <p
                    className="mt-1 cursor-pointer text-center text-xs text-neutral-500 hover:text-neutral-700 dark:text-neutral-600 dark:hover:text-neutral-400"
                    onClick={() => setRpe(5)}
                  >
                    Klicken zum Festlegen
                  </p>
                )}
                {rpe !== null && (
                  <div className="mt-2 flex justify-end">
                    <button
                      type="button"
                      onClick={() => setRpe(null)}
                      className="cursor-pointer text-xs text-neutral-500 transition-colors hover:text-neutral-700 dark:text-neutral-600 dark:hover:text-neutral-400"
                    >
                      Zurücksetzen
                    </button>
                  </div>
                )}
              </div>
            </div>

            {/* Feel Emoji-Buttons */}
            <div>
              <label className="mb-2 block text-sm font-medium text-neutral-600 dark:text-neutral-300">
                Wie war es? <span className="text-neutral-500 font-normal">(optional)</span>
              </label>
              <div className="flex gap-2 rounded-lg border border-neutral-200 bg-neutral-50 px-3 py-3 dark:border-neutral-700 dark:bg-neutral-900">
                {FEEL_OPTIONS.map(opt => (
                  <button
                    key={opt.value}
                    type="button"
                    title={opt.label}
                    onClick={() => setFeel(feel === opt.value ? null : opt.value)}
                    className={[
                      'cursor-pointer flex flex-1 flex-col items-center gap-1 rounded-lg py-2 text-xl transition-all',
                      feel === opt.value
                        ? 'bg-accent-DEFAULT/20 ring-1 ring-accent-DEFAULT'
                        : 'opacity-60 hover:bg-neutral-200 hover:opacity-100 dark:hover:bg-neutral-800',
                    ].join(' ')}
                  >
                    <span>{opt.emoji}</span>
                    <span className="text-xs text-neutral-500 hidden sm:block">{opt.label}</span>
                  </button>
                ))}
              </div>
            </div>

            {/* Spacer + Submit */}
            <div className="flex-1" />
            <button
              type="submit"
              disabled={submitting}
              className="w-full rounded-lg bg-accent-DEFAULT px-4 py-3 text-sm font-semibold text-white transition-colors hover:bg-accent-600 cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
            >
              {submitting ? 'Wird gespeichert...' : 'Aktivität erstellen'}
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}
