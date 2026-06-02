import { ModalShell } from '@/components/ui/ModalShell'
import { XIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

interface LiftHelpModalProps {
  open: boolean
  onClose: () => void
}

/**
 * Explains Skadia's two lift detection methods (OSM-match vs Heuristic stream
 * analysis) and the three heuristic signal classes, including a monospace
 * Talquerung diagram. Opened from three entry points in LiftReviewModal:
 * legend overlay, pause-gap banner, and the modal header.
 */
export default function LiftHelpModal({ open, onClose }: LiftHelpModalProps) {
  const { t } = useTranslation()
  if (!open) return null

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-2xl">
      <div className="flex flex-col gap-0 max-h-[80dvh] overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between gap-3 mb-4 shrink-0">
          <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            {t('lift.help.title')}
          </h2>
          <button
            onClick={onClose}
            aria-label={t('common.close')}
            className="cursor-pointer rounded-md p-1 text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-neutral-800 dark:hover:text-neutral-200"
          >
            <XIcon className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>

        {/* Scrollable body */}
        <div className="overflow-y-auto flex-1 space-y-6 text-sm text-neutral-700 dark:text-neutral-300 pr-1">

          {/* Detection methods overview */}
          <section>
            <p className="mb-3">
              {t('lift.help.intro')}
            </p>
            <dl className="space-y-4">
              <div>
                <dt className="font-semibold text-neutral-900 dark:text-neutral-100 mb-1">
                  {t('lift.help.osm.title')}
                </dt>
                <dd className="text-neutral-600 dark:text-neutral-400 leading-relaxed space-y-2">
                  <p>
                    {t('lift.help.osm.body')}
                  </p>
                  <p>
                    <strong>{t('lift.help.osm.rejectableLead')}</strong> {t('lift.help.osm.rejectableBody')}
                  </p>
                  <p>
                    <strong>{t('lift.help.osm.editLead')}</strong> {t('lift.help.osm.editBody')}{' '}
                    <a
                      href="https://wiki.openstreetmap.org/wiki/How_to_contribute"
                      target="_blank"
                      rel="noreferrer"
                      className="text-accent-DEFAULT hover:underline"
                    >
                      {t('lift.help.osm.editLink')}
                    </a>
                    {t('lift.help.osm.editSuffix')}
                  </p>
                </dd>
              </div>
              <div>
                <dt className="font-semibold text-neutral-900 dark:text-neutral-100 mb-1">
                  {t('lift.help.heuristic.title')}
                </dt>
                <dd className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
                  {t('lift.help.heuristic.body')}
                </dd>
              </div>
            </dl>
          </section>

          {/* Heuristic classes */}
          <section>
            <h3 className="font-semibold text-base text-neutral-900 dark:text-neutral-100 mb-3">
              {t('lift.help.classes.title')}
            </h3>
            <p className="text-neutral-600 dark:text-neutral-400 mb-4">
              {t('lift.help.classes.intro')}
            </p>

            <div className="space-y-4">
              {/* High */}
              <div className="flex gap-3">
                <div
                  className="shrink-0 mt-0.5 w-1 rounded-full"
                  style={{ backgroundColor: '#22c55e', minHeight: '1em', alignSelf: 'stretch' }}
                  aria-hidden="true"
                />
                <div>
                  <div className="font-medium text-neutral-900 dark:text-neutral-100 mb-1">
                    {t('lift.help.classes.high.title')}
                  </div>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
                    {t('lift.help.classes.high.body')}
                  </p>
                </div>
              </div>

              {/* Medium */}
              <div className="flex gap-3">
                <div
                  className="shrink-0 mt-0.5 w-1 rounded-full"
                  style={{ backgroundColor: '#f97316', minHeight: '1em', alignSelf: 'stretch' }}
                  aria-hidden="true"
                />
                <div>
                  <div className="font-medium text-neutral-900 dark:text-neutral-100 mb-1">
                    {t('lift.help.classes.medium.title')}
                  </div>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
                    {t('lift.help.classes.medium.body')}
                  </p>
                </div>
              </div>

              {/* Bidir */}
              <div className="flex gap-3">
                <div
                  className="shrink-0 mt-0.5 w-1 rounded-full"
                  style={{ backgroundColor: '#8b5cf6', minHeight: '1em', alignSelf: 'stretch' }}
                  aria-hidden="true"
                />
                <div>
                  <div className="font-medium text-neutral-900 dark:text-neutral-100 mb-1">
                    {t('lift.help.classes.bidir.title')}
                  </div>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed mb-2">
                    {t('lift.help.classes.bidir.body1')}
                  </p>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed mb-3">
                    <strong>{t('lift.help.classes.bidir.importantLead')}</strong> {t('lift.help.classes.bidir.body2')}
                  </p>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed mb-2">
                    {t('lift.help.classes.bidir.typicalCases')}
                  </p>
                  <ul className="text-neutral-600 dark:text-neutral-400 leading-relaxed mb-3 list-disc list-inside space-y-1">
                    <li>{t('lift.help.classes.bidir.caseValley')}</li>
                    <li>{t('lift.help.classes.bidir.caseRidge')}</li>
                  </ul>
                  <pre
                    className="text-xs leading-tight font-mono bg-neutral-100 dark:bg-neutral-800 rounded-lg px-4 py-3 overflow-x-auto text-neutral-700 dark:text-neutral-300"
                    aria-label={t('lift.help.classes.bidir.diagramLabel')}
                  >
{t('lift.help.classes.bidir.diagram')}
                  </pre>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed mt-3">
                    {t('lift.help.classes.bidir.body3')}
                  </p>
                </div>
              </div>
            </div>
          </section>

          {/* Pipeline modes / toggles */}
          <section>
            <h3 className="font-semibold text-base text-neutral-900 dark:text-neutral-100 mb-2">
              {t('lift.help.pipeline.title')}
            </h3>
            <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed mb-3">
              {t('lift.help.pipeline.introPrefix')}{' '}
              <strong>{t('lift.help.pipeline.osmSnap')}</strong> {t('lift.help.pipeline.and')}{' '}
              <strong>{t('lift.help.pipeline.heuristic')}</strong>.
            </p>
            <dl className="text-neutral-600 dark:text-neutral-400 leading-relaxed space-y-2 mb-3">
              <div>
                <dt className="font-medium text-neutral-900 dark:text-neutral-100">{t('lift.help.pipeline.bothOn.title')}</dt>
                <dd>{t('lift.help.pipeline.bothOn.body')}</dd>
              </div>
              <div>
                <dt className="font-medium text-neutral-900 dark:text-neutral-100">{t('lift.help.pipeline.osmOnly.title')}</dt>
                <dd>
                  {t('lift.help.pipeline.osmOnly.body')}
                </dd>
              </div>
              <div>
                <dt className="font-medium text-neutral-900 dark:text-neutral-100">{t('lift.help.pipeline.heuristicOnly.title')}</dt>
                <dd>
                  {t('lift.help.pipeline.heuristicOnly.body')}
                </dd>
              </div>
              <div>
                <dt className="font-medium text-neutral-900 dark:text-neutral-100">{t('lift.help.pipeline.bothOff.title')}</dt>
                <dd>
                  {t('lift.help.pipeline.bothOff.body')}
                </dd>
              </div>
            </dl>
            <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
              {t('lift.help.pipeline.applyNote')}
            </p>
          </section>

          {/* Pause gaps */}
          <section>
            <h3 className="font-semibold text-base text-neutral-900 dark:text-neutral-100 mb-2">
              {t('lift.help.pauseGaps.title')}
            </h3>
            <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
              {t('lift.help.pauseGaps.bodyPrefix')} <em>{t('lift.help.pauseGaps.where')}</em> {t('lift.help.pauseGaps.bodySuffix')}
            </p>
          </section>
        </div>
      </div>
    </ModalShell>
  )
}
