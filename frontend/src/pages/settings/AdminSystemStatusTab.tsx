import { useQuery } from '@tanstack/react-query'
import { fetchSystemStatus } from '@/api/systemStatus'
import { SettingsSection } from '@/components/ui/SettingsSection'
import { frontendBuildInfo } from '@/lib/buildInfo'

export default function AdminSystemStatusTab() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['admin-system-status'],
    queryFn: fetchSystemStatus,
    staleTime: 15_000,
  })

  if (isLoading) return <div className="text-sm text-neutral-500">Status wird geladen…</div>
  if (isError || !data) return <div className="text-sm text-red-500">Systemstatus konnte nicht geladen werden.</div>

  const build = {
    ...data.build,
    frontend: data.build.frontend ?? frontendBuildInfo(),
  }

  return (
    <div className="space-y-6">
      <SettingsSection title="Systemstatus" description="Laufzeitstatus, Build-Herkunft und Diagnosehinweise.">
        <div className="space-y-2 text-sm">
          <div>Gesamtstatus: {data.overall}</div>
          <div className="text-xs text-neutral-500 dark:text-neutral-400">
            Frontend-Build: {build.frontend.git_commit} · Commit-Zeit: {build.frontend.commit_time}
          </div>
        </div>
      </SettingsSection>
    </div>
  )
}
