import { Routes, Route, Navigate } from 'react-router-dom'
import { ThemeProvider } from '@/lib/theme'
import Layout from '@/components/Layout'
import AuthGuard from '@/components/AuthGuard'
import ActivitiesPage from '@/pages/ActivitiesPage'
import ActivityDetailPage from '@/pages/ActivityDetailPage'
import DashboardPage from '@/pages/DashboardPage'
import HeatmapPage from '@/pages/HeatmapPage'
import SettingsPage from '@/pages/SettingsPage'
import PrivacySettingsPage from '@/pages/PrivacySettingsPage'
import LoginPage from '@/pages/LoginPage'
import CallbackPage from '@/pages/CallbackPage'

export default function App() {
  return (
    <ThemeProvider>
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
            <Route path="heatmap" element={<HeatmapPage />} />
            <Route path="settings" element={<SettingsPage />} />
            <Route path="settings/privacy" element={<PrivacySettingsPage />} />
          </Route>
        </Route>
      </Routes>
    </ThemeProvider>
  )
}
