// lift_detect_viz renders an interactive HTML map showing the full activity
// track plus the detected lift segments as colored overlays. Used to validate
// detector behaviour visually — does the segment cover the right start/end
// stations? does an "Umstieg" really transition between two distinct lifts?
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
)

type trackPt struct {
	T   string  `json:"t"`
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
	Alt float64 `json:"alt"`
}

type rideSeg struct {
	Idx       int       `json:"idx"`
	Start     string    `json:"start"`
	End       string    `json:"end"`
	DurationS int       `json:"dur"`
	AscentM   float64   `json:"asc"`
	DescentM  float64   `json:"desc"`
	DistanceM float64   `json:"dist"`
	Conf      string    `json:"conf"`
	Path      []trackPt `json:"path"`
}

type viewModel struct {
	Title     string
	Banner    string
	Track     template.JS // raw JSON for inline JS context — html/template would otherwise HTML-escape it
	Rides     template.JS
	CenterLat float64
	CenterLng float64
}

func main() {
	file := flag.String("file", "", "FIT or GPX")
	sport := flag.String("sport", "ski", "")
	out := flag.String("out", "", "output HTML (default: <input>.viz.html)")
	banner := flag.String("banner", "", "label rendered in title + corner overlay (e.g. 'PRE-C1' / 'POST-C1+C2')")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "need -file")
		os.Exit(2)
	}
	f, err := os.Open(*file)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	var pts []lifts.StreamPoint
	switch strings.ToLower(filepath.Ext(*file)) {
	case ".fit":
		data, err := fitimport.ParseFIT(f, *file)
		if err != nil {
			panic(err)
		}
		pts = lifts.ConvertActivityDataToStream(data)
	case ".gpx":
		data, err := gpximport.ParseGPX(f, *file)
		if err != nil {
			panic(err)
		}
		pts = lifts.ConvertActivityDataToStream(data)
	default:
		fmt.Fprintln(os.Stderr, "unsupported")
		os.Exit(2)
	}
	cfg := lifts.DefaultDetectorConfig(*sport)
	rides := lifts.DetectHeuristic(pts, cfg)

	track := make([]trackPt, 0, len(pts))
	var sumLat, sumLng float64
	var nLatLng int
	for _, p := range pts {
		if p.Lat == nil || p.Lng == nil {
			continue
		}
		alt := 0.0
		if p.AltitudeM != nil {
			alt = *p.AltitudeM
		}
		track = append(track, trackPt{
			T:   p.Time.Format(time.RFC3339),
			Lat: *p.Lat,
			Lng: *p.Lng,
			Alt: alt,
		})
		sumLat += *p.Lat
		sumLng += *p.Lng
		nLatLng++
	}

	rideSegs := make([]rideSeg, 0, len(rides))
	for i, r := range rides {
		path := make([]trackPt, 0)
		for _, p := range pts {
			if p.Time.Before(r.StartedAt) || p.Time.After(r.EndedAt) {
				continue
			}
			if p.Lat == nil || p.Lng == nil {
				continue
			}
			alt := 0.0
			if p.AltitudeM != nil {
				alt = *p.AltitudeM
			}
			path = append(path, trackPt{
				T:   p.Time.Format(time.RFC3339),
				Lat: *p.Lat,
				Lng: *p.Lng,
				Alt: alt,
			})
		}
		rideSegs = append(rideSegs, rideSeg{
			Idx:       i,
			Start:     r.StartedAt.Format(time.RFC3339),
			End:       r.EndedAt.Format(time.RFC3339),
			DurationS: r.DurationS,
			AscentM:   r.AscentM,
			DescentM:  r.DescentM,
			DistanceM: r.DistanceM,
			Conf:      r.Confidence,
			Path:      path,
		})
	}

	trackJSON, _ := json.Marshal(track)
	ridesJSON, _ := json.Marshal(rideSegs)

	cLat, cLng := 0.0, 0.0
	if nLatLng > 0 {
		cLat = sumLat / float64(nLatLng)
		cLng = sumLng / float64(nLatLng)
	}

	title := filepath.Base(*file)
	if *banner != "" {
		title = "[" + *banner + "] " + title
	}
	vm := viewModel{
		Title:     title,
		Banner:    *banner,
		Track:     template.JS(trackJSON),
		Rides:     template.JS(ridesJSON),
		CenterLat: cLat,
		CenterLng: cLng,
	}

	outPath := *out
	if outPath == "" {
		outPath = *file + ".viz.html"
	}
	w, err := os.Create(outPath)
	if err != nil {
		panic(err)
	}
	defer w.Close()
	if err := tpl.Execute(w, vm); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s (%d rides)\n", outPath, len(rideSegs))
}

