import maplibregl from 'maplibre-gl'

export interface SummitMarker {
  marker: maplibregl.Marker
  abort: () => void
}

export function createSummitMarker(
  isSelected: boolean,
  onClick: () => void,
): SummitMarker {
  const el = document.createElement('div')
  el.className = 'summit-marker'
  el.style.cssText = 'width:28px;height:28px;cursor:pointer;display:flex;align-items:center;justify-content:center;'
  el.innerHTML = isSelected
    ? `<svg width="28" height="28" viewBox="0 0 24 24" fill="#f97316" stroke="white" stroke-width="2"><path d="M3 20l6.5-11L13 15l4-6 4 11H3z"/></svg>`
    : `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#c2410c" stroke-width="2.5"><path d="M3 20l6.5-11L13 15l4-6 4 11H3z"/></svg>`

  const controller = new AbortController()
  el.addEventListener('click', onClick, { signal: controller.signal })

  const marker = new maplibregl.Marker({ element: el })
  return { marker, abort: () => controller.abort() }
}
