## frontend/src/components/PhotoGeoAssignmentOverlay.tsx — Photo Geo Overlay v2
**Suche nach:** PhotoGeoAssignmentOverlay
**Delta:**
- Karte lädt korrekt (mapContainerRef ist direktes flex-item, nicht absolute child)
- Stil-Toggle: OSM / Topo / Satellit — oben links, `buildRasterStyle()` + persistenter `style.load`-Handler
- HTML-Marker statt GeoJSON circles: 56×42px Thumbnail + weißes Dreieck (anchor: 'bottom'); Thumbnail via `apiDownload()`
- Draggable markers: `dragend` → PATCH → `marker.setLngLat()` (kein Neuladen)
- `positionedPhotos` Prop: bereits positionierte Fotos beim Overlay-Öffnen als Marker auf Karte
- Sidebar "Positioniert"-Sektion: grüner Pin-Badge, auch per Drag-from-Sidebar repositionierbar
- Re-entry: Karte zeigt beim nächsten Öffnen alle bereits positionierten Fotos als verschiebbare Marker

## frontend/src/components/ActivityPhotoSection.tsx — positionedPhotos + Repositionier-Button
**Suche nach:** photosWithPosition
**Delta:**
- `photosWithPosition` computed (photos mit lat != null)
- `positionedPhotos={photosWithPosition}` an Overlay übergeben
- "Positionen bearbeiten"-Button wenn alle Fotos bereits positioniert sind
