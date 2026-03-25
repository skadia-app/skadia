import AdminStorageDashboard from '@/components/AdminStorageDashboard'

// AdminSettingsTab is currently a thin wrapper around AdminStorageDashboard.
// Additional admin sections (user management, system config, etc.) will be
// added here as separate components once implemented.
export default function AdminSettingsTab() {
  return (
    <div className="space-y-8">
      <AdminStorageDashboard />
    </div>
  )
}
