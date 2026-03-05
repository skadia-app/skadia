import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { getUserProfile, updateUserProfile } from '@/api/settings'
import type { UserProfile } from '@/api/types'

export default function PhysiologicalProfilePage() {
  const queryClient = useQueryClient()

  const { data: profile } = useQuery({
    queryKey: ['user-profile'],
    queryFn: getUserProfile,
  })

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

  return (
    <div className="space-y-6">
      <div>
        <Link to="/workshop" className="text-sm text-accent-DEFAULT hover:text-accent-600 transition-colors">
          ← Werkstatt
        </Link>
      </div>

      <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Physiologisches Profil</h1>

      <div className="rounded-md bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 px-4 py-3 text-sm text-blue-700 dark:text-blue-300">
        Werte aus deinen Garmin-Dateien werden automatisch vorgeschlagen (nur wenn das Feld noch leer ist).
      </div>

      <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-6">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Körperdaten &amp; Leistungswerte</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Für genaue Leistungszonen, TRIMP und Intensitäts-Auswertung
          </p>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {/* Geburtsdatum */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Geburtsdatum</label>
            <input
              type="date"
              value={profileForm.birth_date ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, birth_date: e.target.value || undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
            />
            {profileForm.birth_date && (
              <p className="text-xs text-neutral-500">
                Alter: {calcAge(profileForm.birth_date)} Jahre · Tanaka-Formel: {tanakaFormula(profileForm.birth_date)} bpm
              </p>
            )}
          </div>

          {/* Geschlecht */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Geschlecht</label>
            <select
              value={profileForm.gender ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, gender: e.target.value as any }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
            >
              <option value="">Keine Angabe</option>
              <option value="M">Männlich</option>
              <option value="F">Weiblich</option>
              <option value="O">Divers</option>
            </select>
          </div>

          {/* Gewicht */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Gewicht (kg)</label>
            <input
              type="number"
              min="20" max="300" step="0.1"
              value={profileForm.weight_kg ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, weight_kg: e.target.value ? parseFloat(e.target.value) : undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
              placeholder="z.B. 72.5"
            />
          </div>

          {/* Größe */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Größe (cm)</label>
            <input
              type="number"
              min="100" max="250"
              value={profileForm.height_cm ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, height_cm: e.target.value ? parseInt(e.target.value) : undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
              placeholder="z.B. 178"
            />
          </div>

          {/* FTP */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              FTP (Watt)
              <span className="ml-1 text-xs text-neutral-400 font-normal">Functional Threshold Power (~1h Max-Dauerleistung)</span>
            </label>
            <input
              type="number" min="0" max="1000"
              value={profileForm.ftp_w ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, ftp_w: e.target.value ? parseInt(e.target.value) : undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
              placeholder="z.B. 280"
            />
          </div>

          {/* LTHR */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              LTHR (bpm)
              <span className="ml-1 text-xs text-neutral-400 font-normal">Laktatschwellen-Herzfrequenz</span>
            </label>
            <input
              type="number" min="0" max="250"
              value={profileForm.lthr_bpm ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, lthr_bpm: e.target.value ? parseInt(e.target.value) : undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
              placeholder="z.B. 165"
            />
          </div>

          {/* HR-Max */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">HR-Max Override (bpm)</label>
            <input
              type="number" min="0" max="250"
              value={profileForm.max_hr_bpm ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, max_hr_bpm: e.target.value ? parseInt(e.target.value) : undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
              placeholder="Optional"
            />
            <div className="text-xs text-neutral-400 space-x-3">
              {profile?.max_hr_formula && <span>Formel: {profile.max_hr_formula} bpm</span>}
              {profile?.max_hr_detected && <span>Erkannt: {profile.max_hr_detected} bpm</span>}
              <span className="font-medium text-neutral-600 dark:text-neutral-300">Effektiv: {profile?.effective_max_hr ?? 220} bpm</span>
            </div>
          </div>

          {/* Ruhepuls */}
          <div className="space-y-1">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Ruhepuls (bpm)</label>
            <input
              type="number" min="0" max="120"
              value={profileForm.resting_hr_bpm ?? ''}
              onChange={e => setProfileForm(f => ({ ...f, resting_hr_bpm: e.target.value ? parseInt(e.target.value) : undefined }))}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm"
              placeholder="z.B. 52"
            />
          </div>
        </div>

        <div className="flex justify-end">
          <button
            onClick={() => profileMutation.mutate(profileForm)}
            disabled={profileMutation.isPending}
            className="px-4 py-2 text-sm font-medium rounded-lg bg-accent-DEFAULT hover:bg-accent-600 text-white disabled:opacity-50 transition-colors"
          >
            {profileMutation.isPending ? 'Speichern...' : 'Profil speichern'}
          </button>
        </div>
      </div>
    </div>
  )
}

function calcAge(birthDate: string | null | undefined): number | null {
  if (!birthDate) return null
  const birth = new Date(birthDate)
  const now = new Date()
  return now.getFullYear() - birth.getFullYear()
}

function tanakaFormula(birthDate: string | null | undefined): number | null {
  const age = calcAge(birthDate)
  if (age === null) return null
  return Math.round(208 - 0.7 * age)
}
