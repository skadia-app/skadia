import { ModalShell } from '@/components/ui/ModalShell'
import { XIcon } from 'lucide-react'

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
              Es gibt zwei unabhängige Verfahren. Beide laufen automatisch über jede Aktivität:
            </p>
            <dl className="space-y-4">
              <div>
                <dt className="font-semibold text-neutral-900 dark:text-neutral-100 mb-1">
                  Methode A: OSM-Match
                </dt>
                <dd className="text-neutral-600 dark:text-neutral-400 leading-relaxed space-y-2">
                  <p>
                    Deine GPS-Spur wird gegen die OpenStreetMap-Lift-Geometrie verglichen.
                    Wenn ein Abschnitt deiner Aktivität auf einem als Lift markierten OSM-Segment liegt,
                    wird er sofort als Lift gezählt. Voraussetzung: der Lift muss in OSM existieren,
                    korrekt getaggt sein und in der lokalen Datenbank importiert sein.
                  </p>
                  <p>
                    <strong>Trotzdem ablehnbar:</strong> der Match ist geometrisch, nicht semantisch.
                    Wenn du z.B. mit dem E-Bike unterhalb der Liftspur bergauf gefahren bist, kann
                    deine GPS-Spur knapp neben der OSM-Geometrie verlaufen und der Match feuert
                    fälschlich. Du kannst OSM-Treffer einzeln über „Ablehnen" entfernen.
                  </p>
                  <p>
                    <strong>Lift fehlt oder ist falsch eingetragen?</strong> Wenn du sicher bist, dass deine
                    GPS-Spur korrekt ist, der Lift in OSM aber versetzt liegt oder fehlt, kannst du
                    OpenStreetMap selbst korrigieren:{' '}
                    <a
                      href="https://wiki.openstreetmap.org/wiki/How_to_contribute"
                      target="_blank"
                      rel="noreferrer"
                      className="text-accent-DEFAULT hover:underline"
                    >
                      Anleitung zum Bearbeiten
                    </a>
                    . Skadia übernimmt deine Änderung beim nächsten OSM-Import.
                  </p>
                </dd>
              </div>
              <div>
                <dt className="font-semibold text-neutral-900 dark:text-neutral-100 mb-1">
                  Methode B: Heuristik aus dem Stream
                </dt>
                <dd className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
                  Wenn kein OSM-Match vorliegt, analysiert der Detektor die Sensordaten direkt:
                  Geschwindigkeit, Höhenrate, Gradient, Geradlinigkeit. Typische Signale sind
                  konstante Geschwindigkeit, kontinuierlicher Höhengewinn oder Höhenverlust und
                  eine gerade Linie. Solche Abschnitte werden als Kandidaten markiert und warten
                  auf deine Bestätigung. So werden auch Lifte erkannt, die nicht in OSM stehen.
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
                    Greift, wenn dein Speedsensor während der Liftfahrt non-Null-Werte liefert.
                    Das ist typisch bei Schleppliften, weil Ski oder Snowboard Bodenkontakt haben,
                    oder wenn die Geschwindigkeit aus GPS abgeleitet wird.
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
                    Greift wenn ein Speedsensor zwar montiert ist, aber während der Liftfahrt
                    keine brauchbare Geschwindigkeit liefert (Ski/Snowboard ohne Bodenkontakt in
                    Gondel oder Sessellift) oder Smart-Recording die Speed-Werte ganz fallen lässt.
                    Der Detektor verlässt sich dann nur auf die Höhenmeter pro Sekunde. Die
                    Konfidenz ist niedriger; prüfe den Kandidaten deshalb bei Bedarf selbst.
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
                    Heuristik (bidirektional)
                  </div>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed mb-2">
                    Klasse für Lifte mit U- oder ∩-förmigem Höhenprofil: Start- und Endpunkt
                    liegen auf ähnlicher Höhe, dazwischen sinkt oder steigt die Trasse aber
                    deutlich. Beim Befahren ergeben sich dadurch sowohl Anstieg als auch
                    Abstieg, obwohl die Netto-Höhenänderung am Ende klein ist.
                  </p>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed mb-3">
                    <strong>Wichtig:</strong> Die Bezeichnung bezieht sich auf den Verlauf der
                    Liftstrecke, nicht auf die Richtung der Fahrt. Eine bidirektionale
                    Klassifizierung beschreibt eine einzige Liftfahrt entlang einer U- oder
                    ∩-förmigen Trasse. Ein Hin-und-Rück-Trip wären dagegen zwei separate
                    Fahrten; der Detektor zählt sie auch so.
                  </p>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed mb-2">
                    Typische Fälle:
                  </p>
                  <ul className="text-neutral-600 dark:text-neutral-400 leading-relaxed mb-3 list-disc list-inside space-y-1">
                    <li>Talquerung (∪): Die Trasse verbindet zwei Bergstationen ähnlicher Höhe und hängt zwischen ihnen ins Tal ab.</li>
                    <li>Bergquerung (∩): Die Trasse führt von der Talstation über einen Grat hinüber zu einer Alm oder einem Hof auf vergleichbarer Höhe.</li>
                  </ul>
                  <pre
                    className="text-xs leading-tight font-mono bg-neutral-100 dark:bg-neutral-800 rounded-lg px-4 py-3 overflow-x-auto text-neutral-700 dark:text-neutral-300"
                    aria-label="Bidirektional ASCII-Diagramm"
                  >
{`Bergstation A 2446 m ────╮                 ╭──── Bergstation B 2444 m
                          ╲               ╱
                           ╲─────────────╱
                              Tal ~2000 m`}
                  </pre>
                  <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed mt-3">
                    Der Detektor kann beide Geometrien nicht voneinander unterscheiden. Er
                    sieht in beiden Fällen einen horizontal direkten Pfad, stabile
                    Geschwindigkeit und kumulierte Höhenmeter, die sich zu einer kleinen
                    Netto-Änderung aufsummieren.
                  </p>
                </div>
              </div>
            </div>
          </section>

          {/* Pipeline modes / toggles */}
          <section>
            <h3 className="font-semibold text-base text-neutral-900 dark:text-neutral-100 mb-2">
              Pipeline-Modi (an-/abschaltbar)
            </h3>
            <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed mb-3">
              Du kannst die beiden Methoden einzeln ein- oder ausschalten. Global geht das in
              den Profil-Einstellungen unter „Lift-Erkennung"; pro Aktivität direkt oben im
              Review-Modal über die Toggles <strong>„OSM-Snap"</strong> und{' '}
              <strong>„Heuristik"</strong>.
            </p>
            <dl className="text-neutral-600 dark:text-neutral-400 leading-relaxed space-y-2 mb-3">
              <div>
                <dt className="font-medium text-neutral-900 dark:text-neutral-100">Beide an (Default)</dt>
                <dd>OSM-Match hat Priorität, Heuristik füllt Lücken wo OSM nichts liefert.</dd>
              </div>
              <div>
                <dt className="font-medium text-neutral-900 dark:text-neutral-100">Nur OSM-Snap</dt>
                <dd>
                  Reine Geometrie-Matches gegen importierte OSM-Lifte. Schnell, kein
                  Bestätigungsschritt nötig. Fehlende Lifte findet diese Methode aber nicht.
                </dd>
              </div>
              <div>
                <dt className="font-medium text-neutral-900 dark:text-neutral-100">Nur Heuristik</dt>
                <dd>
                  Reine Stream-Analyse. Findet auch Lifte die nicht in OSM stehen, jede
                  Detektion landet als Vorschlag zur Bestätigung im Review-Modal.
                </dd>
              </div>
              <div>
                <dt className="font-medium text-neutral-900 dark:text-neutral-100">Beide aus</dt>
                <dd>
                  Keine automatische Erkennung. Manuelle Soft-Rejects und bestätigte Lifte
                  bleiben erhalten.
                </dd>
              </div>
            </dl>
            <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
              Änderst du eine Einstellung, wird die Aktivität im Hintergrund neu analysiert
              („Anwenden"). Wenn bereits ein Job läuft, bekommst du einen dezenten Hinweis und
              kannst es später erneut versuchen.
            </p>
          </section>

          {/* Pause gaps */}
          <section>
            <h3 className="font-semibold text-base text-neutral-900 dark:text-neutral-100 mb-2">
              Pause-Lücken &gt; 75 s
            </h3>
            <p className="text-neutral-600 dark:text-neutral-400 leading-relaxed">
              Wenn dein Gerät die Aufzeichnung pausiert (Smart-Recording / Auto-Pause / Sleep)
              und die Unterbrechung länger als 75 Sekunden dauert, kann der Detektor keinen
              Lift über diese Lücke hinweg erkennen. Dafür braucht er zusammenhängende
              Stream-Daten in seinem gleitenden Zeitfenster. Die Karte zeigt die Lücke als
              gestrichelte graue Linie, damit du siehst, <em>wo</em> die Pause war.
            </p>
          </section>
        </div>
      </div>
    </ModalShell>
  )
}