var tpl = template.Must(template.New("viz").Parse(`<!doctype html>
<html lang="de">
<head>
<meta charset="utf-8">
<title>Lift-Detection — {{.Title}}</title>
<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css" integrity="sha256-p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY=" crossorigin="">
<style>
  html, body { margin: 0; padding: 0; height: 100%; font-family: system-ui, sans-serif; }
  #app { display: grid; grid-template-columns: 1fr 360px; height: 100%; }
  #map { height: 100%; }
  #panel { overflow-y: auto; padding: 12px; border-left: 1px solid #ccc; background: #fafafa; }
  h1 { font-size: 16px; margin: 0 0 8px; }
  .ride { padding: 8px; margin-bottom: 6px; border-radius: 4px; cursor: pointer; border-left: 4px solid #888; background: white; box-shadow: 0 1px 2px rgba(0,0,0,0.06); }
  .ride.active { background: #fff7d0; }
  .ride .meta { font-size: 11px; color: #555; margin-top: 2px; }
  .ride .conf { display: inline-block; padding: 1px 6px; border-radius: 3px; font-size: 10px; font-weight: bold; color: white; margin-left: 4px; }
  .conf.high { background: #2a7e2a; }
  .conf.medium { background: #c98a14; }
  .conf.bidir { background: #7a3aa8; }
  .conf.low { background: #888; }
  .legend { font-size: 11px; color: #444; padding: 6px 0; border-bottom: 1px solid #ddd; margin-bottom: 8px; }
</style>
</head>
<body>
<div id="app">
  <div id="map">
    {{if .Banner}}<div style="position:absolute;top:8px;left:50px;z-index:1000;background:rgba(255,200,0,0.9);padding:6px 12px;font-weight:bold;font-size:14px;border-radius:4px;box-shadow:0 1px 4px rgba(0,0,0,0.3);">{{.Banner}}</div>{{end}}
  </div>
  <div id="panel">
    <h1>{{.Title}}</h1>
    <div class="legend">
      <span style="display:inline-block;width:20px;height:3px;background:#888;vertical-align:middle"></span> Track &nbsp;
      <span style="display:inline-block;width:20px;height:3px;background:#2a7e2a;vertical-align:middle"></span> high &nbsp;
      <span style="display:inline-block;width:20px;height:3px;background:#c98a14;vertical-align:middle"></span> medium &nbsp;
      <span style="display:inline-block;width:20px;height:3px;background:#7a3aa8;vertical-align:middle"></span> bidir
    </div>
    <div id="rides"></div>
  </div>
</div>
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js" integrity="sha256-20nQCchB9co0qIjJZRGuk2/Z9VM+kNiyxNV1lvTlZBo=" crossorigin=""></script>
<script>
  const TRACK = {{.Track}};
  const RIDES = {{.Rides}};
  const map = L.map('map').setView([{{.CenterLat}}, {{.CenterLng}}], 13);
  // CARTO Voyager + OpenTopoMap allow direct file:// referer-less requests.
  // OSM main tiles return 403 because their volunteer policy requires a referer.
  const baseLayers = {
    'CARTO Voyager': L.tileLayer('https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png', {
      attribution: '&copy; OpenStreetMap, &copy; CARTO', subdomains: 'abcd', maxZoom: 19
    }),
    'OpenTopoMap': L.tileLayer('https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png', {
      attribution: '&copy; OpenStreetMap, SRTM | &copy; OpenTopoMap (CC-BY-SA)', maxZoom: 17
    }),
    'CARTO Positron': L.tileLayer('https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png', {
      attribution: '&copy; OpenStreetMap, &copy; CARTO', subdomains: 'abcd', maxZoom: 19
    })
  };
  baseLayers['CARTO Voyager'].addTo(map);
  L.control.layers(baseLayers, null, { collapsed: false }).addTo(map);

  // base track
  const trackLatLng = TRACK.map(p => [p.lat, p.lng]);
  const trackLine = L.polyline(trackLatLng, { color: '#888', weight: 2, opacity: 0.6 }).addTo(map);
  if (trackLatLng.length) map.fitBounds(trackLine.getBounds(), { padding: [16, 16] });

  const confColor = c => ({ high: '#2a7e2a', medium: '#c98a14', bidir: '#7a3aa8', low: '#888' })[c] || '#444';

  const rideLayers = [];
  RIDES.forEach((r, i) => {
    const ll = r.path.map(p => [p.lat, p.lng]);
    const line = L.polyline(ll, { color: confColor(r.conf), weight: 5, opacity: 0.85 }).addTo(map);
    line.bindPopup(
      '<b>ride[' + r.idx + ']</b> ' + r.conf + '<br>' +
      'start ' + r.start.replace('T', ' ').slice(11, 19) + '<br>' +
      'end &nbsp;&nbsp;' + r.end.replace('T', ' ').slice(11, 19) + '<br>' +
      'dur ' + r.dur + 's<br>' +
      'asc ' + r.asc.toFixed(1) + ' m, desc ' + r.desc.toFixed(1) + ' m<br>' +
      'dist ' + r.dist.toFixed(0) + ' m'
    );
    if (ll.length) {
      const startMarker = L.circleMarker(ll[0], { color: confColor(r.conf), radius: 6, fillOpacity: 1, weight: 2, fillColor: 'white' }).addTo(map);
      startMarker.bindTooltip('start[' + r.idx + ']');
      const endMarker = L.circleMarker(ll[ll.length - 1], { color: confColor(r.conf), radius: 6, fillOpacity: 1, weight: 2, fillColor: confColor(r.conf) }).addTo(map);
      endMarker.bindTooltip('end[' + r.idx + ']');
    }
    rideLayers.push(line);
  });

  const ridesEl = document.getElementById('rides');
  RIDES.forEach((r, i) => {
    const div = document.createElement('div');
    div.className = 'ride';
    div.style.borderLeftColor = confColor(r.conf);
    div.innerHTML =
      '<b>ride[' + r.idx + ']</b><span class="conf ' + r.conf + '">' + r.conf + '</span>' +
      '<div class="meta">' + r.start.slice(11, 19) + ' &rarr; ' + r.end.slice(11, 19) + ' &nbsp;|&nbsp; ' + r.dur + 's</div>' +
      '<div class="meta">+' + r.asc.toFixed(0) + 'm / &minus;' + r.desc.toFixed(0) + 'm &nbsp;|&nbsp; ' + r.dist.toFixed(0) + 'm</div>';
    div.addEventListener('click', () => {
      document.querySelectorAll('.ride').forEach(x => x.classList.remove('active'));
      div.classList.add('active');
      const ll = rideLayers[i].getLatLngs();
      if (ll.length) {
        map.fitBounds(rideLayers[i].getBounds(), { padding: [40, 40] });
        rideLayers[i].openPopup();
      }
    });
    ridesEl.appendChild(div);
  });

  // Optional ?ride=N param: programmatic equivalent of clicking ride N in
  // the sidebar — fits the map to that ride and opens its popup. Useful
  // when an external tool (a screenshot pipeline, a deep-link from a doc)
  // needs to land on a specific ride.
  const rideParam = new URLSearchParams(window.location.search).get('ride');
  if (rideParam !== null && rideLayers[+rideParam]) {
    const i = +rideParam;
    document.querySelectorAll('.ride')[i].classList.add('active');
    map.fitBounds(rideLayers[i].getBounds(), { padding: [40, 40] });
  }
</script>
</body>
</html>
`))
