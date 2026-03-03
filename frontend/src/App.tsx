import { lazy, Suspense } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { ThemeProvider } from '@/lib/theme'
import { AccentColorProvider } from '@/lib/accentColor'
import Layout from '@/components/Layout'
import AuthGuard from '@/components/AuthGuard'

// Lazy-loaded pages (route-based code splitting)
const DashboardPage = lazy(() => import('@/pages/DashboardPage'))
const ActivitiesPage = lazy(() => import('@/pages/ActivitiesPage'))
const ActivityDetailPage = lazy(() => import('@/pages/ActivityDetailPage'))
const MapPage = lazy(() => import('@/pages/MapPage'))
const SegmentsPage = lazy(() => import('@/pages/SegmentsPage'))
const GearPage = lazy(() => import('@/pages/GearPage'))
const GearDetailPage = lazy(() => import('@/pages/GearDetailPage'))
const TagsPage = lazy(() => import('@/pages/TagsPage'))
const WorkshopPage = lazy(() => import('@/pages/WorkshopPage'))
const WorkshopReprocessPage = lazy(() => import('@/pages/WorkshopReprocessPage'))
const PhysiologicalProfilePage = lazy(() => import('@/pages/PhysiologicalProfilePage'))
const CalendarPage = lazy(() => import('@/pages/CalendarPage'))
const SettingsPage = lazy(() => import('@/pages/SettingsPage'))
const PrivacySettingsPage = lazy(() => import('@/pages/PrivacySettingsPage'))
const EnrichmentSettingsPage = lazy(() => import('@/pages/EnrichmentSettingsPage'))
const AutomationRulesPage = lazy(() => import('@/pages/AutomationRulesPage'))
const LoginPage = lazy(() => import('@/pages/LoginPage'))
const CallbackPage = lazy(() => import('@/pages/CallbackPage'))

function LoadingSpinner() {
  return (
    <div className="flex h-full items-center justify-center">
      <div className="h-8 w-8 animate-spin rounded-full border-4 border-neutral-300 border-t-blue-600 dark:border-neutral-600 dark:border-t-blue-400" />
    </div>
  )
}

export default function App() {
  return (
    <ThemeProvider>
      <AccentColorProvider>
      <Suspense fallback={<LoadingSpinner />}>
        <Routes>
          {/* Public routes */}
          <Route path="/login" element={<LoginPage />} />
          <Route path="/callback" element={<CallbackPage />} />

          {/* Protected routes */}
          <Route element={<AuthGuard />}>
            <Route path="/" element={<Layout />}>
              <Route index element={<Navigate to="/dashboard" replace />} />
              <Route path="dashboard" element={<DashboardPage />} />
              <Route path="activities" element={<ActivitiesPage />} />
              <Route path="activities/:id" element={<ActivityDetailPage />} />
              <Route path="map" element={<MapPage />} />
              <Route path="calendar" element={<CalendarPage />} />
              <Route path="segments" element={<SegmentsPage />} />
              <Route path="workshop" element={<WorkshopPage />} />
              <Route path="workshop/reprocess" element={<WorkshopReprocessPage />} />
              <Route path="workshop/profile" element={<PhysiologicalProfilePage />} />
              <Route path="workshop/tags" element={<TagsPage />} />
              <Route path="gear" element={<GearPage />} />
              <Route path="gear/:id" element={<GearDetailPage />} />
              <Route path="settings" element={<SettingsPage />} />
              <Route path="settings/privacy" element={<PrivacySettingsPage />} />
              <Route path="settings/enrichment" element={<EnrichmentSettingsPage />} />
              <Route path="settings/rules" element={<AutomationRulesPage />} />
              <Route path="settings/tags" element={<TagsPage />} />
            </Route>
          </Route>
        </Routes>
      </Suspense>
      </AccentColorProvider>
    </ThemeProvider>
  )
}
