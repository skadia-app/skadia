import HRVAnalysisChart from '@/components/analytics/health/HRVAnalysis'
import SleepAnalysisChart from '@/components/analytics/health/SleepAnalysis'
import WeightChart from '@/components/analytics/health/WeightChart'
import HealthMetricsGrid from '@/components/analytics/health/HealthMetricsGrid'
import StressChart from '@/components/analytics/health/StressChart'
import SpO2Chart from '@/components/analytics/health/SpO2Chart'
import BloodPressureChart from '@/components/analytics/health/BloodPressureChart'
import FitnessAgeChart from '@/components/analytics/health/FitnessAgeChart'
import HRVCorrelationChart from '@/components/analytics/health/HRVCorrelationChart'
import GlucoseChart from '@/components/analytics/health/GlucoseChart'

// ── HealthTab ─────────────────────────────────────────────────────────────────

export default function HealthTab() {
  return (
    <div className="space-y-6">
      {/* HRV Analysis — full width */}
      <HRVAnalysisChart />

      {/* Sleep — full width */}
      <SleepAnalysisChart />

      {/* Weight — full width */}
      <WeightChart />

      {/* Stress — full width */}
      <StressChart />

      {/* SpO2 — full width */}
      <SpO2Chart />

      {/* Glucose — full width, only rendered when user has glucose data */}
      <GlucoseChart />

      {/* Blood Pressure — full width */}
      <BloodPressureChart />

      {/* Fitness Age — full width */}
      <FitnessAgeChart />

      {/* HRV Correlation — full width */}
      <HRVCorrelationChart />

      {/* Health Metrics Grid — full width */}
      <HealthMetricsGrid />
    </div>
  )
}
