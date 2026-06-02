import { useTranslation } from 'react-i18next'
import type { DiveGas } from '@/api/types'

interface Props {
  gases: DiveGas[]
}

function gasLabel(gas: DiveGas, t: (key: string) => string): string {
  if (gas.helium_pct > 0) return `Trimix ${gas.oxygen_pct}/${gas.helium_pct}`
  if (gas.oxygen_pct === 21) return t('activityDetail.dive.air')
  return `EANx ${gas.oxygen_pct}`
}

function modeLabel(mode: string, t: (key: string) => string): string {
  if (mode === 'closed_circuit_diluent') return t('activityTables.diveGas.modes.closedCircuitDiluent')
  if (mode === 'open_circuit') return 'OC'
  return mode
}

function statusLabel(status: string, t: (key: string) => string): string {
  const lower = status.toLowerCase()
  if (lower === 'enabled') return t('activityDetail.dive.gasStatus.enabled')
  if (lower === 'backup') return 'Backup'
  if (lower === 'disabled') return t('activityDetail.dive.gasStatus.disabled')
  if (lower === 'bailout') return 'Bailout'
  return status
}

export function DiveGasTable({ gases }: Props) {
  const { t } = useTranslation()
  if (gases.length === 0) return null

  const hasHelium = gases.some(g => g.helium_pct > 0)

  return (
    <div>
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-2">{t('activityDetail.dive.breathingGases')}</h3>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-xs text-neutral-500 dark:text-neutral-400 border-b border-neutral-200 dark:border-neutral-700">
              <th className="text-left py-1 pr-4 font-medium">{t('activityDetail.dive.gas')}</th>
              <th className="text-right py-1 pr-4 font-medium tabular-nums">O₂ %</th>
              {hasHelium && (
                <th className="text-right py-1 pr-4 font-medium tabular-nums">He %</th>
              )}
              <th className="text-left py-1 pr-4 font-medium">{t('activityDetail.dive.mode')}</th>
              <th className="text-left py-1 font-medium">{t('activityDetail.dive.status')}</th>
            </tr>
          </thead>
          <tbody>
            {gases.map(gas => (
              <tr
                key={gas.gas_index}
                className={[
                  'border-b border-neutral-100 dark:border-neutral-800',
                  gas.mode.toLowerCase().includes('diluent')
                    ? 'text-blue-600 dark:text-blue-400'
                    : gas.status.toLowerCase() === 'bailout'
                      ? 'text-amber-600 dark:text-amber-400'
                      : '',
                ].join(' ')}
              >
                <td className="py-1.5 pr-4 font-medium">{gasLabel(gas, t)}</td>
                <td className="py-1.5 pr-4 text-right tabular-nums">{gas.oxygen_pct}</td>
                {hasHelium && (
                  <td className="py-1.5 pr-4 text-right tabular-nums">
                    {gas.helium_pct > 0 ? gas.helium_pct : '—'}
                  </td>
                )}
                <td className="py-1.5 pr-4">{modeLabel(gas.mode, t)}</td>
                <td className="py-1.5">{statusLabel(gas.status, t)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
