package osm

// Region beschreibt eine downloadbare OSM-Region von GeoFabrik.
type Region struct {
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	Continent string  `json:"continent"`
	URL       string  `json:"url"`     // vollständige GeoFabrik-Download-URL
	SizeMB    int     `json:"size_mb"` // geschätzte PBF-Größe in MB
	MinLon    float64 `json:"min_lon"` // BBox West
	MinLat    float64 `json:"min_lat"` // BBox Süd
	MaxLon    float64 `json:"max_lon"` // BBox Ost
	MaxLat    float64 `json:"max_lat"` // BBox Nord
}

const geofabrikBase = "https://download.geofabrik.de/"

var Regions = []Region{
	// Europa — Westeuropa
	{ID: "germany", Label: "Deutschland", Continent: "Europa", URL: geofabrikBase + "europe/germany-latest.osm.pbf", SizeMB: 4000, MinLon: 5.87, MinLat: 47.27, MaxLon: 15.04, MaxLat: 55.06},
	{ID: "austria", Label: "Österreich", Continent: "Europa", URL: geofabrikBase + "europe/austria-latest.osm.pbf", SizeMB: 700, MinLon: 9.53, MinLat: 46.37, MaxLon: 17.16, MaxLat: 49.02},
	{ID: "switzerland", Label: "Schweiz", Continent: "Europa", URL: geofabrikBase + "europe/switzerland-latest.osm.pbf", SizeMB: 380, MinLon: 5.96, MinLat: 45.82, MaxLon: 10.49, MaxLat: 47.81},
	{ID: "liechtenstein", Label: "Liechtenstein", Continent: "Europa", URL: geofabrikBase + "europe/liechtenstein-latest.osm.pbf", SizeMB: 2, MinLon: 9.47, MinLat: 47.05, MaxLon: 9.64, MaxLat: 47.27},
	{ID: "france", Label: "Frankreich", Continent: "Europa", URL: geofabrikBase + "europe/france-latest.osm.pbf", SizeMB: 4200, MinLon: -5.14, MinLat: 41.36, MaxLon: 9.56, MaxLat: 51.09},
	{ID: "andorra", Label: "Andorra", Continent: "Europa", URL: geofabrikBase + "europe/andorra-latest.osm.pbf", SizeMB: 3, MinLon: 1.41, MinLat: 42.43, MaxLon: 1.79, MaxLat: 42.66},
	{ID: "monaco", Label: "Monaco", Continent: "Europa", URL: geofabrikBase + "europe/monaco-latest.osm.pbf", SizeMB: 1, MinLon: 7.41, MinLat: 43.72, MaxLon: 7.44, MaxLat: 43.75},
	{ID: "spain", Label: "Spanien", Continent: "Europa", URL: geofabrikBase + "europe/spain-latest.osm.pbf", SizeMB: 1800, MinLon: -18.17, MinLat: 27.64, MaxLon: 4.33, MaxLat: 43.79},
	{ID: "portugal", Label: "Portugal", Continent: "Europa", URL: geofabrikBase + "europe/portugal-latest.osm.pbf", SizeMB: 350, MinLon: -31.27, MinLat: 32.40, MaxLon: -6.19, MaxLat: 42.15},
	{ID: "italy", Label: "Italien", Continent: "Europa", URL: geofabrikBase + "europe/italy-latest.osm.pbf", SizeMB: 2800, MinLon: 6.63, MinLat: 35.49, MaxLon: 18.52, MaxLat: 47.09},
	{ID: "san-marino", Label: "San Marino", Continent: "Europa", URL: geofabrikBase + "europe/san-marino-latest.osm.pbf", SizeMB: 1, MinLon: 12.40, MinLat: 43.89, MaxLon: 12.52, MaxLat: 43.99},
	{ID: "great-britain", Label: "Großbritannien", Continent: "Europa", URL: geofabrikBase + "europe/great-britain-latest.osm.pbf", SizeMB: 1800, MinLon: -8.65, MinLat: 49.86, MaxLon: 1.77, MaxLat: 60.86},
	{ID: "ireland", Label: "Irland", Continent: "Europa", URL: geofabrikBase + "europe/ireland-and-northern-ireland-latest.osm.pbf", SizeMB: 200, MinLon: -10.48, MinLat: 51.42, MaxLon: -5.99, MaxLat: 55.38},
	{ID: "isle-of-man", Label: "Isle of Man", Continent: "Europa", URL: geofabrikBase + "europe/isle-of-man-latest.osm.pbf", SizeMB: 3, MinLon: -4.79, MinLat: 54.06, MaxLon: -4.31, MaxLat: 54.42},
	{ID: "netherlands", Label: "Niederlande", Continent: "Europa", URL: geofabrikBase + "europe/netherlands-latest.osm.pbf", SizeMB: 900, MinLon: 3.36, MinLat: 50.75, MaxLon: 7.21, MaxLat: 53.47},
	{ID: "belgium", Label: "Belgien", Continent: "Europa", URL: geofabrikBase + "europe/belgium-latest.osm.pbf", SizeMB: 400, MinLon: 2.54, MinLat: 49.50, MaxLon: 6.41, MaxLat: 51.50},
	{ID: "luxembourg", Label: "Luxemburg", Continent: "Europa", URL: geofabrikBase + "europe/luxembourg-latest.osm.pbf", SizeMB: 30, MinLon: 5.73, MinLat: 49.45, MaxLon: 6.53, MaxLat: 50.18},
	// Europa — Nordeuropa
	{ID: "norway", Label: "Norwegen", Continent: "Europa", URL: geofabrikBase + "europe/norway-latest.osm.pbf", SizeMB: 1000, MinLon: 4.65, MinLat: 57.96, MaxLon: 31.07, MaxLat: 71.19},
	{ID: "sweden", Label: "Schweden", Continent: "Europa", URL: geofabrikBase + "europe/sweden-latest.osm.pbf", SizeMB: 800, MinLon: 11.11, MinLat: 55.34, MaxLon: 24.17, MaxLat: 69.06},
	{ID: "denmark", Label: "Dänemark", Continent: "Europa", URL: geofabrikBase + "europe/denmark-latest.osm.pbf", SizeMB: 300, MinLon: 8.08, MinLat: 54.57, MaxLon: 15.16, MaxLat: 57.75},
	{ID: "finland", Label: "Finnland", Continent: "Europa", URL: geofabrikBase + "europe/finland-latest.osm.pbf", SizeMB: 350, MinLon: 20.55, MinLat: 59.81, MaxLon: 31.59, MaxLat: 70.09},
	{ID: "iceland", Label: "Island", Continent: "Europa", URL: geofabrikBase + "europe/iceland-latest.osm.pbf", SizeMB: 60, MinLon: -24.55, MinLat: 63.39, MaxLon: -13.50, MaxLat: 66.54},
	{ID: "faroe-islands", Label: "Färöer", Continent: "Europa", URL: geofabrikBase + "europe/faroe-islands-latest.osm.pbf", SizeMB: 4, MinLon: -7.42, MinLat: 61.39, MaxLon: -6.26, MaxLat: 62.40},
	// Europa — Mittel- und Osteuropa
	{ID: "poland", Label: "Polen", Continent: "Europa", URL: geofabrikBase + "europe/poland-latest.osm.pbf", SizeMB: 1600, MinLon: 14.12, MinLat: 49.00, MaxLon: 24.15, MaxLat: 54.84},
	{ID: "czech-republic", Label: "Tschechien", Continent: "Europa", URL: geofabrikBase + "europe/czech-republic-latest.osm.pbf", SizeMB: 650, MinLon: 12.09, MinLat: 48.55, MaxLon: 18.86, MaxLat: 51.06},
	{ID: "slovakia", Label: "Slowakei", Continent: "Europa", URL: geofabrikBase + "europe/slovakia-latest.osm.pbf", SizeMB: 200, MinLon: 16.84, MinLat: 47.73, MaxLon: 22.57, MaxLat: 49.61},
	{ID: "hungary", Label: "Ungarn", Continent: "Europa", URL: geofabrikBase + "europe/hungary-latest.osm.pbf", SizeMB: 400, MinLon: 16.11, MinLat: 45.74, MaxLon: 22.90, MaxLat: 48.58},
	{ID: "romania", Label: "Rumänien", Continent: "Europa", URL: geofabrikBase + "europe/romania-latest.osm.pbf", SizeMB: 600, MinLon: 20.26, MinLat: 43.62, MaxLon: 29.69, MaxLat: 48.27},
	{ID: "ukraine", Label: "Ukraine", Continent: "Europa", URL: geofabrikBase + "europe/ukraine-latest.osm.pbf", SizeMB: 650, MinLon: 22.14, MinLat: 44.39, MaxLon: 40.23, MaxLat: 52.38},
	{ID: "belarus", Label: "Belarus", Continent: "Europa", URL: geofabrikBase + "europe/belarus-latest.osm.pbf", SizeMB: 400, MinLon: 23.18, MinLat: 51.26, MaxLon: 32.77, MaxLat: 56.17},
	{ID: "moldova", Label: "Moldau", Continent: "Europa", URL: geofabrikBase + "europe/moldova-latest.osm.pbf", SizeMB: 60, MinLon: 26.62, MinLat: 45.47, MaxLon: 30.16, MaxLat: 48.49},
	{ID: "estonia", Label: "Estland", Continent: "Europa", URL: geofabrikBase + "europe/estonia-latest.osm.pbf", SizeMB: 60, MinLon: 21.76, MinLat: 57.52, MaxLon: 28.21, MaxLat: 59.69},
	{ID: "latvia", Label: "Lettland", Continent: "Europa", URL: geofabrikBase + "europe/latvia-latest.osm.pbf", SizeMB: 100, MinLon: 20.97, MinLat: 55.67, MaxLon: 28.24, MaxLat: 58.09},
	{ID: "lithuania", Label: "Litauen", Continent: "Europa", URL: geofabrikBase + "europe/lithuania-latest.osm.pbf", SizeMB: 100, MinLon: 20.93, MinLat: 53.90, MaxLon: 26.84, MaxLat: 56.45},
	// Europa — Südosteuropa
	{ID: "slovenia", Label: "Slowenien", Continent: "Europa", URL: geofabrikBase + "europe/slovenia-latest.osm.pbf", SizeMB: 100, MinLon: 13.38, MinLat: 45.42, MaxLon: 16.61, MaxLat: 46.88},
	{ID: "croatia", Label: "Kroatien", Continent: "Europa", URL: geofabrikBase + "europe/croatia-latest.osm.pbf", SizeMB: 250, MinLon: 13.49, MinLat: 42.39, MaxLon: 19.43, MaxLat: 46.55},
	{ID: "bosnia-herzegovina", Label: "Bosnien-Herzegowina", Continent: "Europa", URL: geofabrikBase + "europe/bosnia-herzegovina-latest.osm.pbf", SizeMB: 120, MinLon: 15.72, MinLat: 42.56, MaxLon: 19.62, MaxLat: 45.28},
	{ID: "serbia", Label: "Serbien", Continent: "Europa", URL: geofabrikBase + "europe/serbia-latest.osm.pbf", SizeMB: 250, MinLon: 18.83, MinLat: 42.23, MaxLon: 23.01, MaxLat: 46.19},
	{ID: "montenegro", Label: "Montenegro", Continent: "Europa", URL: geofabrikBase + "europe/montenegro-latest.osm.pbf", SizeMB: 50, MinLon: 18.43, MinLat: 41.85, MaxLon: 20.36, MaxLat: 43.56},
	{ID: "kosovo", Label: "Kosovo", Continent: "Europa", URL: geofabrikBase + "europe/kosovo-latest.osm.pbf", SizeMB: 50, MinLon: 20.01, MinLat: 41.86, MaxLon: 21.79, MaxLat: 43.27},
	{ID: "north-macedonia", Label: "Nordmazedonien", Continent: "Europa", URL: geofabrikBase + "europe/north-macedonia-latest.osm.pbf", SizeMB: 100, MinLon: 20.46, MinLat: 40.85, MaxLon: 23.04, MaxLat: 42.36},
	{ID: "albania", Label: "Albanien", Continent: "Europa", URL: geofabrikBase + "europe/albania-latest.osm.pbf", SizeMB: 80, MinLon: 19.26, MinLat: 39.64, MaxLon: 21.06, MaxLat: 42.66},
	{ID: "bulgaria", Label: "Bulgarien", Continent: "Europa", URL: geofabrikBase + "europe/bulgaria-latest.osm.pbf", SizeMB: 300, MinLon: 22.36, MinLat: 41.24, MaxLon: 28.61, MaxLat: 44.22},
	{ID: "greece", Label: "Griechenland", Continent: "Europa", URL: geofabrikBase + "europe/greece-latest.osm.pbf", SizeMB: 550, MinLon: 19.37, MinLat: 34.80, MaxLon: 29.65, MaxLat: 41.75},
	{ID: "turkey", Label: "Türkei", Continent: "Europa", URL: geofabrikBase + "europe/turkey-latest.osm.pbf", SizeMB: 750, MinLon: 25.67, MinLat: 35.82, MaxLon: 44.79, MaxLat: 42.11},
	{ID: "cyprus", Label: "Zypern", Continent: "Europa", URL: geofabrikBase + "europe/cyprus-latest.osm.pbf", SizeMB: 80, MinLon: 32.27, MinLat: 34.57, MaxLon: 34.60, MaxLat: 35.71},
	{ID: "malta", Label: "Malta", Continent: "Europa", URL: geofabrikBase + "europe/malta-latest.osm.pbf", SizeMB: 5, MinLon: 14.18, MinLat: 35.81, MaxLon: 14.57, MaxLat: 36.08},
	// Europa — Gesamt
	{ID: "russia", Label: "Russland", Continent: "Europa", URL: geofabrikBase + "russia-latest.osm.pbf", SizeMB: 65000, MinLon: 27.31, MinLat: 41.19, MaxLon: 180, MaxLat: 81.86},
	{ID: "europe", Label: "Europa gesamt", Continent: "Europa", URL: geofabrikBase + "europe-latest.osm.pbf", SizeMB: 28000, MinLon: -31.27, MinLat: 27.64, MaxLon: 44.79, MaxLat: 71.19},
	// Asien — Ostasien
	{ID: "japan", Label: "Japan", Continent: "Asien", URL: geofabrikBase + "asia/japan-latest.osm.pbf", SizeMB: 2000, MinLon: 122.93, MinLat: 24.05, MaxLon: 153.99, MaxLat: 45.52},
	{ID: "south-korea", Label: "Südkorea", Continent: "Asien", URL: geofabrikBase + "asia/south-korea-latest.osm.pbf", SizeMB: 550, MinLon: 124.61, MinLat: 33.11, MaxLon: 131.87, MaxLat: 38.62},
	{ID: "china", Label: "China", Continent: "Asien", URL: geofabrikBase + "asia/china-latest.osm.pbf", SizeMB: 11000, MinLon: 73.50, MinLat: 18.16, MaxLon: 134.77, MaxLat: 53.56},
	{ID: "taiwan", Label: "Taiwan", Continent: "Asien", URL: geofabrikBase + "asia/taiwan-latest.osm.pbf", SizeMB: 300, MinLon: 119.53, MinLat: 21.90, MaxLon: 122.01, MaxLat: 25.30},
	{ID: "mongolia", Label: "Mongolei", Continent: "Asien", URL: geofabrikBase + "asia/mongolia-latest.osm.pbf", SizeMB: 180, MinLon: 87.75, MinLat: 41.58, MaxLon: 119.93, MaxLat: 52.15},
	// Asien — Südasien
	{ID: "india", Label: "Indien", Continent: "Asien", URL: geofabrikBase + "asia/india-latest.osm.pbf", SizeMB: 2300, MinLon: 68.11, MinLat: 6.75, MaxLon: 97.40, MaxLat: 35.50},
	{ID: "nepal", Label: "Nepal", Continent: "Asien", URL: geofabrikBase + "asia/nepal-latest.osm.pbf", SizeMB: 150, MinLon: 80.06, MinLat: 26.35, MaxLon: 88.20, MaxLat: 30.45},
	{ID: "pakistan", Label: "Pakistan", Continent: "Asien", URL: geofabrikBase + "asia/pakistan-latest.osm.pbf", SizeMB: 700, MinLon: 60.87, MinLat: 23.69, MaxLon: 77.84, MaxLat: 37.08},
	{ID: "sri-lanka", Label: "Sri Lanka", Continent: "Asien", URL: geofabrikBase + "asia/sri-lanka-latest.osm.pbf", SizeMB: 120, MinLon: 79.65, MinLat: 5.92, MaxLon: 81.88, MaxLat: 9.84},
	{ID: "bangladesh", Label: "Bangladesch", Continent: "Asien", URL: geofabrikBase + "asia/bangladesh-latest.osm.pbf", SizeMB: 190, MinLon: 88.01, MinLat: 20.74, MaxLon: 92.67, MaxLat: 26.63},
	{ID: "afghanistan", Label: "Afghanistan", Continent: "Asien", URL: geofabrikBase + "asia/afghanistan-latest.osm.pbf", SizeMB: 200, MinLon: 60.50, MinLat: 29.38, MaxLon: 74.89, MaxLat: 38.49},
	// Asien — Zentralasien
	{ID: "central-asia", Label: "Zentralasien", Continent: "Asien", URL: geofabrikBase + "asia/central-asia-latest.osm.pbf", SizeMB: 600, MinLon: 46.49, MinLat: 35.14, MaxLon: 87.32, MaxLat: 55.44},
	// Asien — Südostasien
	{ID: "southeast-asia", Label: "Südostasien", Continent: "Asien", URL: geofabrikBase + "asia/southeast-asia-latest.osm.pbf", SizeMB: 2500, MinLon: 92.19, MinLat: -11.01, MaxLon: 141.02, MaxLat: 28.55},
	{ID: "philippines", Label: "Philippinen", Continent: "Asien", URL: geofabrikBase + "asia/philippines-latest.osm.pbf", SizeMB: 600, MinLon: 116.93, MinLat: 4.59, MaxLon: 126.60, MaxLat: 21.12},
	// Asien — Naher Osten
	{ID: "iran", Label: "Iran", Continent: "Asien", URL: geofabrikBase + "asia/iran-latest.osm.pbf", SizeMB: 750, MinLon: 44.03, MinLat: 25.06, MaxLon: 63.32, MaxLat: 39.78},
	{ID: "iraq", Label: "Irak", Continent: "Asien", URL: geofabrikBase + "asia/iraq-latest.osm.pbf", SizeMB: 250, MinLon: 38.79, MinLat: 29.06, MaxLon: 48.56, MaxLat: 37.38},
	{ID: "syria", Label: "Syrien", Continent: "Asien", URL: geofabrikBase + "asia/syria-latest.osm.pbf", SizeMB: 180, MinLon: 35.73, MinLat: 32.31, MaxLon: 42.38, MaxLat: 37.32},
	{ID: "lebanon", Label: "Libanon", Continent: "Asien", URL: geofabrikBase + "asia/lebanon-latest.osm.pbf", SizeMB: 60, MinLon: 35.10, MinLat: 33.05, MaxLon: 36.62, MaxLat: 34.69},
	{ID: "jordan", Label: "Jordanien", Continent: "Asien", URL: geofabrikBase + "asia/jordan-latest.osm.pbf", SizeMB: 150, MinLon: 34.96, MinLat: 29.19, MaxLon: 39.30, MaxLat: 33.38},
	{ID: "israel-and-palestine", Label: "Israel & Palästina", Continent: "Asien", URL: geofabrikBase + "asia/israel-and-palestine-latest.osm.pbf", SizeMB: 250, MinLon: 34.27, MinLat: 29.49, MaxLon: 35.90, MaxLat: 33.29},
	{ID: "gcc-states", Label: "Golfstaaten", Continent: "Asien", URL: geofabrikBase + "asia/gcc-states-latest.osm.pbf", SizeMB: 500, MinLon: 39.00, MinLat: 16.37, MaxLon: 59.84, MaxLat: 32.16},
	{ID: "yemen", Label: "Jemen", Continent: "Asien", URL: geofabrikBase + "asia/yemen-latest.osm.pbf", SizeMB: 140, MinLon: 42.55, MinLat: 12.11, MaxLon: 54.53, MaxLat: 19.00},
	// Nordamerika
	{ID: "us-northeast", Label: "USA Nordost", Continent: "Nordamerika", URL: geofabrikBase + "north-america/us/northeast-latest.osm.pbf", SizeMB: 1700, MinLon: -80.52, MinLat: 38.79, MaxLon: -66.95, MaxLat: 47.46},
	{ID: "us-south", Label: "USA Süd", Continent: "Nordamerika", URL: geofabrikBase + "north-america/us/south-latest.osm.pbf", SizeMB: 1600, MinLon: -106.65, MinLat: 24.52, MaxLon: -75.46, MaxLat: 39.47},
	{ID: "us-midwest", Label: "USA Mitte", Continent: "Nordamerika", URL: geofabrikBase + "north-america/us/midwest-latest.osm.pbf", SizeMB: 1700, MinLon: -104.06, MinLat: 36.99, MaxLon: -80.52, MaxLat: 49.38},
	{ID: "us-west", Label: "USA West", Continent: "Nordamerika", URL: geofabrikBase + "north-america/us/west-latest.osm.pbf", SizeMB: 2500, MinLon: -124.73, MinLat: 31.33, MaxLon: -102.04, MaxLat: 49.00},
	{ID: "us-alaska", Label: "Alaska", Continent: "Nordamerika", URL: geofabrikBase + "north-america/us/alaska-latest.osm.pbf", SizeMB: 400, MinLon: -179.15, MinLat: 51.21, MaxLon: -129.98, MaxLat: 71.39},
	{ID: "canada", Label: "Kanada", Continent: "Nordamerika", URL: geofabrikBase + "north-america/canada-latest.osm.pbf", SizeMB: 3400, MinLon: -141.00, MinLat: 41.68, MaxLon: -52.65, MaxLat: 83.11},
	{ID: "mexico", Label: "Mexiko", Continent: "Nordamerika", URL: geofabrikBase + "north-america/mexico-latest.osm.pbf", SizeMB: 1400, MinLon: -118.60, MinLat: 14.53, MaxLon: -86.70, MaxLat: 32.72},
	{ID: "central-america", Label: "Zentralamerika", Continent: "Nordamerika", URL: geofabrikBase + "central-america-latest.osm.pbf", SizeMB: 400, MinLon: -92.23, MinLat: 7.22, MaxLon: -77.16, MaxLat: 18.50},
	// Südamerika
	{ID: "brazil", Label: "Brasilien", Continent: "Südamerika", URL: geofabrikBase + "south-america/brazil-latest.osm.pbf", SizeMB: 5000, MinLon: -73.99, MinLat: -33.75, MaxLon: -34.79, MaxLat: 5.27},
	{ID: "argentina", Label: "Argentinien", Continent: "Südamerika", URL: geofabrikBase + "south-america/argentina-latest.osm.pbf", SizeMB: 1200, MinLon: -73.56, MinLat: -55.06, MaxLon: -53.64, MaxLat: -21.78},
	{ID: "chile", Label: "Chile", Continent: "Südamerika", URL: geofabrikBase + "south-america/chile-latest.osm.pbf", SizeMB: 550, MinLon: -109.45, MinLat: -55.98, MaxLon: -66.42, MaxLat: -17.50},
	{ID: "peru", Label: "Peru", Continent: "Südamerika", URL: geofabrikBase + "south-america/peru-latest.osm.pbf", SizeMB: 450, MinLon: -81.33, MinLat: -18.35, MaxLon: -68.65, MaxLat: -0.04},
	{ID: "colombia", Label: "Kolumbien", Continent: "Südamerika", URL: geofabrikBase + "south-america/colombia-latest.osm.pbf", SizeMB: 600, MinLon: -81.73, MinLat: -4.23, MaxLon: -66.87, MaxLat: 13.39},
	{ID: "bolivia", Label: "Bolivien", Continent: "Südamerika", URL: geofabrikBase + "south-america/bolivia-latest.osm.pbf", SizeMB: 250, MinLon: -69.64, MinLat: -22.90, MaxLon: -57.45, MaxLat: -9.68},
	{ID: "ecuador", Label: "Ecuador", Continent: "Südamerika", URL: geofabrikBase + "south-america/ecuador-latest.osm.pbf", SizeMB: 200, MinLon: -92.01, MinLat: -5.01, MaxLon: -75.19, MaxLat: 1.68},
	{ID: "venezuela", Label: "Venezuela", Continent: "Südamerika", URL: geofabrikBase + "south-america/venezuela-latest.osm.pbf", SizeMB: 400, MinLon: -73.38, MinLat: 0.65, MaxLon: -59.80, MaxLat: 12.20},
	// Afrika
	{ID: "morocco", Label: "Marokko", Continent: "Afrika", URL: geofabrikBase + "africa/morocco-latest.osm.pbf", SizeMB: 350, MinLon: -17.02, MinLat: 21.42, MaxLon: -1.01, MaxLat: 35.93},
	{ID: "egypt", Label: "Ägypten", Continent: "Afrika", URL: geofabrikBase + "africa/egypt-latest.osm.pbf", SizeMB: 350, MinLon: 24.70, MinLat: 22.00, MaxLon: 36.90, MaxLat: 31.67},
	{ID: "kenya", Label: "Kenia", Continent: "Afrika", URL: geofabrikBase + "africa/kenya-latest.osm.pbf", SizeMB: 400, MinLon: 33.91, MinLat: -4.68, MaxLon: 41.90, MaxLat: 5.51},
	{ID: "tanzania", Label: "Tansania", Continent: "Afrika", URL: geofabrikBase + "africa/tanzania-latest.osm.pbf", SizeMB: 350, MinLon: 29.33, MinLat: -11.75, MaxLon: 40.44, MaxLat: -0.99},
	{ID: "ethiopia", Label: "Äthiopien", Continent: "Afrika", URL: geofabrikBase + "africa/ethiopia-latest.osm.pbf", SizeMB: 300, MinLon: 32.99, MinLat: 3.40, MaxLon: 47.99, MaxLat: 14.89},
	{ID: "south-africa", Label: "Südafrika", Continent: "Afrika", URL: geofabrikBase + "africa/south-africa-and-lesotho-latest.osm.pbf", SizeMB: 800, MinLon: 16.45, MinLat: -34.84, MaxLon: 32.89, MaxLat: -22.13},
	{ID: "east-africa", Label: "Ostafrika (Gesamt)", Continent: "Afrika", URL: geofabrikBase + "africa/east-africa-latest.osm.pbf", SizeMB: 700, MinLon: 24.98, MinLat: -26.87, MaxLon: 51.42, MaxLat: 22.23},
	{ID: "north-africa", Label: "Nordafrika (Gesamt)", Continent: "Afrika", URL: geofabrikBase + "africa/north-africa-latest.osm.pbf", SizeMB: 600, MinLon: -17.54, MinLat: 15.00, MaxLon: 36.90, MaxLat: 37.34},
	// Ozeanien
	{ID: "australia", Label: "Australien", Continent: "Ozeanien", URL: geofabrikBase + "australia-oceania/australia-latest.osm.pbf", SizeMB: 1700, MinLon: 112.92, MinLat: -43.74, MaxLon: 153.64, MaxLat: -10.06},
	{ID: "new-zealand", Label: "Neuseeland", Continent: "Ozeanien", URL: geofabrikBase + "australia-oceania/new-zealand-latest.osm.pbf", SizeMB: 320, MinLon: 166.43, MinLat: -47.29, MaxLon: 178.58, MaxLat: -34.39},
}

// RegionByID gibt eine Region anhand der ID zurück.
func RegionByID(id string) (Region, bool) {
	for _, r := range Regions {
		if r.ID == id {
			return r, true
		}
	}
	return Region{}, false
}
