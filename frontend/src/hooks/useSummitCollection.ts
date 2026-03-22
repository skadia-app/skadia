import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchSummitCollection } from '@/api/summits'
import type { ElevClass } from '@/components/summit/summitConstants'

interface UseSummitCollectionOptions {
  countryFilter: string
  elevClass: ElevClass
  yearFilter: string
}

export function useSummitCollection({ countryFilter, elevClass, yearFilter }: UseSummitCollectionOptions) {
  const { data: allSummits, isLoading: isLoadingAll } = useQuery({
    queryKey: ['summit-collection-all'],
    queryFn: () => fetchSummitCollection(),
    staleTime: 5 * 60 * 1000,
  })

  const isFiltered = !!(countryFilter || elevClass.min != null || elevClass.max != null || yearFilter)

  const { data: filteredData, isLoading: isLoadingFiltered } = useQuery({
    queryKey: ['summit-collection', countryFilter, elevClass.label, yearFilter],
    queryFn: () => fetchSummitCollection({
      country: countryFilter || undefined,
      min_elevation: elevClass.min ?? undefined,
      max_elevation: elevClass.max ?? undefined,
      year: yearFilter ? Number(yearFilter) : undefined,
    }),
    enabled: isFiltered,
    staleTime: 2 * 60 * 1000,
  })

  const displaySummits = (isFiltered ? filteredData : allSummits) ?? []
  const isLoading = isLoadingAll || (isFiltered && isLoadingFiltered)

  const availableCountries = useMemo(() =>
    [...new Set((allSummits ?? []).flatMap(s => s.country_codes ?? []).filter(Boolean))].sort(),
    [allSummits]
  )

  const availableYears = useMemo(() => {
    const years = new Set<string>()
    for (const s of (allSummits ?? [])) {
      if (s.first_visit) years.add(new Date(s.first_visit).getFullYear().toString())
      if (s.last_visit) years.add(new Date(s.last_visit).getFullYear().toString())
    }
    return [...years].sort().reverse()
  }, [allSummits])

  return { allSummits: allSummits ?? [], displaySummits, isLoading, isFiltered, availableCountries, availableYears }
}
