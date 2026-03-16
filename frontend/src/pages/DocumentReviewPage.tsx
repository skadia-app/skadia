import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchPendingReviewGear,
  confirmGear,
  rejectGear,
  updateAndConfirmGear,
  type PendingReviewGear,
} from '@/api/documents'

const GEAR_TYPES = [
  { value: 'bike', label: 'Fahrrad' },
  { value: 'shoes', label: 'Laufschuhe' },
  { value: 'goggles', label: 'Schwimmbrille' },
  { value: 'ski', label: 'Ski' },
  { value: 'paddle', label: 'Paddel' },
  { value: 'other', label: 'Sonstiges' },
]

export default function DocumentReviewPage() {
  const qc = useQueryClient()
  const { data: items = [], isLoading } = useQuery({
    queryKey: ['pending-review-gear'],
    queryFn: fetchPendingReviewGear,
    refetchInterval: 10_000,
    refetchIntervalInBackground: false,
  })

  const confirmMutation = useMutation({
    mutationFn: confirmGear,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['pending-review-gear'] }),
  })

  const rejectMutation = useMutation({
    mutationFn: rejectGear,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['pending-review-gear'] }),
  })

  const updateConfirmMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Record<string, unknown> }) =>
      updateAndConfirmGear(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['pending-review-gear'] }),
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade...</div>
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          Dokument-Überprüfung
        </h1>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Aus Rechnungen automatisch erkannte Ausrüstung überprüfen und bestätigen.
        </p>
      </div>

      {items.length === 0 ? (
        <div className="text-center py-16 text-neutral-400">
          <p className="text-lg">Keine ausstehenden Überprüfungen</p>
          <p className="text-sm mt-1">Hochgeladene Rechnungen werden hier nach der Verarbeitung angezeigt.</p>
        </div>
      ) : (
        <div className="space-y-3">
          {items.map(item => (
            <PendingGearCard
              key={item.id}
              item={item}
              onConfirm={() => confirmMutation.mutate(item.id)}
              onReject={() => rejectMutation.mutate(item.id)}
              onUpdateConfirm={(data) => updateConfirmMutation.mutate({ id: item.id, data })}
              isConfirming={confirmMutation.isPending || updateConfirmMutation.isPending}
              isRejecting={rejectMutation.isPending}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function PendingGearCard({ item, onConfirm, onReject, onUpdateConfirm, isConfirming, isRejecting }: {
  item: PendingReviewGear
  onConfirm: () => void
  onReject: () => void
  onUpdateConfirm: (data: Record<string, unknown>) => void
  isConfirming: boolean
  isRejecting: boolean
}) {
  const [editing, setEditing] = useState(false)
  const [editData, setEditData] = useState({
    name: item.name,
    type: item.type,
    brand: item.brand ?? '',
    model: item.model ?? '',
  })

  function handleSaveAndConfirm() {
    onUpdateConfirm({
      name: editData.name,
      type: editData.type,
      brand: editData.brand || null,
      model: editData.model || null,
    })
  }

  return (
    <div className="border border-amber-200 dark:border-amber-800/50 rounded-xl p-4 bg-amber-50/50 dark:bg-amber-900/10">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 space-y-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium
                             bg-amber-100 text-amber-800 dark:bg-amber-800/30 dark:text-amber-300">
              KI-Vorschlag
            </span>
            {!editing && (
              <span className="text-sm text-neutral-500 dark:text-neutral-400">{item.type}</span>
            )}
          </div>

          {editing ? (
            <div className="grid grid-cols-2 gap-2 mt-2">
              <div className="col-span-2">
                <label className="text-xs text-neutral-500 dark:text-neutral-400 mb-0.5 block">Name</label>
                <input
                  type="text"
                  value={editData.name}
                  onChange={e => setEditData(d => ({ ...d, name: e.target.value }))}
                  className="w-full px-2 py-1 text-sm rounded border border-neutral-300 dark:border-neutral-600
                             bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100
                             focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                />
              </div>
              <div>
                <label className="text-xs text-neutral-500 dark:text-neutral-400 mb-0.5 block">Typ</label>
                <select
                  value={editData.type}
                  onChange={e => setEditData(d => ({ ...d, type: e.target.value }))}
                  className="w-full px-2 py-1 text-sm rounded border border-neutral-300 dark:border-neutral-600
                             bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100
                             focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                >
                  {GEAR_TYPES.map(t => (
                    <option key={t.value} value={t.value}>{t.label}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-xs text-neutral-500 dark:text-neutral-400 mb-0.5 block">Marke</label>
                <input
                  type="text"
                  value={editData.brand}
                  onChange={e => setEditData(d => ({ ...d, brand: e.target.value }))}
                  className="w-full px-2 py-1 text-sm rounded border border-neutral-300 dark:border-neutral-600
                             bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100
                             focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                />
              </div>
              <div className="col-span-2">
                <label className="text-xs text-neutral-500 dark:text-neutral-400 mb-0.5 block">Modell</label>
                <input
                  type="text"
                  value={editData.model}
                  onChange={e => setEditData(d => ({ ...d, model: e.target.value }))}
                  className="w-full px-2 py-1 text-sm rounded border border-neutral-300 dark:border-neutral-600
                             bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100
                             focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                />
              </div>
            </div>
          ) : (
            <>
              <p className="font-medium text-neutral-900 dark:text-neutral-100">{item.name}</p>
              {item.brand && (
                <p className="text-sm text-neutral-500 dark:text-neutral-400">{item.brand}</p>
              )}
              {(item.model || item.purchase_price != null || item.weight_g != null || item.notes) && (
                <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm mt-2">
                  {item.model && (
                    <>
                      <span className="text-neutral-500 dark:text-neutral-400">Modell</span>
                      <span className="text-neutral-800 dark:text-neutral-200">{item.model}</span>
                    </>
                  )}
                  {item.purchase_price != null && (
                    <>
                      <span className="text-neutral-500 dark:text-neutral-400">Preis</span>
                      <span className="text-neutral-800 dark:text-neutral-200">
                        {item.purchase_price} {item.currency || 'EUR'}
                      </span>
                    </>
                  )}
                  {item.weight_g != null && (
                    <>
                      <span className="text-neutral-500 dark:text-neutral-400">Gewicht</span>
                      <span className="text-neutral-800 dark:text-neutral-200">{item.weight_g} g</span>
                    </>
                  )}
                  {item.notes && (
                    <>
                      <span className="text-neutral-500 dark:text-neutral-400">Notizen</span>
                      <span className="text-neutral-800 dark:text-neutral-200 truncate">{item.notes}</span>
                    </>
                  )}
                </div>
              )}
            </>
          )}

          {item.source_document_id && (
            <a
              href={`/api/v1/gear/${item.id}/documents/${item.source_document_id}/download`}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-block text-xs mt-1"
              style={{ color: 'var(--color-accent-600)' }}
            >
              Quelldokument anzeigen
            </a>
          )}
        </div>

        <div className="flex flex-col gap-2 flex-shrink-0 items-end">
          <div className="flex gap-2">
            <button
              onClick={onReject}
              disabled={isRejecting}
              className="px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                         text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800
                         disabled:opacity-50 transition-colors"
            >
              Ablehnen
            </button>
            {editing ? (
              <>
                <button
                  onClick={() => setEditing(false)}
                  className="px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                             text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800
                             transition-colors"
                >
                  Abbrechen
                </button>
                <button
                  onClick={handleSaveAndConfirm}
                  disabled={isConfirming}
                  className="px-3 py-1.5 text-sm rounded-lg text-white
                             hover:opacity-90 disabled:opacity-50 transition-colors"
                  style={{ backgroundColor: 'var(--color-accent-DEFAULT)' }}
                >
                  Speichern & Bestätigen
                </button>
              </>
            ) : (
              <>
                <button
                  onClick={() => setEditing(true)}
                  className="px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                             text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800
                             transition-colors"
                >
                  Bearbeiten
                </button>
                <button
                  onClick={onConfirm}
                  disabled={isConfirming}
                  className="px-3 py-1.5 text-sm rounded-lg text-white
                             hover:opacity-90 disabled:opacity-50 transition-colors"
                  style={{ backgroundColor: 'var(--color-accent-DEFAULT)' }}
                >
                  Bestätigen
                </button>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
