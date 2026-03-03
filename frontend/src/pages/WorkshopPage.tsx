import { Link } from 'react-router-dom'

const sections = [
  {
    to: '/workshop/profile',
    title: 'Physiologisches Profil',
    description: 'Körperdaten, FTP, LTHR und Herzfrequenz-Werte verwalten',
    icon: ProfileIcon,
  },
  {
    to: '/gear',
    title: 'Ausrüstung',
    description: 'Räder, Schuhe, Zubehör und Komponenten verwalten',
    icon: GearIcon,
  },
  {
    to: '/workshop/tags',
    title: 'Tags',
    description: 'Tags für Aktivitäten erstellen und verwalten',
    icon: TagIcon,
  },
  {
    to: '/workshop/reprocess',
    title: 'Neu verarbeiten',
    description: 'Aktivitäten erneut durch die Enrichment-Pipeline verarbeiten',
    icon: ReprocessIcon,
  },
]

export default function WorkshopPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Werkstatt</h1>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Verwaltung deiner importierten Daten und Ausrüstung.
        </p>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {sections.map(({ to, title, description, icon: Icon }) => (
          <Link
            key={title}
            to={to}
            className="group rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-3
                       hover:border-orange-300 dark:hover:border-orange-700 hover:shadow-sm transition-all"
          >
            <div className="flex items-center gap-3">
              <div className="flex items-center justify-center w-9 h-9 rounded-lg bg-orange-50 dark:bg-orange-900/20 text-orange-500">
                <Icon />
              </div>
              <h2 className="text-base font-medium text-neutral-900 dark:text-neutral-100 group-hover:text-orange-500 transition-colors">
                {title}
              </h2>
            </div>
            <p className="text-sm text-neutral-500 dark:text-neutral-400">{description}</p>
          </Link>
        ))}
      </div>
    </div>
  )
}

function ProfileIcon() {
  return (
    <svg className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2" />
      <circle cx="12" cy="7" r="4" />
    </svg>
  )
}

function GearIcon() {
  return (
    <svg className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-4 0v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83 0 2 2 0 010-2.83l.06-.06A1.65 1.65 0 004.68 15a1.65 1.65 0 00-1.51-1H3a2 2 0 010-4h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 012.83-2.83l.06.06A1.65 1.65 0 009 4.68a1.65 1.65 0 001-1.51V3a2 2 0 014 0v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 2.83l-.06.06A1.65 1.65 0 0019.4 9a1.65 1.65 0 001.51 1H21a2 2 0 010 4h-.09a1.65 1.65 0 00-1.51 1z" />
    </svg>
  )
}

function TagIcon() {
  return (
    <svg className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M20.59 13.41l-7.17 7.17a2 2 0 01-2.83 0L2 12V2h10l8.59 8.59a2 2 0 010 2.82z" />
      <line x1="7" y1="7" x2="7.01" y2="7" />
    </svg>
  )
}

function ReprocessIcon() {
  return (
    <svg className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21.5 2v6h-6M2.5 22v-6h6" />
      <path d="M2.5 11.5a10 10 0 0118.4-4.5M21.5 12.5a10 10 0 01-18.4 4.5" />
    </svg>
  )
}
