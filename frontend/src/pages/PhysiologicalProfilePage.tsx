import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { getUserProfile, updateUserProfile } from '@/api/settings'
import type { UserProfile } from '@/api/types'
import { fetchTrainingZones } from '@/api/trainingZones'
import ZoneEditorCard from '@/components/zones/ZoneEditor'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { weightFromDisplayValue, weightInputUnit, weightToDisplayValue } from '@/lib/units'

export default function PhysiologicalProfilePage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { options: unitOptions } = useUnitFormatters()

  const { data: profile } = useQuery({
    queryKey: ['user-profile'],
    queryFn: getUserProfile,
  })

  const { data: zonesData, isLoading: zonesLoading } = useQuery({
    queryKey: ['training-zones'],
    queryFn: fetchTrainingZones,
  })

  const [activeSportTab, setActiveSportTab] = useState<number | null>(null)

  // Set first sport as default tab once data arrives
  useEffect(() => {
    if (zonesData?.sports?.length && activeSportTab === null) {
      setActiveSportTab(zonesData.sports[0].sport)
    }
  }, [zonesData, activeSportTab])

  const [profileForm, setProfileForm] = useState<Partial<UserProfile>>({})

  useEffect(() => {
    if (profile) {
      setProfileForm({
        birth_date: profile.birth_date ?? undefined,
        gender: profile.gender,
        height_cm: profile.height_cm ?? undefined,
        weight_kg: profile.weight_kg ?? undefined,
        ftp_w: profile.ftp_w ?? undefined,
        lthr_bpm: profile.lthr_bpm ?? undefined,
        max_hr_bpm: profile.max_hr_bpm ?? undefined,
        resting_hr_bpm: profile.resting_hr_bpm ?? undefined,
      })
    }
  }, [profile])

  const profileMutation = useMutation({
    mutationFn: updateUserProfile,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user-profile'] })
    },
  })

  const weightUnit = weightInputUnit(unitOptions.units)
  const displayedWeight = profileForm.weight_kg == null
    ? ''
    : weightToDisplayValue(profileForm.weight_kg, unitOptions.units).toFixed(1)
  const displayWeightMin = weightToDisplayValue(20, unitOptions.units).toFixed(1)
  const displayWeightMax = weightToDisplayValue(300, unitOptions.units).toFixed(1)
  const weightPlaceholder = unitOptions.units === 'imperial' ? '154.0' : '70.0'

  return (
    <div className="space-y-6">
      <div>
        <Link to="/workshop" className="text-sm text-accent-DEFAULT hover:text-accent-600 transition-colors">
          {t('physiologicalProfile.backToWorkshop')}
        </Link>
      </div>

      <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">{t('physiologicalProfile.title')}</h1>

      <div className="rounded-md bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 px-4 py-3 text-sm text-blue-700 dark:text-blue-300">
        {t('physiologicalProfile.garminHint')}
      </div>

      <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-6">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">{t('physiologicalProfile.bodyPerformanceTitle')}</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            {t('physiologicalProfile.bodyPerformanceDescription')}
          </p>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {/* Geburtsdatum */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('physiologicalProfile.birthDate')}</label>
            <input
              type="date"
              value={profileForm.birth_date ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, birth_date: e.target.value || undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
            />
            {profileForm.birth_date && (
              <p className="text-xs text-neutral-500">
                {t('physiologicalProfile.ageFormula', { age: calcAge(profileForm.birth_date), bpm: tanakaFormula(profileForm.birth_date) })}
              </p>
            )}
          </div>

          {/* Geschlecht */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('physiologicalProfile.gender')}</label>
            <select
              value={profileForm.gender ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, gender: e.target.value as any }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
            >
              <option value="">{t('physiologicalProfile.genderOptions.unspecified')}</option>
              <option value="M">{t('physiologicalProfile.genderOptions.male')}</option>
              <option value="F">{t('physiologicalProfile.genderOptions.female')}</option>
              <option value="O">{t('physiologicalProfile.genderOptions.diverse')}</option>
            </select>
          </div>

          {/* Weight */}
          <div className="space-y-1">
            <label htmlFor="physiological-weight" className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('physiologicalProfile.weightKg', { unit: weightUnit })}</label>
            <input
              id="physiological-weight"
              type="number"
              min={displayWeightMin} max={displayWeightMax} step="0.1"
              value={displayedWeight}
              onChange={e => setProfileForm(f => ({ ...f, weight_kg: e.target.value ? weightFromDisplayValue(parseFloat(e.target.value), unitOptions.units) : undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
              placeholder={t('physiologicalProfile.weightPlaceholder', { value: weightPlaceholder })}
            />
          </div>

          {/* Height */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('physiologicalProfile.heightCm')}</label>
            <input
              type="number"
              min="100" max="250"
              value={profileForm.height_cm ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, height_cm: e.target.value ? parseInt(e.target.value) : undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
              placeholder={t('physiologicalProfile.heightPlaceholder')}
            />
          </div>

          {/* FTP */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              FTP (Watt)
              <span className="ml-1 text-xs text-neutral-400 font-normal">{t('physiologicalProfile.ftpDescription')}</span>
            </label>
            <input
              type="number" min="0" max="1000"
              value={profileForm.ftp_w ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, ftp_w: e.target.value ? parseInt(e.target.value) : undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
              placeholder={t('physiologicalProfile.ftpPlaceholder')}
            />
          </div>

          {/* LTHR */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              LTHR (bpm)
              <span className="ml-1 text-xs text-neutral-400 font-normal">{t('physiologicalProfile.lthrDescription')}</span>
            </label>
            <input
              type="number" min="0" max="250"
              value={profileForm.lthr_bpm ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, lthr_bpm: e.target.value ? parseInt(e.target.value) : undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
              placeholder={t('physiologicalProfile.lthrPlaceholder')}
            />
          </div>

          {/* HR-Max */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('physiologicalProfile.hrMaxOverride')}</label>
            <input
              type="number" min="0" max="250"
              value={profileForm.max_hr_bpm ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, max_hr_bpm: e.target.value ? parseInt(e.target.value) : undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
              placeholder={t('physiologicalProfile.optional')}
            />
            <div className="text-xs text-neutral-400 space-x-3">
              {profile?.max_hr_formula && <span>{t('physiologicalProfile.formulaBpm', { value: profile.max_hr_formula })}</span>}
              {profile?.max_hr_detected && <span>{t('physiologicalProfile.detectedBpm', { value: profile.max_hr_detected })}</span>}
              <span className="font-medium text-neutral-600 dark:text-neutral-300">{t('physiologicalProfile.effectiveBpm', { value: profile?.effective_max_hr ?? 220 })}</span>
            </div>
          </div>

          {/* Ruhepuls */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('physiologicalProfile.restingHr')}</label>
            <input
              type="number" min="0" max="120"
              value={profileForm.resting_hr_bpm ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, resting_hr_bpm: e.target.value ? parseInt(e.target.value) : undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
              placeholder={t('physiologicalProfile.restingHrPlaceholder')}
            />
          </div>
        </div>

        <div className="flex justify-end">
          <button
            onClick={() => profileMutation.mutate(profileForm)}
            disabled={profileMutation.isPending}
            className="cursor-pointer px-4 py-2 text-sm font-medium rounded-lg bg-accent-DEFAULT hover:bg-accent-600 text-white disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {profileMutation.isPending ? t('physiologicalProfile.saving') : t('physiologicalProfile.saveProfile')}
          </button>
        </div>
      </div>

      {/* Training Zones */}
      <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-5">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">{t('physiologicalProfile.trainingZonesTitle')}</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            {t('physiologicalProfile.trainingZonesDescription')}
          </p>
        </div>

        {zonesLoading && (
          <div className="text-sm text-neutral-500 dark:text-neutral-400 py-2">{t('physiologicalProfile.loadingZones')}</div>
        )}

        {!zonesLoading && (!zonesData?.sports || zonesData.sports.length === 0) && (
          <div className="text-sm text-neutral-500 dark:text-neutral-400 py-2">
            {t('physiologicalProfile.noZones')}
          </div>
        )}

        {!zonesLoading && zonesData && zonesData.sports.length > 0 && (
          <>
            {/* Sport tabs */}
            <div className="flex flex-wrap gap-1 border-b border-neutral-200 dark:border-neutral-700 pb-0">
              {zonesData.sports.map(s => (
                <button
                  key={s.sport}
                  type="button"
                  onClick={() => setActiveSportTab(s.sport)}
                  className={[
                    'px-3 py-2 text-sm font-medium rounded-t-lg border-b-2 -mb-px transition-colors',
                    activeSportTab === s.sport
                      ? 'border-accent-DEFAULT text-accent-DEFAULT dark:text-accent-400'
                      : 'border-transparent text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200',
                  ].join(' ')}
                >
                  {s.sport_label}
                </button>
              ))}
            </div>

            {/* Active sport zone editor */}
            {zonesData.sports
              .filter(s => s.sport === activeSportTab)
              .map(s => (
                <ZoneEditorCard key={s.sport} sportZones={s} />
              ))}
          </>
        )}
      </div>
    </div>
  )
}

function calcAge(birthDate: string | null | undefined): number | null {
  if (!birthDate) return null
  const birth = new Date(birthDate)
  const now = new Date()
  let age = now.getFullYear() - birth.getFullYear()
  const monthDiff = now.getMonth() - birth.getMonth()
  if (monthDiff < 0 || (monthDiff === 0 && now.getDate() < birth.getDate())) age--
  return age
}

function tanakaFormula(birthDate: string | null | undefined): number | null {
  const age = calcAge(birthDate)
  if (age === null) return null
  return Math.round(208 - 0.7 * age)
}
