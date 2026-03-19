import { lazy, Suspense, useEffect } from 'react'
import { Routes, Route, Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { apiClient } from '@/api/client'
import { ThemeProvider } from '@/lib/theme'
import { AccentColorProvider } from '@/lib/accentColor'
import { GlobalSportSettingsProvider } from '@/lib/globalSportSettings'
import { ToastProvider } from '@/lib/toast'
import { ModalProvider } from '@/lib/modal'
import Layout from '@/components/Layout'
import AuthGuard from '@/components/AuthGuard'
import ErrorBoundary from '@/components/ErrorBoundary'
import { ToastContainer } from '@/components/Toast'
import { ModalContainer } from '@/components/Modal'

// Lazy-loaded pages (route-based code splitting)
const DashboardPage = lazy(() => import('@/pages/DashboardPage'))
const PhotoGalleryPage = lazy(() => import('@/pages/PhotoGalleryPage'))
const ActivitiesPage = lazy(() => import('@/pages/ActivitiesPage'))
const ActivityDetailPage = lazy(() => import('@/pages/ActivityDetailPage'))
const MapPage = lazy(() => import('@/pages/MapPage'))
const SegmentsPage = lazy(() => import('@/pages/SegmentsPage'))
const SegmentDetailPage = lazy(() => import('@/pages/SegmentDetailPage'))
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
const DocumentReviewPage = lazy(() => import('@/pages/DocumentReviewPage'))
const PeakCollectionPage = lazy(() => import('@/pages/PeakCollectionPage'))
const ActivityEditorPage = lazy(() => import('@/pages/ActivityEditorPage'))
const AnalyticsPage = lazy(() => import('@/pages/AnalyticsPage'))
const TrainingTab = lazy(() => import('@/pages/analytics/TrainingTab'))
const HealthTab = lazy(() => import('@/pages/analytics/HealthTab'))
const StatsTab = lazy(() => import('@/pages/analytics/StatsTab'))
const InsightsTab = lazy(() => import('@/pages/analytics/InsightsTab'))
const LoginPage = lazy(() => import('@/pages/LoginPage'))
const CallbackPage = lazy(() => import('@/pages/CallbackPage'))
const ImportPage = lazy(() => import('@/pages/ImportPage'))
const ActivityComparisonPage = lazy(() => import('@/pages/ActivityComparisonPage'))
const FusionPage = lazy(() => import('@/pages/FusionPage'))
const ChatPage = lazy(() => import('@/pages/ChatPage'))
const FusionQueuePage = lazy(() => import('@/pages/FusionQueuePage'))
const RoutesPage = lazy(() => import('@/pages/RoutesPage'))
const ImportQueuePage = lazy(() => import('@/pages/ImportQueuePage'))
const ImportsPage = lazy(() => import('@/pages/ImportsPage'))
const DeviceListPage = lazy(() => import('@/pages/DeviceListPage'))
const DeviceDetailPage = lazy(() => import('@/pages/DeviceDetailPage'))

function ImportRedirect() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const url = searchParams.get('url')

  useEffect(() => {
    if (!url) {
      navigate('/')
      return
    }
    apiClient<{ activity?: { id: string } }>('/import/from-url', {
      method: 'POST',
      body: { url },
    })
      .then((result) => {
        if (result?.activity?.id) {
          navigate(`/activities/${result.activity.id}`)
        } else {
          navigate('/')
        }
      })
      .catch(() => {
        navigate('/')
      })
  }, [url, navigate])

  return (
    <div className="flex items-center justify-center h-64">
      <p className="text-neutral-500">Importiere Aktivität...</p>
    </div>
  )
}

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
      <ToastProvider>
        <ModalProvider>
          <AccentColorProvider>
            <GlobalSportSettingsProvider>
              <ErrorBoundary>
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
                        <Route path="photos" element={<PhotoGalleryPage />} />
                        <Route path="activities" element={<ActivitiesPage />} />
                        <Route path="activities/compare" element={<ActivityComparisonPage />} />
                        <Route path="fusion" element={<FusionPage />} />
                        <Route path="activities/:id" element={<ActivityDetailPage />} />
                        <Route path="activities/:id/edit" element={<ActivityEditorPage />} />
                        <Route path="map" element={<MapPage />} />
                        <Route path="calendar" element={<CalendarPage />} />
                        <Route path="health" element={<Navigate to="/analytics/health" replace />} />
                        <Route path="analytics" element={<AnalyticsPage />}>
                          <Route index element={<Navigate to="/analytics/training" replace />} />
                          <Route path="training" element={<TrainingTab />} />
                          <Route path="health" element={<HealthTab />} />
                          <Route path="stats" element={<StatsTab />} />
                          <Route path="insights" element={<InsightsTab />} />
                        </Route>
                        <Route path="chat" element={<ChatPage />} />
                        <Route path="routes" element={<RoutesPage />} />
                        <Route path="segments" element={<SegmentsPage />} />
                        <Route path="segments/:id" element={<SegmentDetailPage />} />
                        <Route path="workshop" element={<WorkshopPage />} />
                        <Route path="workshop/reprocess" element={<WorkshopReprocessPage />} />
                        <Route path="workshop/profile" element={<PhysiologicalProfilePage />} />
                        <Route path="workshop/tags" element={<TagsPage />} />
                        <Route path="gear" element={<GearPage />} />
                        <Route path="gear/:id" element={<GearDetailPage />} />
                        <Route path="hardware" element={<DeviceListPage />} />
                        <Route path="hardware/:type" element={<DeviceListPage />} />
                        <Route path="hardware/detail/:id" element={<DeviceDetailPage />} />
                        <Route path="settings" element={<SettingsPage />} />
                        <Route path="settings/privacy" element={<PrivacySettingsPage />} />
                        <Route path="settings/enrichment" element={<EnrichmentSettingsPage />} />
                        <Route path="workshop/rules" element={<AutomationRulesPage />} />
                        <Route path="workshop/document-review" element={<DocumentReviewPage />} />
                        <Route path="workshop/peaks" element={<PeakCollectionPage />} />
                        <Route path="workshop/fusion" element={<FusionQueuePage />} />
                        <Route path="workshop/import-queue" element={<ImportQueuePage />} />
                        <Route path="settings/rules" element={<Navigate to="/workshop/rules" replace />} />
                        <Route path="settings/tags" element={<TagsPage />} />
                        <Route path="settings/import" element={<ImportPage />} />
                        <Route path="import/url" element={<ImportRedirect />} />
                        <Route path="queue" element={<Navigate to="/settings/import?tab=queue" replace />} />
                        <Route path="imports" element={<ImportsPage />} />
                      </Route>
                    </Route>
                  </Routes>
                </Suspense>
              </ErrorBoundary>
            </GlobalSportSettingsProvider>
          </AccentColorProvider>
          <ToastContainer />
          <ModalContainer />
        </ModalProvider>
      </ToastProvider>
    </ThemeProvider>
  )
}
