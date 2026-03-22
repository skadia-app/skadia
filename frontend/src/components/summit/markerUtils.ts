import maplibregl from 'maplibre-gl'
import type { SummitVisit } from '@/api/summits'

export function createSummitMarker(summit: SummitVisit, isSelected: boolean): maplibregl.Marker {
  const el = document.createElement('div')
  el.className = 'summit-marker'
  el.style.cssText = 'width:28px;height:28px;cursor:pointer;display:flex;align-items:center;justify-content:center;'
  el.innerHTML = isSelected
    ? `<svg width="28" height="28" viewBox="0 0 24 24" fill="#f97316" stroke="white" stroke-width="2"><path d="M3 20l6.5-11L13 15l4-6 4 11H3z"/></svg>`
    : `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#c2410c" stroke-width="2.5"><path d="M3 20l6.5-11L13 15l4-6 4 11H3z"/></svg>`
  return new maplibregl.Marker({ element: el })
}
