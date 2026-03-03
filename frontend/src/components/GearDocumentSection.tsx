import { useState, useRef, useCallback, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchGearDocuments, uploadGearDocument, deleteGearDocument,
  downloadGearDocumentBlob,
} from '@/api/gear'
import type { GearDocument } from '@/api/types'

interface Props {
  gearId: string
  componentId?: string
}

const DOC_TYPE_LABELS: Record<string, string> = {
  receipt: 'Rechnung',
  invoice: 'Rechnung',
  warranty: 'Garantie',
  manual: 'Handbuch',
  photo: 'Foto',
  other: 'Sonstiges',
}

const DOC_TYPE_ICONS: Record<string, string> = {
  receipt: '\u{1F4C4}',
  invoice: '\u{1F4B0}',
  warranty: '\u{1F6E1}\uFE0F',
  manual: '\u{1F4D6}',
  photo: '\u{1F5BC}\uFE0F',
  other: '\u{1F4CE}',
}

const STATUS_STYLES: Record<string, string> = {
  pending: 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400',
  processed: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
  failed: 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-400',
}

const STATUS_LABELS: Record<string, string> = {
  pending: 'Ausstehend',
  processed: 'Verarbeitet',
  failed: 'Fehlgeschlagen',
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatDate(dateStr: string): string {
  const d = new Date(dateStr)
  return d.toLocaleDateString('de-DE', { day: '2-digit', month: '2-digit', year: 'numeric' })
}

function formatCurrency(amount: number, currency: string): string {
  return new Intl.NumberFormat('de-DE', { style: 'currency', currency: currency || 'EUR' }).format(amount)
}

export default function GearDocumentSection({ gearId }: Props) {
  const queryClient = useQueryClient()
  const [collapsed, setCollapsed] = useState<boolean | null>(null)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [previewDoc, setPreviewDoc] = useState<GearDocument | null>(null)
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)

  const { data: documents = [], isLoading } = useQuery({
    queryKey: ['gear-documents-all', gearId],
    queryFn: () => fetchGearDocuments(gearId),
    enabled: !!gearId,
  })

  // Filter out pure images (they are shown in GearImagesSection)
  const docs = documents.filter(d => !d.content_type.startsWith('image/') || d.doc_type !== 'photo')

  const deleteMutation = useMutation({
    mutationFn: (docId: string) => deleteGearDocument(docId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gear-documents-all', gearId] })
      queryClient.invalidateQueries({ queryKey: ['gear-documents', gearId] })
      queryClient.invalidateQueries({ queryKey: ['gear-costs', gearId] })
      setDeleteConfirmId(null)
      if (previewDoc && deleteConfirmId === previewDoc.id) setPreviewDoc(null)
    },
  })

  // Cost summary
  const totalCost = docs.reduce((sum, d) => sum + (d.document_amount ?? 0), 0)
  const costCurrency = docs.find(d => d.document_currency)?.document_currency ?? 'EUR'

  const isCollapsed = collapsed ?? docs.length === 0

  if (isLoading) return null

  return (
    <>
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        <button
          onClick={() => setCollapsed(!isCollapsed)}
          className="w-full flex items-center justify-between px-4 py-3 text-sm font-medium text-neutral-900 dark:text-neutral-100 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors cursor-pointer"
        >
          <span>
            Dokumente{' '}
            {docs.length > 0 && <span className="text-neutral-400">({docs.length})</span>}
          </span>
          <svg
            className={`w-4 h-4 text-neutral-400 transition-transform ${isCollapsed ? '-rotate-90' : ''}`}
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
          </svg>
        </button>

        {!isCollapsed && (
          <div className="border-t border-neutral-100 dark:border-neutral-800">
            {/* Cost summary */}
            {totalCost > 0 && (
              <div className="mx-4 mt-3 px-3 py-2 rounded-lg bg-neutral-50 dark:bg-neutral-800/50 flex items-center gap-2">
                <svg className="w-4 h-4 text-neutral-400" viewBox="0 0 20 20" fill="currentColor">
                  <path d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.736 6.979C9.208 6.193 9.696 6 10 6c.304 0 .792.193 1.264.979a1 1 0 001.715-1.029C12.279 4.784 11.232 4 10 4s-2.279.784-2.979 1.95c-.285.475-.507 1-.67 1.55H6a1 1 0 000 2h.013a9.358 9.358 0 000 1H6a1 1 0 100 2h.351c.163.55.385 1.075.67 1.55C7.721 15.216 8.768 16 10 16s2.279-.784 2.979-1.95a1 1 0 10-1.715-1.029c-.472.786-.96.979-1.264.979-.304 0-.792-.193-1.264-.979a5.389 5.389 0 01-.421-.821H10a1 1 0 100-2H8.014a7.36 7.36 0 010-1H10a1 1 0 100-2H8.315c.134-.292.29-.566.421-.821z" />
                </svg>
                <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
                  Dokumentierte Kosten: {formatCurrency(totalCost, costCurrency)}
                </span>
              </div>
            )}

            {/* Document list */}
            {docs.length > 0 ? (
              <div className="divide-y divide-neutral-100 dark:divide-neutral-800">
                {docs.map(doc => (
                  <button
                    key={doc.id}
                    onClick={() => setPreviewDoc(doc)}
                    className="w-full flex items-center gap-3 px-4 py-3 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors cursor-pointer text-left"
                  >
                    <span className="text-lg flex-shrink-0">{DOC_TYPE_ICONS[doc.doc_type] ?? DOC_TYPE_ICONS.other}</span>
                    <div className="flex-1 min-w-0">
                      <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
                        {doc.title || doc.original_filename}
                      </div>
                      <div className="flex items-center gap-2 mt-0.5 text-xs text-neutral-500 dark:text-neutral-400">
                        <span>{DOC_TYPE_LABELS[doc.doc_type] ?? doc.doc_type}</span>
                        <span>·</span>
                        <span>{formatFileSize(doc.file_size_bytes)}</span>
                        {doc.document_date && (
                          <>
                            <span>·</span>
                            <span>{formatDate(doc.document_date)}</span>
                          </>
                        )}
                        {doc.document_amount != null && doc.document_amount > 0 && (
                          <>
                            <span>·</span>
                            <span className="font-medium text-neutral-700 dark:text-neutral-300">
                              {formatCurrency(doc.document_amount, doc.document_currency ?? 'EUR')}
                            </span>
                          </>
                        )}
                      </div>
                    </div>
                    {doc.processing_status && doc.processing_status !== 'processed' && (
                      <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium flex-shrink-0 ${STATUS_STYLES[doc.processing_status] ?? STATUS_STYLES.pending}`}>
                        {STATUS_LABELS[doc.processing_status] ?? doc.processing_status}
                      </span>
                    )}
                  </button>
                ))}
              </div>
            ) : (
              <div className="px-4 py-6 text-center text-sm text-neutral-400">
                Noch keine Dokumente vorhanden.
              </div>
            )}

            {/* Add button */}
            <div className="px-4 py-3 border-t border-neutral-100 dark:border-neutral-800">
              <button
                onClick={() => setUploadOpen(true)}
                className="text-xs text-[var(--color-accent-DEFAULT,#F97316)] hover:text-[var(--color-accent-600,#EA580C)] cursor-pointer font-medium"
              >
                + Dokument hinzufuegen
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Upload Dialog */}
      {uploadOpen && (
        <UploadDialog
          gearId={gearId}
          onClose={() => setUploadOpen(false)}
          onSuccess={() => {
            queryClient.invalidateQueries({ queryKey: ['gear-documents-all', gearId] })
            queryClient.invalidateQueries({ queryKey: ['gear-documents', gearId] })
            queryClient.invalidateQueries({ queryKey: ['gear-costs', gearId] })
            setUploadOpen(false)
          }}
        />
      )}

      {/* Preview Panel */}
      {previewDoc && (
        <PreviewPanel
          doc={previewDoc}
          onClose={() => setPreviewDoc(null)}
          onDelete={(docId) => setDeleteConfirmId(docId)}
        />
      )}

      {/* Delete Confirm */}
      {deleteConfirmId && (
        <DeleteConfirmDialog
          onConfirm={() => deleteMutation.mutate(deleteConfirmId)}
          onCancel={() => setDeleteConfirmId(null)}
          isPending={deleteMutation.isPending}
        />
      )}
    </>
  )
}

// ── Upload Dialog ─────────────────────────────────────────────────────────────

function UploadDialog({
  gearId,
  onClose,
  onSuccess,
}: {
  gearId: string
  onClose: () => void
  onSuccess: () => void
}) {
  const [file, setFile] = useState<File | null>(null)
  const [docType, setDocType] = useState('receipt')
  const [title, setTitle] = useState('')
  const [docDate, setDocDate] = useState('')
  const [amount, setAmount] = useState('')
  const [currency, setCurrency] = useState('EUR')
  const [dragOver, setDragOver] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const uploadMutation = useMutation({
    mutationFn: () => uploadGearDocument(gearId, file!, {
      doc_type: docType,
      title: title || undefined,
      document_date: docDate || undefined,
      document_amount: amount ? parseFloat(amount) : undefined,
      document_currency: currency,
    }),
    onSuccess,
  })

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    const f = e.dataTransfer.files[0]
    if (f && f.size <= 10 * 1024 * 1024) setFile(f)
  }, [])

  const handleFileSelect = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    if (f && f.size <= 10 * 1024 * 1024) setFile(f)
  }, [])

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="bg-white dark:bg-neutral-900 rounded-xl shadow-2xl max-w-lg w-full mx-4 overflow-hidden"
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-6 pt-5 pb-3">
          <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Dokument hinzufuegen</h3>
          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full hover:bg-neutral-100 dark:hover:bg-neutral-800 flex items-center justify-center text-neutral-400 hover:text-neutral-600 cursor-pointer transition-colors"
          >
            <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
              <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
            </svg>
          </button>
        </div>

        <div className="px-6 pb-6 space-y-4">
          {/* Drop zone */}
          <div
            onDragOver={e => { e.preventDefault(); setDragOver(true) }}
            onDragLeave={() => setDragOver(false)}
            onDrop={handleDrop}
            onClick={() => fileInputRef.current?.click()}
            className={`rounded-xl border-2 border-dashed p-6 text-center cursor-pointer transition-colors ${
              dragOver
                ? 'border-[var(--color-accent-DEFAULT,#F97316)] bg-[var(--color-accent-DEFAULT,#F97316)]/5'
                : file
                  ? 'border-emerald-300 dark:border-emerald-700 bg-emerald-50 dark:bg-emerald-900/10'
                  : 'border-neutral-200 dark:border-neutral-700 hover:border-neutral-300 dark:hover:border-neutral-600'
            }`}
          >
            <input
              ref={fileInputRef}
              type="file"
              accept=".pdf,.jpg,.jpeg,.png,.webp"
              className="hidden"
              onChange={handleFileSelect}
            />
            {file ? (
              <div className="space-y-1">
                <svg className="w-8 h-8 mx-auto text-emerald-500" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                </svg>
                <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{file.name}</p>
                <p className="text-xs text-neutral-400">{formatFileSize(file.size)}</p>
              </div>
            ) : (
              <div className="space-y-2">
                <svg className="w-10 h-10 mx-auto text-neutral-300 dark:text-neutral-600" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zM6.293 6.707a1 1 0 010-1.414l3-3a1 1 0 011.414 0l3 3a1 1 0 01-1.414 1.414L11 5.414V13a1 1 0 11-2 0V5.414L7.707 6.707a1 1 0 01-1.414 0z" clipRule="evenodd" />
                </svg>
                <p className="text-sm text-neutral-500 dark:text-neutral-400">
                  Datei hierher ziehen oder <span className="text-[var(--color-accent-DEFAULT,#F97316)] font-medium">auswaehlen</span>
                </p>
                <p className="text-xs text-neutral-400">PDF, JPG, PNG — max. 10 MB</p>
              </div>
            )}
          </div>

          {/* Form fields */}
          <div className="grid grid-cols-2 gap-3">
            <div className="col-span-2">
              <label className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-1 block">Typ</label>
              <select
                value={docType}
                onChange={e => setDocType(e.target.value)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-[var(--color-accent-DEFAULT,#F97316)]/40"
              >
                <option value="receipt">Rechnung</option>
                <option value="warranty">Garantie</option>
                <option value="manual">Handbuch</option>
                <option value="photo">Foto</option>
                <option value="other">Sonstiges</option>
              </select>
            </div>

            <div className="col-span-2">
              <label className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-1 block">Titel (optional)</label>
              <input
                type="text"
                value={title}
                onChange={e => setTitle(e.target.value)}
                placeholder="z.B. Kaufbeleg Canyon Spectral"
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 placeholder:text-neutral-400 focus:outline-none focus:ring-2 focus:ring-[var(--color-accent-DEFAULT,#F97316)]/40"
              />
            </div>

            <div>
              <label className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-1 block">Datum (optional)</label>
              <input
                type="date"
                value={docDate}
                onChange={e => setDocDate(e.target.value)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-[var(--color-accent-DEFAULT,#F97316)]/40"
              />
            </div>

            <div className="flex gap-2">
              <div className="flex-1">
                <label className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-1 block">Betrag</label>
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  value={amount}
                  onChange={e => setAmount(e.target.value)}
                  placeholder="0,00"
                  className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 placeholder:text-neutral-400 focus:outline-none focus:ring-2 focus:ring-[var(--color-accent-DEFAULT,#F97316)]/40"
                />
              </div>
              <div className="w-20">
                <label className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-1 block">Waehrung</label>
                <select
                  value={currency}
                  onChange={e => setCurrency(e.target.value)}
                  className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-[var(--color-accent-DEFAULT,#F97316)]/40"
                >
                  <option value="EUR">EUR</option>
                  <option value="USD">USD</option>
                  <option value="GBP">GBP</option>
                  <option value="CHF">CHF</option>
                </select>
              </div>
            </div>
          </div>

          {/* Upload error */}
          {uploadMutation.isError && (
            <p className="text-sm text-rose-500">
              Upload fehlgeschlagen: {uploadMutation.error instanceof Error ? uploadMutation.error.message : 'Unbekannt'}
            </p>
          )}

          {/* Actions */}
          <div className="flex justify-end gap-2 pt-2">
            <button
              onClick={onClose}
              className="px-4 py-2 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
            >
              Abbrechen
            </button>
            <button
              onClick={() => uploadMutation.mutate()}
              disabled={!file || uploadMutation.isPending}
              className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-[var(--color-accent-DEFAULT,#F97316)] hover:bg-[var(--color-accent-600,#EA580C)] disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
            >
              {uploadMutation.isPending ? (
                <span className="flex items-center gap-2">
                  <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                  </svg>
                  Hochladen...
                </span>
              ) : 'Hochladen'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

// ── Preview Panel (right sheet) ──────────────────────────────────────────────────

function PreviewPanel({
  doc,
  onClose,
  onDelete,
}: {
  doc: GearDocument
  onClose: () => void
  onDelete: (id: string) => void
}) {
  const [blobUrl, setBlobUrl] = useState<string | null>(null)
  const [numPages, setNumPages] = useState<number | null>(null)
  const [currentPage, setCurrentPage] = useState(1)
  const [pdfModule, setPdfModule] = useState<typeof import('react-pdf') | null>(null)

  useEffect(() => {
    let url: string | null = null
    downloadGearDocumentBlob(doc.id)
      .then(u => { url = u; setBlobUrl(u) })
      .catch(() => {})
    return () => {
      if (url) URL.revokeObjectURL(url)
    }
  }, [doc.id])

  // Lazy-load react-pdf only for PDFs
  useEffect(() => {
    if (doc.content_type === 'application/pdf') {
      import('react-pdf').then(mod => {
        mod.pdfjs.GlobalWorkerOptions.workerSrc = `//unpkg.com/pdfjs-dist@${mod.pdfjs.version}/build/pdf.worker.min.mjs`
        setPdfModule(mod)
      })
    }
  }, [doc.content_type])

  const handleDownload = useCallback(async () => {
    if (!blobUrl) return
    const a = document.createElement('a')
    a.href = blobUrl
    a.download = doc.original_filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
  }, [blobUrl, doc.original_filename])

  // Close on Escape
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [onClose])

  const isPdf = doc.content_type === 'application/pdf'
  const isImage = doc.content_type.startsWith('image/')

  return (
    <div className="fixed inset-0 z-50 flex" onClick={onClose}>
      {/* Backdrop */}
      <div className="flex-1" />

      {/* Panel */}
      <div
        className="w-full max-w-xl bg-white dark:bg-neutral-900 shadow-2xl border-l border-neutral-200 dark:border-neutral-800 flex flex-col animate-slide-in-right"
        onClick={e => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-neutral-100 dark:border-neutral-800">
          <div className="min-w-0">
            <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100 truncate">
              {doc.title || doc.original_filename}
            </h3>
            <p className="text-xs text-neutral-500 mt-0.5">
              {DOC_TYPE_LABELS[doc.doc_type] ?? doc.doc_type} · {formatFileSize(doc.file_size_bytes)}
              {doc.document_date && ` · ${formatDate(doc.document_date)}`}
            </p>
          </div>
          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full hover:bg-neutral-100 dark:hover:bg-neutral-800 flex items-center justify-center text-neutral-400 hover:text-neutral-600 cursor-pointer transition-colors flex-shrink-0"
          >
            <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
              <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
            </svg>
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-auto p-5">
          {!blobUrl ? (
            <div className="flex items-center justify-center h-64">
              <svg className="animate-spin h-6 w-6 text-neutral-300" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
            </div>
          ) : isPdf && pdfModule ? (
            <div className="flex flex-col items-center gap-3">
              <pdfModule.Document
                file={blobUrl}
                onLoadSuccess={({ numPages: n }) => setNumPages(n)}
                loading={
                  <div className="flex items-center justify-center h-64">
                    <svg className="animate-spin h-6 w-6 text-neutral-300" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                  </div>
                }
              >
                <pdfModule.Page
                  pageNumber={currentPage}
                  width={460}
                  renderAnnotationLayer={false}
                  renderTextLayer={false}
                />
              </pdfModule.Document>
              {numPages && numPages > 1 && (
                <div className="flex items-center gap-3 text-sm text-neutral-600 dark:text-neutral-400">
                  <button
                    onClick={() => setCurrentPage(p => Math.max(1, p - 1))}
                    disabled={currentPage <= 1}
                    className="w-7 h-7 rounded-full hover:bg-neutral-100 dark:hover:bg-neutral-800 flex items-center justify-center disabled:opacity-30 cursor-pointer disabled:cursor-not-allowed"
                  >
                    &#8249;
                  </button>
                  <span>{currentPage} / {numPages}</span>
                  <button
                    onClick={() => setCurrentPage(p => Math.min(numPages, p + 1))}
                    disabled={currentPage >= numPages}
                    className="w-7 h-7 rounded-full hover:bg-neutral-100 dark:hover:bg-neutral-800 flex items-center justify-center disabled:opacity-30 cursor-pointer disabled:cursor-not-allowed"
                  >
                    &#8250;
                  </button>
                </div>
              )}
            </div>
          ) : isImage ? (
            <img
              src={blobUrl}
              alt={doc.title ?? doc.original_filename}
              className="max-w-full rounded-lg"
            />
          ) : (
            <div className="flex flex-col items-center justify-center h-64 text-neutral-400 text-sm gap-2">
              <svg className="w-12 h-12" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4z" clipRule="evenodd" />
              </svg>
              <p>Vorschau nicht verfuegbar</p>
              <p className="text-xs">{doc.content_type}</p>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-5 py-3 border-t border-neutral-100 dark:border-neutral-800">
          <button
            onClick={() => onDelete(doc.id)}
            className="text-xs text-rose-500 hover:text-rose-600 cursor-pointer font-medium"
          >
            Loeschen
          </button>
          <button
            onClick={handleDownload}
            disabled={!blobUrl}
            className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-[var(--color-accent-DEFAULT,#F97316)] hover:bg-[var(--color-accent-600,#EA580C)] disabled:opacity-50 transition-colors cursor-pointer"
          >
            Herunterladen
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Delete Confirm Dialog ──────────────────────────────────────────────────────

function DeleteConfirmDialog({
  onConfirm,
  onCancel,
  isPending,
}: {
  onConfirm: () => void
  onCancel: () => void
  isPending: boolean
}) {
  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50" onClick={onCancel}>
      <div
        className="bg-white dark:bg-neutral-900 rounded-xl shadow-2xl max-w-sm w-full mx-4 p-6"
        onClick={e => e.stopPropagation()}
      >
        <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-2">
          Dokument loeschen?
        </h3>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mb-5">
          Das Dokument wird unwiderruflich geloescht. Dieser Vorgang kann nicht rueckgaengig gemacht werden.
        </p>
        <div className="flex justify-end gap-2">
          <button
            onClick={onCancel}
            className="px-4 py-2 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
          >
            Abbrechen
          </button>
          <button
            onClick={onConfirm}
            disabled={isPending}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-rose-500 hover:bg-rose-600 disabled:opacity-50 transition-colors cursor-pointer"
          >
            {isPending ? 'Wird geloescht...' : 'Loeschen'}
          </button>
        </div>
      </div>
    </div>
  )
}
