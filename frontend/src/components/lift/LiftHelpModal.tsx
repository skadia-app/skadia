import { ModalShell } from '@/components/ui/ModalShell'

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
  if (!open) return null

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-2xl">
      <div className="flex flex-col gap-0 max-h-[80dvh] overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between gap-3 mb-4 shrink-0">
          <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            Wie Skadia Lifte erkennt
          </h2>
          <button
            onClick={onClose}
            aria-label="Schließen"
            className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer transition-colors text-lg leading-none"
          >
            ✕
          </button>
        </div>

        {/* Scrollable body */}
        <div className="overflow-y-auto flex-1 space-y-6 text-sm text-neutral-700 dark:text-neutral-300 pr-1">

          {/* Detection methods overview */}
          <section>
            <p className="mb-3">
              Es gibt zwei unabhängige Verfahren — beide laufen automatisch über jede Aktivität:
            </p>
            <dl className="space-y-4">
              <div>
                <dt className="font-semibold text-neutral-900 dark:text-neutral-100 mb-1">
                  Methode A — OSM-Match
                </dt>
                <dd className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
                  Deine GPS-Spur wird gegen die OpenStreetMap-Lift-Geometrie verglichen.
                  Wenn ein Abschnitt deiner Aktivität auf einem als Lift markierten OSM-Segment liegt,
                  ist der Match eindeutig — diese Lifte werden sofort gezählt, ohne dass du sie bestätigen musst.
                  Voraussetzung: der Lift muss in OSM existieren und korrekt getaggt sein.
                </dd>
              </div>
              <div>
                <dt className="font-semibold text-neutral-900 dark:text-neutral-100 mb-1">
                  Methode B — Heuristik aus dem Stream
                </dt>
                <dd className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
                  Wenn kein OSM-Match vorliegt, analysiert der Detektor die Sensordaten direkt:
                  Geschwindigkeit, Höhenrate, Gradient, Geradlinigkeit. Was wie eine Liftfahrt aussieht
                  — konstante Geschwindigkeit, kontinuierlicher Höhengewinn (oder -verlust!), gerade Linie —
                  wird als Kandidat markiert und wartet auf deine Bestätigung. So werden auch Lifte erkannt
                  die nicht in OSM stehen.
                </dd>
              </div>
            </dl>
          </section>

          {/* Heuristic classes */}
          <section>
            <h3 className="font-semibold text-base text-neutral-900 dark:text-neutral-100 mb-3">
              Heuristik-Klassen
            </h3>
            <p className="text-neutral-600 dark:text-neutral-400 mb-4">
              Innerhalb von Methode B unterscheidet der Detektor drei Klassen je nachdem
              welches Signal am stärksten matcht:
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
                    Heuristik (Speed + Steigung)
                  </div>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
                    Der eindeutigste Fall: stabile Geschwindigkeit mit deutlichem Gradient.
                    Greift wenn dein Speedsensor brauchbare Werte liefert.
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
                    Heuristik (Höhenrate)
                  </div>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
                    Der Speedsensor lieferte nichts brauchbares (z.B. Smart-Recording, Stationsphasen)
                    — der Detektor verlässt sich nur auf die Höhenmeter pro Sekunde.
                    Niedrigere Konfidenz, prüfe ggf. selbst.
                  </p>
                </div>
              </div>

              {/* Bidir / Talquerung */}
              <div className="flex gap-3">
                <div
                  className="shrink-0 mt-0.5 w-1 rounded-full"
                  style={{ backgroundColor: '#8b5cf6', minHeight: '1em', alignSelf: 'stretch' }}
                  aria-hidden="true"
                />
                <div>
                  <div className="font-medium text-neutral-900 dark:text-neutral-100 mb-1">
                    Heuristik (Talquerung)
                  </div>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed mb-3">
                    Spezialfall für Pendel-Seilbahnen mit U-förmigem Cable-Verlauf:
                  </p>
                  <pre
                    className="text-xs leading-tight font-mono bg-neutral-100 dark:bg-neutral-800 rounded-lg px-4 py-3 overflow-x-auto text-neutral-700 dark:text-neutral-300"
                    aria-label="Talquerung ASCII-Diagramm"
                  >
{`Bergstation A 2446 m ────╮                 ╭──── Bergstation B 2444 m
                          ╲               ╱
                           ╲─────────────╱
                              Tal ~2000 m`}
                  </pre>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed mt-3">
                    EINE Liftfahrt — nicht zwei separate Rides. Netto-Höhe ändert sich kaum (~2 m),
                    aber kumulativ wurden ~900 m durchfahren (450 m runter + 450 m hoch).
                    Cable ist kerzengerade, Geschwindigkeit konstant.
                    Häufig bei Talquerungs-Cable-Cars zwischen Berggipfeln.
                  </p>
                </div>
              </div>
            </div>
          </section>

          {/* Pause gaps */}
          <section>
            <h3 className="font-semibold text-base text-neutral-900 dark:text-neutral-100 mb-2">
              Pause-Lücken &gt; 75 s
            </h3>
            <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
              Wenn dein Gerät die Aufzeichnung pausiert (Smart-Recording / Auto-Pause / Sleep)
              und der Gap länger als 75 Sekunden dauert, kann der Detektor keinen Lift über
              diese Lücke hinweg erkennen — er braucht zusammenhängende Stream-Daten in
              seinem Sliding-Window. Die Karte zeigt die Lücke als gestrichelte orange Linie
              damit du siehst <em>wo</em> die Pause war.
            </p>
          </section>
        </div>
      </div>
    </ModalShell>
  )
}
