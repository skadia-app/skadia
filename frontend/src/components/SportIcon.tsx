import { categoryForSport } from '@/lib/sportTaxonomy'

interface Props {
  sport: string
  subType?: string | null
  className?: string
}

export function SportIcon({ sport, subType, className = 'w-4 h-4' }: Props) {
  if (sport === 'cycling' || subType?.includes('bike') || subType?.includes('cycling')) {
    if (subType === 'e_bike_fitness' || subType === 'e_bike_mountain') return <EBikeIcon className={className} />
    if (sport === 'e_biking') return <EBikeIcon className={className} />
    if (subType === 'mountain') return <MtbIcon className={className} />
    if (subType === 'road') return <RoadBikeIcon className={className} />
    return <BikeIcon className={className} />
  }

  // Direct sport → icon mapping
  const directIcons: Record<string, React.FC<{ className?: string }>> = {
    cycling: BikeIcon,
    e_biking: EBikeIcon,
    inline_skating: BikeIcon,
    running: RunIcon,
    hiking: HikeIcon,
    walking: WalkIcon,
    mountaineering: HikeIcon,
    snowshoeing: HikeIcon,
    swimming: SwimIcon,
    alpine_skiing: SkiIcon,
    cross_country_skiing: SkiIcon,
    snowboarding: SkiIcon,
    ice_skating: SkiIcon,
    snowmobiling: SkiIcon,
    paddling: PaddleIcon,
    kayaking: PaddleIcon,
    stand_up_paddleboarding: SupIcon,
    rowing: PaddleIcon,
    training: StrengthIcon,
    fitness_equipment: StrengthIcon,
    hiit: StrengthIcon,
    rock_climbing: RockClimbingIcon,
    generic: GenericIcon,
    unknown: UnknownIcon,
  }

  const directIcon = directIcons[sport]
  if (directIcon) {
    const Icon = directIcon
    return <Icon className={className} />
  }

  // Category-based fallback for unknown sport types
  const cat = categoryForSport(sport)
  const fallbacks: Record<string, React.FC<{ className?: string }>> = {
    run: RunIcon,
    bike: BikeIcon,
    winter: SkiIcon,
    water: SwimIcon,
    fitness: StrengthIcon,
    game: GenericIcon,
    climb: RockClimbingIcon,
    outdoor: HikeIcon,
    multi: GenericIcon,
    other: GenericIcon,
  }

  const FallbackIcon = fallbacks[cat.key] ?? UnknownIcon
  return <FallbackIcon className={className} />
}

// ── Fahrrad-Varianten ─────────────────────────────────────────────────────────

/**
 * Normales Stadtrad / Trekking — aufrechter Lenker, entspannte Geometrie
 */
function BikeIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="18.5" cy="17.5" r="3.5" />
      <circle cx="5.5" cy="17.5" r="3.5" />
      {/* Sattel */}
      <line x1="13.5" y1="5" x2="16.5" y2="5" />
      <line x1="15" y1="5" x2="15" y2="7" />
      {/* Rahmen: Sattelstrebe → Tretlager → Steuerrohr */}
      <path d="M15 7 L12 17.5" />
      <path d="M15 7 L10 11.5 L5.5 17.5" />
      <path d="M12 17.5 L10 11.5" />
      {/* Aufrechter Lenker */}
      <line x1="10" y1="11.5" x2="10" y2="9" />
      <line x1="8.5" y1="9" x2="11.5" y2="9" />
      {/* Hintere Strebe */}
      <path d="M18.5 17.5 L15 7" />
    </svg>
  )
}

/**
 * Rennrad — Dropbar, geduckter Rahmen, Speedlines
 */
function RoadBikeIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="18.5" cy="17.5" r="3.5" />
      <circle cx="5.5" cy="17.5" r="3.5" />
      {/* Sattel — flach und weit hinten */}
      <line x1="14" y1="6" x2="17" y2="6" />
      <line x1="15.5" y1="6" x2="15.5" y2="8" />
      {/* Rahmen — aggressivere, flachere Geometrie */}
      <path d="M15.5 8 L12 17.5" />
      <path d="M15.5 8 L9.5 11 L5.5 17.5" />
      <path d="M12 17.5 L9.5 11" />
      {/* Hintere Strebe */}
      <path d="M18.5 17.5 L15.5 8" />
      {/* Drop-Lenker — Stiel hoch, Bar curvet nach unten */}
      <path d="M9.5 11 L9.5 9" />
      <path d="M9.5 9 C11 9 11.5 9.5 11.5 11.5" strokeWidth="1.6" />
      {/* Speedlines links */}
      <line x1="0.5" y1="13" x2="3" y2="13" strokeWidth="1.3" />
      <line x1="0.5" y1="15.5" x2="2.5" y2="15.5" strokeWidth="1.3" />
    </svg>
  )
}

/**
 * MTB — dicke Reifen, Federgabel, Berge im Hintergrund
 */
function MtbIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      {/* Dicke Reifen (doppelter Kreis) */}
      <circle cx="18.5" cy="17.5" r="3.5" />
      <circle cx="18.5" cy="17.5" r="2" strokeWidth="1" strokeOpacity="0.4" />
      <circle cx="5.5" cy="17.5" r="3.5" />
      <circle cx="5.5" cy="17.5" r="2" strokeWidth="1" strokeOpacity="0.4" />
      {/* Sattel */}
      <line x1="13.5" y1="5" x2="16.5" y2="5" />
      <line x1="15" y1="5" x2="15" y2="7" />
      {/* Rahmen */}
      <path d="M15 7 L12 17.5" />
      <path d="M15 7 L10 11 L5.5 17.5" />
      <path d="M12 17.5 L10 11" />
      <path d="M18.5 17.5 L15 7" />
      {/* Federgabel — Gabelholm mit Querstrebe */}
      <path d="M10 11 L8 14 L5.5 17.5" />
      <path d="M10 11 L7 14" strokeOpacity="0.5" strokeWidth="1.3" />
      <line x1="7" y1="14" x2="8" y2="14" strokeWidth="1.2" />
      {/* Berge oben links */}
      <path d="M1 13 L3.5 8 L6 13" strokeWidth="1.3" strokeOpacity="0.6" />
    </svg>
  )
}

/**
 * E-Bike — Rahmen wie normales Rad + Blitz
 */
function EBikeIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="18.5" cy="17.5" r="3.5" />
      <circle cx="5.5" cy="17.5" r="3.5" />
      {/* Rahmen (identisch BikeIcon) */}
      <path d="M15 7 L12 17.5" />
      <path d="M15 7 L10 11.5 L5.5 17.5" />
      <path d="M12 17.5 L10 11.5" />
      <path d="M18.5 17.5 L15 7" />
      {/* Aufrechter Lenker */}
      <line x1="10" y1="11.5" x2="10" y2="9.5" />
      <line x1="8.5" y1="9.5" x2="11.5" y2="9.5" />
      {/* Blitz — auf dem Oberrohr */}
      <path d="M16 3 L13.5 7 L16 7 L13.5 11" strokeWidth="1.6" strokeLinejoin="round" />
    </svg>
  )
}

// ── Andere Sportarten ─────────────────────────────────────────────────────────

function RunIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="13" cy="4" r="1" />
      <path d="M7 21l3-4 2 1 2-4" />
      <path d="M16 21l-2-7-3-1 1-5" />
      <path d="m8 11 2-5 4 1 2-2" />
    </svg>
  )
}

function HikeIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="4" r="1" />
      <path d="m9 20 3-7 3 7" />
      <path d="M9 12H6l2-5h5l1 3" />
      <path d="M6 20h3M15 20h3" />
      <path d="m16 11 2 9" />
    </svg>
  )
}

function SwimIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M2 20c2-2 4-2 6 0s4 2 6 0 4-2 6 0" />
      <path d="M2 16c2-2 4-2 6 0s4 2 6 0 4-2 6 0" />
      <circle cx="13" cy="6" r="1" />
      <path d="m15 11-4-1-2-4 3-1 2 3 4 1" />
    </svg>
  )
}

function SkiIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="7" cy="5" r="1" />
      <path d="m6 8 1 3 5 2 2 4-1 2-8-3" />
      <path d="m3 20 18-6" />
      <path d="m3 17 18-6" />
    </svg>
  )
}

function PaddleIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 19V5" />
      <path d="M9 8c0-2 1.5-3 3-3s3 1 3 3-1.5 3-3 3-3-1-3-3z" />
      <path d="M9 16c0 2 1.5 3 3 3s3-1 3-3-1.5-3-3-3-3 1-3 3z" />
    </svg>
  )
}

function WalkIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="13" cy="4" r="1" />
      <path d="m10 21 2-6 3 3" />
      <path d="m8 21 4-12 4 2" />
      <path d="M6 14h3" />
    </svg>
  )
}

function StrengthIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M6.5 6.5h11M6.5 17.5h11" />
      <rect x="2" y="5" width="4" height="4" rx="1" />
      <rect x="2" y="15" width="4" height="4" rx="1" />
      <rect x="18" y="5" width="4" height="4" rx="1" />
      <rect x="18" y="15" width="4" height="4" rx="1" />
      <path d="M6.5 12h11" />
    </svg>
  )
}

/**
 * Klettern / Bouldern — Kletterer an Wand mit Griffen
 */
function RockClimbingIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      {/* Wand */}
      <line x1="22" y1="2" x2="22" y2="22" />
      {/* Griffe */}
      <line x1="20" y1="4" x2="22" y2="4" strokeWidth="2.2" />
      <line x1="20" y1="13" x2="22" y2="13" strokeWidth="2.2" />
      {/* Kopf */}
      <circle cx="15" cy="4.5" r="1.5" />
      {/* Rumpf */}
      <line x1="15" y1="6" x2="13" y2="12" />
      {/* Rechter Arm — greift oberen Griff */}
      <path d="M15 7.5 L20 4" />
      {/* Linker Arm — Balance */}
      <path d="M15 7.5 L11 8.5" />
      {/* Rechtes Bein — Fuss an Wand */}
      <path d="M13 12 L19 16" />
      {/* Linkes Bein */}
      <path d="M13 12 L11 17" />
    </svg>
  )
}

/**
 * Stand-Up Paddleboarding — stehende Figur auf Board mit Paddel
 */
function SupIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      {/* Kopf */}
      <circle cx="11" cy="6" r="1.5" />
      {/* Rumpf */}
      <line x1="11" y1="7.5" x2="11" y2="14" />
      {/* Beine */}
      <line x1="11" y1="14" x2="10" y2="18" />
      <line x1="11" y1="14" x2="12" y2="18" />
      {/* Paddelschaft — diagonal */}
      <line x1="15" y1="4" x2="8" y2="21" />
      {/* Paddelblatt */}
      <path d="M7 19.5 L8.5 22 L10 19.5" />
      {/* Obere Hand */}
      <line x1="11" y1="9" x2="13.5" y2="7" />
      {/* Untere Hand */}
      <line x1="11" y1="12" x2="9.5" y2="13" />
      {/* Board */}
      <path d="M4 20 Q11.5 18 19 20" />
    </svg>
  )
}

/**
 * Sonstige / Generic — Aktivitaets-Pulslinie
 */
function GenericIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M2 12 L6 12 L8 6 L11 18 L14 9 L16 14 L18 12 L22 12" />
    </svg>
  )
}

function UnknownIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="10" />
      <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3" />
      <path d="M12 17h.01" />
    </svg>
  )
}
