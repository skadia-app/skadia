import { Routes, Route, Navigate } from 'react-router-dom'
import { ThemeProvider } from '@/lib/theme'
import Layout from '@/components/Layout'
import ActivitiesPage from '@/pages/ActivitiesPage'

export default function App() {
  return (
    <ThemeProvider>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Navigate to="/activities" replace />} />
          <Route path="activities" element={<ActivitiesPage />} />
        </Route>
      </Routes>
    </ThemeProvider>
  )
}
