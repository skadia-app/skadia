import type { DiveGas } from '@/api/types'

interface Props {
  gases: DiveGas[]
}

function gasLabel(gas: DiveGas): string {
  if (gas.helium_pct > 0) return `Trimix ${gas.oxygen_pct}/${gas.helium_pct}`
  if (gas.oxygen_pct === 21) return 'Luft'
  return `EANx ${gas.oxygen_pct}`
}

function modeLabel(mode: string): string {
  const lower = mode.toLowerCase()
  if (lower === 'cc_diluent' || lower === 'closed_circuit_diluent') return 'Diluent (CCR)'
  if (lower === 'cc_o2') return 'O₂ (CCR)'
  if (lower === 'oc_gas' || lower === 'open_circuit') return 'OC'
  return mode
}

function statusLabel(status: string): string {
  const lower = status.toLowerCase()
  if (lower === 'enabled') return 'Aktiv'
  if (lower === 'backup') return 'Backup'
  if (lower === 'disabled') return 'Inaktiv'
  if (lower === 'bailout') return 'Bailout'
  return status
}

export function DiveGasTable({ gases }: Props) {
  if (gases.length === 0) return null

  const hasHelium = gases.some(g => g.helium_pct > 0)

  return (
    <div>
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-2">Atemgase</h3>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-xs text-neutral-500 dark:text-neutral-400 border-b border-neutral-200 dark:border-neutral-700">
              <th className="text-left py-1 pr-4 font-medium">Gas</th>
              <th className="text-right py-1 pr-4 font-medium tabular-nums">O₂ %</th>
              {hasHelium && (
                <th className="text-right py-1 pr-4 font-medium tabular-nums">He %</th>
              )}
              <th className="text-left py-1 pr-4 font-medium">Modus</th>
              <th className="text-left py-1 font-medium">Status</th>
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
                <td className="py-1.5 pr-4 font-medium">{gasLabel(gas)}</td>
                <td className="py-1.5 pr-4 text-right tabular-nums">{gas.oxygen_pct}</td>
                {hasHelium && (
                  <td className="py-1.5 pr-4 text-right tabular-nums">
                    {gas.helium_pct > 0 ? gas.helium_pct : '—'}
                  </td>
                )}
                <td className="py-1.5 pr-4">{modeLabel(gas.mode)}</td>
                <td className="py-1.5">{statusLabel(gas.status)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
