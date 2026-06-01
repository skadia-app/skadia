import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import '@testing-library/jest-dom/vitest'
import { beforeEach, describe, it, expect, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import GearDetailPage from './GearDetailPage'
import type { Gear, GearComponent, ChainRotationGroup, BikeSetup, ComponentTypePreset } from '@/api/types'
import {
  createServiceInterval,
  createChainRotation,
  fetchGear,
  fetchGearDetail,
  updateComponent,
  listServiceEvents,
  listServiceIntervals,
  getChainRotation,
  updateChainRotation,
  listBikeSetups,
  createBikeSetup,
  activateBikeSetup,
  fetchComponentPresets,
} from '@/api/gear'

const mockUserPrefs = vi.hoisted((): { locale: string; units: 'metric' | 'imperial' } => ({
  locale: 'de',
  units: 'metric',
}))

const MOCK_COMPONENT: GearComponent = {
  id: 'c1',
  gear_id: 'g1',
  parent_component_id: null,
  name: 'Testkette',
  type: 'chain',
  brand: 'TestBrand',
  total_distance_m: 500,
  total_elevation_gain_m: 50,
  total_elevation_loss_m: 0,
  total_moving_time_s: 1800,
  total_time_s: 1800,
  total_activities: 0,
  is_retired: false,
  status: 'active',
}

const MOCK_GEAR: Gear = {
  id: 'g1',
  user_id: 'u1',
  name: 'Testrad',
  type: 'bike',
  total_distance_m: 1000,
  total_elevation_gain_m: 100,
  total_elevation_loss_m: 0,
  total_time_s: 3600,
  total_moving_time_s: 3600,
  total_activities: 5,
  is_retired: false,
  status: 'active',
  applicable_sport_predicates: [],
  created_at: '2024-01-01T00:00:00Z',
  components: [MOCK_COMPONENT],
}

vi.mock('@/api/gear', () => ({
  fetchGearDetail: vi.fn(() => Promise.resolve(MOCK_GEAR)),
  updateGear: vi.fn(),
  updateGearStatus: vi.fn(),
  createComponent: vi.fn(),
  updateComponent: vi.fn(),
  deleteComponent: vi.fn(),
  transferComponent: vi.fn(),
  getComponentHistory: vi.fn(() => Promise.resolve([])),
  fetchGear: vi.fn(() => Promise.resolve([])),
  getChainRotation: vi.fn(() => Promise.resolve(null)),
  createChainRotation: vi.fn(),
  updateChainRotation: vi.fn(() => Promise.resolve()),
  swapChain: vi.fn(),
  deleteChainRotation: vi.fn(),
  listServiceEvents: vi.fn(() => Promise.resolve([])),
  createServiceEvent: vi.fn(),
  deleteServiceEvent: vi.fn(),
  listServiceIntervals: vi.fn(() => Promise.resolve([])),
  createServiceInterval: vi.fn(),
  deleteServiceInterval: vi.fn(),
  fetchComponentPresets: vi.fn(() => Promise.resolve([])),
  listBikeSetups: vi.fn(() => Promise.resolve([])),
  createBikeSetup: vi.fn(),
  updateBikeSetup: vi.fn(),
  deleteBikeSetup: vi.fn(),
  activateBikeSetup: vi.fn(),
  fetchGearDocuments: vi.fn(() => Promise.resolve([])),
  uploadGearDocument: vi.fn(),
  deleteGearDocument: vi.fn(),
  downloadGearDocumentBlob: vi.fn(),
  uploadComponentDocument: vi.fn(),
  listComponentDocuments: vi.fn(() => Promise.resolve([])),
  getGearCosts: vi.fn(() => Promise.resolve({ total_cost: 0, currency: 'EUR' })),
  getGearDocumentUrl: vi.fn(() => ''),
  getComponentDocumentUrl: vi.fn(() => ''),
}))

vi.mock('@/api/hardware', () => ({
  fetchDevices: vi.fn(() => Promise.resolve([])),
}))

vi.mock('@/lib/modal', () => ({
  useModal: vi.fn(() => ({ open: vi.fn(), close: vi.fn() })),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

vi.mock('@/components/GearDocumentSection', () => ({ default: () => null }))
vi.mock('@/components/GearForm', () => ({ default: () => null }))

function renderPage(initialPath = '/gear/g1') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return {
    client,
    ...render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={[initialPath]}>
          <Routes>
            <Route path="/gear/:id" element={<GearDetailPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    ),
  }
}

function componentPresetFixture(overrides: Partial<ComponentTypePreset> = {}): ComponentTypePreset {
  return {
    id: 'preset-1',
    name: 'Kette',
    type: 'Kette',
    category: 'drivetrain',
    gear_type: 'bike',
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  mockUserPrefs.locale = 'de'
  mockUserPrefs.units = 'metric'
  vi.mocked(fetchGearDetail).mockResolvedValue(MOCK_GEAR)
  vi.mocked(fetchGear).mockResolvedValue([])
  vi.mocked(listServiceEvents).mockResolvedValue([])
  vi.mocked(listServiceIntervals).mockResolvedValue([])
  vi.mocked(getChainRotation).mockResolvedValue(null)
  vi.mocked(listBikeSetups).mockResolvedValue([])
  vi.mocked(fetchComponentPresets).mockResolvedValue([])
})

describe('GearDetailPage tabs', () => {
  beforeEach(() => {
    mockUserPrefs.locale = 'de'
    mockUserPrefs.units = 'metric'
    vi.mocked(fetchGearDetail).mockResolvedValue(MOCK_GEAR)
    vi.mocked(fetchGear).mockResolvedValue([])
    vi.mocked(listServiceEvents).mockResolvedValue([])
    vi.mocked(listServiceIntervals).mockResolvedValue([])
    vi.mocked(getChainRotation).mockResolvedValue(null)
    vi.mocked(listBikeSetups).mockResolvedValue([])
    vi.mocked(fetchComponentPresets).mockResolvedValue([])
  })

  it('TestDefaultTabIsStammdatenWithNoURLParam', async () => {
    renderPage('/gear/g1')
    // The tab bar must be rendered with the default tab active.
    const stammdaten = await screen.findByRole('tab', { name: 'Stammdaten' })
    expect(stammdaten).toBeInTheDocument()
    expect(stammdaten).toHaveAttribute('data-state', 'active')
  })

  it('TestURLParamSportSelectsSportTab', async () => {
    renderPage('/gear/g1?tab=sports')
    const sportTab = await screen.findByRole('tab', { name: 'Sportarten' })
    expect(sportTab).toHaveAttribute('data-state', 'active')
  })

  it('TestSwitchingTabUpdatesURLParam', async () => {
    const user = userEvent.setup()
    renderPage('/gear/g1')
    const sportTab = await screen.findByRole('tab', { name: 'Sportarten' })
    // Before clicking, the sports tab is not active.
    expect(sportTab).not.toHaveAttribute('data-state', 'active')

    await user.click(sportTab)

    // After clicking, the sports tab becomes active.
    await waitFor(() => {
      expect(screen.getByRole('tab', { name: 'Sportarten' })).toHaveAttribute('data-state', 'active')
    })
    // Stammdaten is no longer active.
    expect(screen.getByRole('tab', { name: 'Stammdaten' })).not.toHaveAttribute('data-state', 'active')
  })

  it('TestComponentsTabShowsEditableListWithoutFlatHierarchy', async () => {
    // Flat component lists should not be duplicated by the read-only hierarchy.
    // The editable CRUD list is the primary surface.
    renderPage('/gear/g1?tab=components')

    expect(await screen.findByTestId('components-crud')).toBeInTheDocument()

    expect(screen.getAllByTestId('components-crud')).toHaveLength(1)
    expect(screen.queryByTestId('components-hierarchy')).not.toBeInTheDocument()
  })

  it('TestComponentsTabShowsEditableComponentsWithoutChainRotationOrSetups', async () => {
    renderPage('/gear/g1?tab=components')

    expect(await screen.findByRole('heading', { name: 'Komponenten' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Kettenrotation' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Setups' })).not.toBeInTheDocument()
  })

  it('uses a generic label for unknown component status values', async () => {
    vi.mocked(fetchGearDetail).mockResolvedValue({
      ...MOCK_GEAR,
      components: [{
        ...MOCK_COMPONENT,
        status: 'backend_private_component_status' as GearComponent['status'],
      }],
    })

    renderPage('/gear/g1?tab=components')

    expect(await screen.findByText('Testkette')).toBeInTheDocument()
    expect(screen.getByText('Unbekannter Status')).toBeInTheDocument()
    expect(screen.queryByText('backend_private_component_status')).not.toBeInTheDocument()
  })

  it('TestWearTabShowsChainRotation', async () => {
    renderPage('/gear/g1?tab=wear')

    expect(await screen.findByRole('heading', { name: 'Kettenrotation' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Gesamtnutzung' })).toBeInTheDocument()
  })

  it('TestComponentsTabShowsHierarchyAfterEditableListOnlyForNestedComponents', async () => {
    const childComponent: GearComponent = {
      ...MOCK_COMPONENT,
      id: 'c-child',
      parent_component_id: 'c1',
      name: 'Kettenblatt',
      type: 'bike.chainring',
    }
    vi.mocked(fetchGearDetail).mockResolvedValue({
      ...MOCK_GEAR,
      components: [MOCK_COMPONENT, childComponent],
    })

    renderPage('/gear/g1?tab=components')

    const crudSection = await screen.findByTestId('components-crud')
    const hierarchySection = screen.getByTestId('components-hierarchy')

    expect(screen.getAllByTestId('components-hierarchy')).toHaveLength(1)
    expect(screen.getAllByTestId('components-crud')).toHaveLength(1)
    expect(
      Boolean(crudSection.compareDocumentPosition(hierarchySection) & Node.DOCUMENT_POSITION_FOLLOWING),
    ).toBe(true)
  })

  it('TestSetupsUrlFallsBackToComponentsForNonBikeGear', async () => {
    vi.mocked(fetchGearDetail).mockResolvedValue({
      ...MOCK_GEAR,
      type: 'shoes.running',
      scope: 'shoes',
      components: [],
    })

    renderPage('/gear/g1?tab=setups')

    expect(await screen.findByTestId('components-crud')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Komponenten' })).toHaveAttribute('data-state', 'active')
    expect(screen.queryByRole('tab', { name: 'Setups' })).not.toBeInTheDocument()
  })

  it('TestComponentEditViaCRUDUpdatesEditableList', async () => {
    // Regression anchor: ComponentsList reads from gear.components. After an
    // edit triggers invalidateQueries, the refetch must update the visible row.
    const user = userEvent.setup()

    const renamedComponent: GearComponent = { ...MOCK_COMPONENT, name: 'Neue Kette' }
    const updatedGear: Gear = { ...MOCK_GEAR, components: [renamedComponent] }

    // First call returns original data; second call (after invalidation) returns updated.
    vi.mocked(fetchGearDetail)
      .mockResolvedValueOnce(MOCK_GEAR)
      .mockResolvedValueOnce(updatedGear)
    vi.mocked(updateComponent).mockResolvedValueOnce(renamedComponent as any)

    renderPage('/gear/g1?tab=components')

    // Wait for the editable list to show original component name.
    await waitFor(() => {
      expect(screen.getByTestId('components-crud')).toHaveTextContent('Testkette')
    })

    // Click "Bearbeiten" in the CRUD section to open the edit form.
    // Scope to the CRUD section to avoid ambiguity with other edit controls.
    const crudSection = screen.getByTestId('components-crud')
    const editButton = within(crudSection).getByRole('button', { name: 'Bearbeiten' })
    await user.click(editButton)

    // Clear the name field and type new name.
    const nameInput = screen.getByPlaceholderText('Name')
    await user.clear(nameInput)
    await user.type(nameInput, 'Neue Kette')

    // Submit the form.
    const saveButton = screen.getByRole('button', { name: 'Speichern' })
    await user.click(saveButton)

    // After refetch, the editable list must reflect the new name.
    await waitFor(() => {
      expect(screen.getByTestId('components-crud')).toHaveTextContent('Neue Kette')
    })
  })

  it('TestDetailStatsUseUserUnitPreferenceForDistanceAndElevation', async () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    vi.mocked(fetchGearDetail).mockResolvedValue({
      ...MOCK_GEAR,
      total_distance_m: 3218.688,
      total_elevation_gain_m: 100,
      total_elevation_loss_m: 50,
      components: [],
    })

    renderPage('/gear/g1')

    expect(await screen.findByText('Testrad')).toBeInTheDocument()
    expect(screen.getByText('2.0 mi')).toBeInTheDocument()
    expect(screen.getByText('+328 ft')).toBeInTheDocument()
    expect(screen.getByText('−164 ft')).toBeInTheDocument()
    expect(screen.queryByText('3.22 km')).not.toBeInTheDocument()
    expect(screen.queryByText('+100 m')).not.toBeInTheDocument()
  })

  it('TestComponentSummaryUsesUserUnitPreferenceForDistanceAndElevation', async () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    vi.mocked(fetchGearDetail).mockResolvedValue({
      ...MOCK_GEAR,
      components: [{
        ...MOCK_COMPONENT,
        total_distance_m: 1609.344,
        total_elevation_gain_m: 25,
        total_elevation_loss_m: 10,
      }],
    })

    renderPage('/gear/g1?tab=components')

    const crudSection = await screen.findByTestId('components-crud')
    expect(within(crudSection).getByText('1.0 mi')).toBeInTheDocument()
    expect(within(crudSection).getByText('+82 ft')).toBeInTheDocument()
    expect(within(crudSection).getByText('−33 ft')).toBeInTheDocument()
    expect(within(crudSection).queryByText('1 km')).not.toBeInTheDocument()
    expect(within(crudSection).queryByText('+25 m')).not.toBeInTheDocument()
  })

  it('TestComponentFormFiltersGearTypePresetsAndUsesNeutralPlaceholders', async () => {
    const user = userEvent.setup()
    const shoeGear: Gear = {
      ...MOCK_GEAR,
      name: 'ASICS GEL-Kayano 2011',
      type: 'shoes.running',
      scope: 'shoes',
      components: [],
    }
    vi.mocked(fetchGearDetail).mockResolvedValue(shoeGear)
    vi.mocked(fetchComponentPresets).mockResolvedValue([
      componentPresetFixture({ id: 'bad', name: 'Laufschuhe', category: 'schuhe', gear_type: 'shoes' }),
    ])

    renderPage('/gear/g1?tab=components')

    const crudSection = await screen.findByTestId('components-crud')
    await user.click(within(crudSection).getByRole('button', { name: 'Hinzufügen' }))

    await waitFor(() => {
      expect(fetchComponentPresets).toHaveBeenCalledWith('shoes.running')
    })
    expect(within(crudSection).queryByText('Vorlage')).not.toBeInTheDocument()
    expect(within(crudSection).getByPlaceholderText('Typ *')).toBeInTheDocument()
    expect(within(crudSection).queryByPlaceholderText(/Kette|Reifen/)).not.toBeInTheDocument()
  })
})

describe('GearDetailPage i18n fallbacks', () => {
  it('does not ship gearPages defaultValue fallbacks from GearDetailPage', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/pages/GearDetailPage.tsx'), 'utf8')
    const fallbackMatches = source
      .split('\n')
      .filter(line => line.includes("t('gearPages.") && line.includes('defaultValue'))

    expect(fallbackMatches).toEqual([])
  })
})

describe('ServiceEventsPanel status gate', () => {
  const RETIRED_COMPONENT: GearComponent = {
    ...MOCK_COMPONENT,
    id: 'c2',
    name: 'Alte Kette',
    status: 'retired',
    is_retired: true,
  }

  it('TestServiceButtonsHiddenForRetiredComponent', async () => {
    const user = userEvent.setup()

    const gearWithBoth: Gear = { ...MOCK_GEAR, components: [MOCK_COMPONENT, RETIRED_COMPONENT] }
    vi.mocked(fetchGearDetail).mockResolvedValue(gearWithBoth)
    vi.mocked(listServiceEvents).mockResolvedValue([])
    vi.mocked(listServiceIntervals).mockResolvedValue([])

    renderPage('/gear/g1?tab=components')

    const crudSection = await screen.findByTestId('components-crud')

    // Open service panel for the active component
    const serviceButtons = within(crudSection).getAllByRole('button', { name: 'Service' })
    expect(serviceButtons).toHaveLength(2)

    // First button belongs to the active component (c1 rendered first)
    await user.click(serviceButtons[0])
    await waitFor(() => {
      expect(within(crudSection).getByRole('button', { name: '+ Intervall' })).toBeInTheDocument()
      expect(within(crudSection).getByRole('button', { name: '+ Service' })).toBeInTheDocument()
    })

    // Close active panel, open retired component panel
    await user.click(serviceButtons[0])
    await user.click(serviceButtons[1])

    // Give panel time to mount
    await waitFor(() => {
      // Read-only content renders (the "Service-Historie" heading is always shown)
      expect(within(crudSection).getByText('Service-Historie')).toBeInTheDocument()
    })

    // Create buttons must NOT be present for the retired component
    expect(within(crudSection).queryByRole('button', { name: '+ Intervall' })).not.toBeInTheDocument()
    expect(within(crudSection).queryByRole('button', { name: '+ Service' })).not.toBeInTheDocument()
  })

  it('TestServiceIntervalValueUsesVisibleDisplayUnits', async () => {
    const user = userEvent.setup()
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    vi.mocked(fetchGearDetail).mockResolvedValue(MOCK_GEAR)
    vi.mocked(listServiceEvents).mockResolvedValue([])
    vi.mocked(listServiceIntervals).mockResolvedValue([])

    renderPage('/gear/g1?tab=components')

    const crudSection = await screen.findByTestId('components-crud')
    await user.click(within(crudSection).getByRole('button', { name: 'Service' }))
    await user.click(within(crudSection).getByRole('button', { name: '+ Intervall' }))

    expect(within(crudSection).getByText('Wert (h)')).toBeInTheDocument()
    await user.selectOptions(within(crudSection).getByLabelText('Metrik'), 'distance_m')
    expect(within(crudSection).getByText('Wert (mi)')).toBeInTheDocument()

    await user.type(within(crudSection).getByLabelText('Wert (mi)'), '1')
    await user.click(within(crudSection).getByRole('button', { name: 'Speichern' }))

    await waitFor(() => {
      expect(createServiceInterval).toHaveBeenCalledWith('c1', expect.objectContaining({
        metric_key: 'distance_m',
        interval_value: 1609.344,
      }))
    })
  })
})

describe('Component transfer', () => {
  it('TestTransferTargetsAreFilteredToCompatibleGearScope', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchGearDetail).mockResolvedValue(MOCK_GEAR)
    vi.mocked(fetchGear).mockResolvedValue([
      { ...MOCK_GEAR, id: 'bike-2', name: 'Zielrad', type: 'bike.mtb.enduro', scope: 'bike' },
      { ...MOCK_GEAR, id: 'legacy-ebike', name: 'Legacy E-Bike', type: 'e_bike', scope: undefined },
      { ...MOCK_GEAR, id: 'shoe-1', name: 'Laufschuh', type: 'shoes.running', scope: 'shoes' },
    ])

    renderPage('/gear/g1?tab=components')

    const crudSection = await screen.findByTestId('components-crud')
    await user.click(within(crudSection).getByRole('button', { name: 'Transfer' }))

    const select = within(crudSection).getByDisplayValue('Zielausrüstung wählen...')
    expect(within(select).getByRole('option', { name: 'Zielrad (Fahrrad / Mountainbike / Enduro)' })).toBeInTheDocument()
    expect(within(select).getByRole('option', { name: 'Legacy E-Bike (E-Bike)' })).toBeInTheDocument()
    expect(within(select).queryByRole('option', { name: 'Laufschuh (Schuhe / Laufschuhe)' })).not.toBeInTheDocument()
  })
})

describe('ChainRotation edit form', () => {
  const MOCK_ROTATION: ChainRotationGroup = {
    id: 'rot1',
    gear_id: 'g1',
    name: 'Kettenrotation',
    swap_interval_metric_key: 'distance_m',
    swap_interval_value: 500000,
    members: [],
    created_at: '2024-01-01T00:00:00Z',
  }

  const UPDATED_ROTATION: ChainRotationGroup = {
    ...MOCK_ROTATION,
    swap_interval_metric_key: 'elevation_gain_m',
    swap_interval_value: 5000,
  }

  it('TestChainRotationEditMetricRoundTrip', async () => {
    const user = userEvent.setup()

    vi.mocked(fetchGearDetail).mockResolvedValue(MOCK_GEAR)
    // First call returns original rotation; after updateChainRotation + invalidation, second call returns updated.
    vi.mocked(getChainRotation)
      .mockResolvedValueOnce(MOCK_ROTATION)
      .mockResolvedValueOnce(UPDATED_ROTATION)
    vi.mocked(updateChainRotation).mockResolvedValue(undefined as any)

    renderPage('/gear/g1?tab=wear')

    // Wait for the chain rotation section to display the initial swap interval.
    await waitFor(() => {
      expect(screen.getByText(/Wechsel alle/)).toBeInTheDocument()
    })
    // Initial display shows 500 km (500000 m / 1000).
    expect(screen.getByText(/Wechsel alle 500/)).toBeInTheDocument()

    // Open the edit form via the pencil button.
    const editBtn = screen.getByRole('button', { name: 'Rotationsintervall bearbeiten' })
    await user.click(editBtn)

    // The edit form must be visible with a Metrik select and a value input.
    const metrikSelect = screen.getByRole('combobox', { name: /Metrik/i })
    // Change metric from distance to elevation_gain_m.
    await user.selectOptions(metrikSelect, 'elevation_gain_m')

    // Clear the interval value and enter 5000 (metres of climb).
    const valueInput = screen.getByRole('spinbutton')
    await user.clear(valueInput)
    await user.type(valueInput, '5000')

    // Submit.
    const saveButton = screen.getByRole('button', { name: 'Speichern' })
    await user.click(saveButton)

    // updateChainRotation must have been called with the correct base-unit values.
    // name is always included (R2-2: backend accepts and trims the name field).
    expect(updateChainRotation).toHaveBeenCalledWith('rot1', {
      name: 'Kettenrotation',
      swap_interval_metric_key: 'elevation_gain_m',
      swap_interval_value: 5000,
    })

    // After the mutation succeeds and the query refetches, the UI must display
    // the new metric (elevation_gain_m shows "5000 m").
    await waitFor(() => {
      expect(screen.getByText(/Wechsel alle 5\.000 m/)).toBeInTheDocument()
    })
  })

  it('TestChainRotationCreateUsesVisibleDisplayUnits', async () => {
    const user = userEvent.setup()
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    const secondChain: GearComponent = { ...MOCK_COMPONENT, id: 'c2', name: 'Reserve chain' }
    vi.mocked(fetchGearDetail).mockResolvedValue({ ...MOCK_GEAR, components: [MOCK_COMPONENT, secondChain] })
    vi.mocked(getChainRotation).mockResolvedValue(null)
    vi.mocked(createChainRotation).mockResolvedValue({} as ChainRotationGroup)

    renderPage('/gear/g1?tab=wear')

    await user.click(await screen.findByRole('button', { name: 'Rotation anlegen' }))
    expect(screen.getByText('Wechselintervall (mi)')).toBeInTheDocument()

    const intervalInput = screen.getByRole('spinbutton')
    await user.clear(intervalInput)
    await user.type(intervalInput, '1')
    await user.click(screen.getByRole('checkbox', { name: /Testkette/ }))
    await user.click(screen.getByRole('checkbox', { name: /Reserve chain/ }))
    await user.click(screen.getByRole('button', { name: 'Rotation anlegen' }))

    await waitFor(() => {
      expect(createChainRotation).toHaveBeenCalledWith('g1', expect.objectContaining({
        swap_interval_metric_key: 'distance_m',
        swap_interval_value: 1609.344,
        component_ids: ['c1', 'c2'],
      }))
    })
  })

  it('TestChainRotationEditNameSentWithSave', async () => {
    // R2-2: editing the rotation name must include the new name in the PATCH payload.
    const user = userEvent.setup()

    const rotationWithNewName: ChainRotationGroup = { ...MOCK_ROTATION, name: 'Neue Bezeichnung' }
    vi.mocked(fetchGearDetail).mockResolvedValue(MOCK_GEAR)
    vi.mocked(getChainRotation)
      .mockResolvedValueOnce(MOCK_ROTATION)
      .mockResolvedValueOnce(rotationWithNewName)
    vi.mocked(updateChainRotation).mockResolvedValue(undefined as any)

    renderPage('/gear/g1?tab=wear')

    await waitFor(() => {
      expect(screen.getByText(/Wechsel alle/)).toBeInTheDocument()
    })

    // Open edit form.
    const editBtn = screen.getByRole('button', { name: 'Rotationsintervall bearbeiten' })
    await user.click(editBtn)

    // Find the Bezeichnung input and change the name.
    const nameInput = screen.getByDisplayValue('Kettenrotation')
    await user.clear(nameInput)
    await user.type(nameInput, 'Neue Bezeichnung')

    // Submit.
    const saveButton = screen.getByRole('button', { name: 'Speichern' })
    await user.click(saveButton)

    // The new name must appear in the PATCH payload.
    expect(updateChainRotation).toHaveBeenCalledWith('rot1', expect.objectContaining({
      name: 'Neue Bezeichnung',
    }))

    // UI must reflect the new name after refetch.
    await waitFor(() => {
      expect(screen.getByText('Neue Bezeichnung')).toBeInTheDocument()
    })
  })

  it('TestChainRotationSaveDisabledWhenNameEmpty', async () => {
    // R2-2: Save button must be disabled when the rotation name is cleared.
    const user = userEvent.setup()

    vi.mocked(fetchGearDetail).mockResolvedValue(MOCK_GEAR)
    vi.mocked(getChainRotation).mockResolvedValue(MOCK_ROTATION)
    vi.mocked(updateChainRotation).mockResolvedValue(undefined as any)

    renderPage('/gear/g1?tab=wear')

    await waitFor(() => {
      expect(screen.getByText(/Wechsel alle/)).toBeInTheDocument()
    })

    const editBtn = screen.getByRole('button', { name: 'Rotationsintervall bearbeiten' })
    await user.click(editBtn)

    // Clear the name field.
    const nameInput = screen.getByDisplayValue('Kettenrotation')
    await user.clear(nameInput)

    // Save button must be disabled.
    const saveButton = screen.getByRole('button', { name: 'Speichern' })
    expect(saveButton).toBeDisabled()

    expect(updateChainRotation).not.toHaveBeenCalled()
  })
})

// --- BikeSetups Dropdown-Header tests ---

const MOCK_SETUP_1: BikeSetup = {
  id: 's1',
  gear_id: 'g1',
  name: 'Trail Setup',
  is_active: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

const MOCK_SETUP_2: BikeSetup = {
  id: 's2',
  gear_id: 'g1',
  name: 'Bikepark Setup',
  is_active: false,
  created_at: '2024-01-02T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
}

const MOCK_SETUP_WITH_SNAPSHOT: BikeSetup = {
  id: 's3',
  gear_id: 'g1',
  name: 'Setup mit Snapshot',
  is_active: false,
  component_snapshot: {
    version: 1,
    captured_at: '2024-01-03T00:00:00Z',
    components: [
      { id: 'c1', name: 'Testkette', type: 'chain', status: 'active', brand: 'TestBrand', model: null },
      { id: 'c2', name: 'Federgabel', type: 'fork', status: 'active', brand: null, model: null },
    ],
  },
  created_at: '2024-01-03T00:00:00Z',
  updated_at: '2024-01-03T00:00:00Z',
}

describe('BikeSetupsSection — Setup tab', () => {
  it('TestBikeSetupsRenderForScopedBikeLeafType', async () => {
    vi.mocked(fetchGearDetail).mockResolvedValue({
      ...MOCK_GEAR,
      type: 'bike.mtb.enduro',
      scope: 'bike',
    })
    vi.mocked(listBikeSetups).mockResolvedValue([])

    renderPage('/gear/g1?tab=setups')

    expect(await screen.findByRole('heading', { name: 'Setups' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Setups' })).toHaveAttribute('data-state', 'active')
    expect(screen.getByText('Noch keine Setups vorhanden.')).toBeInTheDocument()
  })

  it('TestBikeSetupDropdownSwitchesSelectedSetup', async () => {
    // Render with 2 setups; select the inactive one via the dropdown.
    const user = userEvent.setup()
    vi.mocked(listBikeSetups).mockResolvedValue([MOCK_SETUP_1, MOCK_SETUP_2])

    renderPage('/gear/g1?tab=setups')

    // Wait for the setup selector dropdown to be present.
    const dropdown = await screen.findByRole('combobox', { name: /Setup auswählen/i })
    expect(dropdown).toBeInTheDocument()

    // Initially the active setup (s1 / Trail Setup) is shown.
    expect((dropdown as HTMLSelectElement).value).toBe('s1')

    // Switch to Bikepark Setup.
    await user.selectOptions(dropdown, 's2')

    // Detail panel must now show the second setup's name.
    // Use getAllByText because the name appears both in the <option> and the detail span.
    await waitFor(() => {
      const matches = screen.getAllByText('Bikepark Setup')
      expect(matches.length).toBeGreaterThanOrEqual(1)
    })
  })

  it('TestBikeSetupCaptureComponentsFlagSent', async () => {
    // Open "+ Neu", fill in a name, check "Aktuelle Komponenten mitschreiben", submit.
    const user = userEvent.setup()
    const newSetup: BikeSetup = { ...MOCK_SETUP_1, id: 's10', name: 'Neues Setup', is_active: false }
    vi.mocked(listBikeSetups)
      .mockResolvedValueOnce([MOCK_SETUP_1])
      .mockResolvedValueOnce([MOCK_SETUP_1, newSetup])
    vi.mocked(createBikeSetup).mockResolvedValue(newSetup)

    renderPage('/gear/g1?tab=setups')

    // Wait for initial render.
    await screen.findByRole('combobox', { name: /Setup auswählen/i })

    // Click "+ Neu".
    const neuBtn = screen.getByRole('button', { name: '+ Neu' })
    await user.click(neuBtn)

    // Fill in the name field.
    const nameInput = await screen.findByPlaceholderText('z. B. Park-Setup')
    await user.type(nameInput, 'Neues Setup')

    // Check "Aktuelle Komponenten mitschreiben".
    const captureCheckbox = screen.getByRole('checkbox', { name: /Aktuelle Komponenten mitschreiben/i })
    await user.click(captureCheckbox)
    expect(captureCheckbox).toBeChecked()

    // Submit the form.
    const saveButtons = screen.getAllByRole('button', { name: 'Speichern' })
    await user.click(saveButtons[0])

    // createBikeSetup must have been called with capture_components: true.
    await waitFor(() => {
      expect(createBikeSetup).toHaveBeenCalledWith(
        'g1',
        expect.objectContaining({ capture_components: true }),
      )
    })
  })

  it('TestBikeSetupFormUsesBikeUnitsAndFlipchipPosition', async () => {
    const user = userEvent.setup()
    const newSetup: BikeSetup = { ...MOCK_SETUP_1, id: 's11', name: 'Unit Setup', is_active: false }
    vi.mocked(listBikeSetups).mockResolvedValue([MOCK_SETUP_1])
    vi.mocked(createBikeSetup).mockResolvedValue(newSetup)

    renderPage('/gear/g1?tab=setups')

    await screen.findByRole('combobox', { name: /Setup auswählen/i })
    await user.click(screen.getByRole('button', { name: '+ Neu' }))

    await user.type(await screen.findByPlaceholderText('z. B. Park-Setup'), 'Unit Setup')
    await user.click(screen.getByRole('button', { name: 'Hintere Federung' }))
    await user.click(screen.getByRole('button', { name: 'Reifen' }))
    expect(screen.getByLabelText('Flipchip-Position')).toBeInTheDocument()
    expect(screen.getByLabelText('Vorne: Breite (in)')).toBeInTheDocument()
    expect(screen.getByLabelText('Vorne: Druck (bar)')).toBeInTheDocument()
    expect(screen.queryByLabelText('Vorne: Breite (mm)')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Vorne: Druck (PSI)')).not.toBeInTheDocument()

    await user.selectOptions(screen.getByLabelText('Flipchip-Position'), 'low')
    await user.type(screen.getByLabelText('Vorne: Breite (in)'), '2.5')
    await user.type(screen.getByLabelText('Vorne: Druck (bar)'), '1.7')

    const saveButtons = screen.getAllByRole('button', { name: 'Speichern' })
    await user.click(saveButtons[0])

    await waitFor(() => {
      expect(createBikeSetup).toHaveBeenCalledWith(
        'g1',
        expect.objectContaining({
          suspension_rear: expect.objectContaining({ flipchip_position: 'low' }),
          tires: expect.objectContaining({
            front_width_in: 2.5,
            front_pressure_bar: 1.7,
          }),
        }),
      )
    })
  })

  it('TestBikeSetupCaptureComponentsHelpUsesIconWithoutBrokenGlyph', async () => {
    const user = userEvent.setup()
    vi.mocked(listBikeSetups).mockResolvedValue([MOCK_SETUP_1])

    renderPage('/gear/g1?tab=setups')

    await screen.findByRole('combobox', { name: /Setup auswählen/i })
    await user.click(screen.getByRole('button', { name: '+ Neu' }))

    expect(screen.getByLabelText(/Speichert eine Momentaufnahme der montierten Komponenten/i)).toBeInTheDocument()
    expect(screen.queryByText('ⓘ')).not.toBeInTheDocument()
  })

  it('TestBikeSetupSnapshotRendersInfoBanner', async () => {
    // Render a setup with component_snapshot; expand the collapsible; assert info banner.
    const user = userEvent.setup()
    vi.mocked(listBikeSetups).mockResolvedValue([MOCK_SETUP_WITH_SNAPSHOT])

    renderPage('/gear/g1?tab=setups')

    // Wait for the section to render.
    await screen.findByRole('combobox', { name: /Setup auswählen/i })

    // The collapsible toggle must be present.
    const toggle = await screen.findByRole('button', { name: /2 Komponenten-Snapshot/i })
    await user.click(toggle)

    // After expanding, the info note must be visible.
    await waitFor(() => {
      expect(screen.getByRole('note')).toBeInTheDocument()
    })
    expect(screen.getByRole('note')).toHaveTextContent('Dieser Snapshot zeigt die beim Speichern montierten Komponenten.')
  })

  it('TestBikeSetupActivationDoesNotPromptComponentChange', async () => {
    // Render setup with snapshot, click "Aktivieren"; verify activateBikeSetup is called
    // and no component-mutation prompt appears.
    const user = userEvent.setup()
    const activatedSetup: BikeSetup = { ...MOCK_SETUP_WITH_SNAPSHOT, is_active: true }
    vi.mocked(listBikeSetups)
      .mockResolvedValueOnce([MOCK_SETUP_WITH_SNAPSHOT])
      .mockResolvedValueOnce([activatedSetup])
    vi.mocked(activateBikeSetup).mockResolvedValue(activatedSetup)

    renderPage('/gear/g1?tab=setups')

    // Wait for setup to render.
    await screen.findByRole('combobox', { name: /Setup auswählen/i })

    // Click "Aktivieren".
    const aktivierenBtn = await screen.findByRole('button', { name: 'Aktivieren' })
    await user.click(aktivierenBtn)

    // activateBikeSetup must have been called with 's3' as the first argument.
    await waitFor(() => {
      expect(activateBikeSetup).toHaveBeenCalled()
      expect(vi.mocked(activateBikeSetup).mock.calls[0][0]).toBe('s3')
    })

    // No component-mutation prompt / dialog should have appeared.
    expect(screen.queryByText(/Komponenten.*geändert/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})
