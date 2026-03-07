package osm

// Region beschreibt eine downloadbare OSM-Region von GeoFabrik.
type Region struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Continent string `json:"continent"`
	URL       string `json:"url"`     // vollständige GeoFabrik-Download-URL
	SizeMB    int    `json:"size_mb"` // geschätzte PBF-Größe in MB
}

const geofabrikBase = "https://download.geofabrik.de/"

var Regions = []Region{
	// Europa — Westeuropa
	{ID: "germany", Label: "Deutschland", Continent: "Europa", URL: geofabrikBase + "europe/germany-latest.osm.pbf", SizeMB: 4000},
	{ID: "austria", Label: "Österreich", Continent: "Europa", URL: geofabrikBase + "europe/austria-latest.osm.pbf", SizeMB: 700},
	{ID: "switzerland", Label: "Schweiz", Continent: "Europa", URL: geofabrikBase + "europe/switzerland-latest.osm.pbf", SizeMB: 380},
	{ID: "liechtenstein", Label: "Liechtenstein", Continent: "Europa", URL: geofabrikBase + "europe/liechtenstein-latest.osm.pbf", SizeMB: 2},
	{ID: "france", Label: "Frankreich", Continent: "Europa", URL: geofabrikBase + "europe/france-latest.osm.pbf", SizeMB: 4200},
	{ID: "andorra", Label: "Andorra", Continent: "Europa", URL: geofabrikBase + "europe/andorra-latest.osm.pbf", SizeMB: 3},
	{ID: "monaco", Label: "Monaco", Continent: "Europa", URL: geofabrikBase + "europe/monaco-latest.osm.pbf", SizeMB: 1},
	{ID: "spain", Label: "Spanien", Continent: "Europa", URL: geofabrikBase + "europe/spain-latest.osm.pbf", SizeMB: 1800},
	{ID: "portugal", Label: "Portugal", Continent: "Europa", URL: geofabrikBase + "europe/portugal-latest.osm.pbf", SizeMB: 350},
	{ID: "italy", Label: "Italien", Continent: "Europa", URL: geofabrikBase + "europe/italy-latest.osm.pbf", SizeMB: 2800},
	{ID: "san-marino", Label: "San Marino", Continent: "Europa", URL: geofabrikBase + "europe/san-marino-latest.osm.pbf", SizeMB: 1},
	{ID: "great-britain", Label: "Großbritannien", Continent: "Europa", URL: geofabrikBase + "europe/great-britain-latest.osm.pbf", SizeMB: 1800},
	{ID: "ireland", Label: "Irland", Continent: "Europa", URL: geofabrikBase + "europe/ireland-and-northern-ireland-latest.osm.pbf", SizeMB: 200},
	{ID: "isle-of-man", Label: "Isle of Man", Continent: "Europa", URL: geofabrikBase + "europe/isle-of-man-latest.osm.pbf", SizeMB: 3},
	{ID: "netherlands", Label: "Niederlande", Continent: "Europa", URL: geofabrikBase + "europe/netherlands-latest.osm.pbf", SizeMB: 900},
	{ID: "belgium", Label: "Belgien", Continent: "Europa", URL: geofabrikBase + "europe/belgium-latest.osm.pbf", SizeMB: 400},
	{ID: "luxembourg", Label: "Luxemburg", Continent: "Europa", URL: geofabrikBase + "europe/luxembourg-latest.osm.pbf", SizeMB: 30},
	// Europa — Nordeuropa
	{ID: "norway", Label: "Norwegen", Continent: "Europa", URL: geofabrikBase + "europe/norway-latest.osm.pbf", SizeMB: 1000},
	{ID: "sweden", Label: "Schweden", Continent: "Europa", URL: geofabrikBase + "europe/sweden-latest.osm.pbf", SizeMB: 800},
	{ID: "denmark", Label: "Dänemark", Continent: "Europa", URL: geofabrikBase + "europe/denmark-latest.osm.pbf", SizeMB: 300},
	{ID: "finland", Label: "Finnland", Continent: "Europa", URL: geofabrikBase + "europe/finland-latest.osm.pbf", SizeMB: 350},
	{ID: "iceland", Label: "Island", Continent: "Europa", URL: geofabrikBase + "europe/iceland-latest.osm.pbf", SizeMB: 60},
	{ID: "faroe-islands", Label: "Färöer", Continent: "Europa", URL: geofabrikBase + "europe/faroe-islands-latest.osm.pbf", SizeMB: 4},
	// Europa — Mittel- und Osteuropa
	{ID: "poland", Label: "Polen", Continent: "Europa", URL: geofabrikBase + "europe/poland-latest.osm.pbf", SizeMB: 1600},
	{ID: "czech-republic", Label: "Tschechien", Continent: "Europa", URL: geofabrikBase + "europe/czech-republic-latest.osm.pbf", SizeMB: 650},
	{ID: "slovakia", Label: "Slowakei", Continent: "Europa", URL: geofabrikBase + "europe/slovakia-latest.osm.pbf", SizeMB: 200},
	{ID: "hungary", Label: "Ungarn", Continent: "Europa", URL: geofabrikBase + "europe/hungary-latest.osm.pbf", SizeMB: 400},
	{ID: "romania", Label: "Rumänien", Continent: "Europa", URL: geofabrikBase + "europe/romania-latest.osm.pbf", SizeMB: 600},
	{ID: "ukraine", Label: "Ukraine", Continent: "Europa", URL: geofabrikBase + "europe/ukraine-latest.osm.pbf", SizeMB: 650},
	{ID: "belarus", Label: "Belarus", Continent: "Europa", URL: geofabrikBase + "europe/belarus-latest.osm.pbf", SizeMB: 400},
	{ID: "moldova", Label: "Moldau", Continent: "Europa", URL: geofabrikBase + "europe/moldova-latest.osm.pbf", SizeMB: 60},
	{ID: "estonia", Label: "Estland", Continent: "Europa", URL: geofabrikBase + "europe/estonia-latest.osm.pbf", SizeMB: 60},
	{ID: "latvia", Label: "Lettland", Continent: "Europa", URL: geofabrikBase + "europe/latvia-latest.osm.pbf", SizeMB: 100},
	{ID: "lithuania", Label: "Litauen", Continent: "Europa", URL: geofabrikBase + "europe/lithuania-latest.osm.pbf", SizeMB: 100},
	// Europa — Südosteuropa
	{ID: "slovenia", Label: "Slowenien", Continent: "Europa", URL: geofabrikBase + "europe/slovenia-latest.osm.pbf", SizeMB: 100},
	{ID: "croatia", Label: "Kroatien", Continent: "Europa", URL: geofabrikBase + "europe/croatia-latest.osm.pbf", SizeMB: 250},
	{ID: "bosnia-herzegovina", Label: "Bosnien-Herzegowina", Continent: "Europa", URL: geofabrikBase + "europe/bosnia-herzegovina-latest.osm.pbf", SizeMB: 120},
	{ID: "serbia", Label: "Serbien", Continent: "Europa", URL: geofabrikBase + "europe/serbia-latest.osm.pbf", SizeMB: 250},
	{ID: "montenegro", Label: "Montenegro", Continent: "Europa", URL: geofabrikBase + "europe/montenegro-latest.osm.pbf", SizeMB: 50},
	{ID: "kosovo", Label: "Kosovo", Continent: "Europa", URL: geofabrikBase + "europe/kosovo-latest.osm.pbf", SizeMB: 50},
	{ID: "north-macedonia", Label: "Nordmazedonien", Continent: "Europa", URL: geofabrikBase + "europe/north-macedonia-latest.osm.pbf", SizeMB: 100},
	{ID: "albania", Label: "Albanien", Continent: "Europa", URL: geofabrikBase + "europe/albania-latest.osm.pbf", SizeMB: 80},
	{ID: "bulgaria", Label: "Bulgarien", Continent: "Europa", URL: geofabrikBase + "europe/bulgaria-latest.osm.pbf", SizeMB: 300},
	{ID: "greece", Label: "Griechenland", Continent: "Europa", URL: geofabrikBase + "europe/greece-latest.osm.pbf", SizeMB: 550},
	{ID: "turkey", Label: "Türkei", Continent: "Europa", URL: geofabrikBase + "europe/turkey-latest.osm.pbf", SizeMB: 750},
	{ID: "cyprus", Label: "Zypern", Continent: "Europa", URL: geofabrikBase + "europe/cyprus-latest.osm.pbf", SizeMB: 80},
	{ID: "malta", Label: "Malta", Continent: "Europa", URL: geofabrikBase + "europe/malta-latest.osm.pbf", SizeMB: 5},
	// Europa — Gesamt
	{ID: "russia", Label: "Russland ⚠️", Continent: "Europa", URL: geofabrikBase + "russia-latest.osm.pbf", SizeMB: 65000},
	{ID: "europe", Label: "Europa gesamt ⚠️", Continent: "Europa", URL: geofabrikBase + "europe-latest.osm.pbf", SizeMB: 28000},
	// Asien
	{ID: "japan", Label: "Japan", Continent: "Asien", URL: geofabrikBase + "asia/japan-latest.osm.pbf", SizeMB: 2000},
	{ID: "south-korea", Label: "Südkorea", Continent: "Asien", URL: geofabrikBase + "asia/south-korea-latest.osm.pbf", SizeMB: 550},
	{ID: "india", Label: "Indien", Continent: "Asien", URL: geofabrikBase + "asia/india-latest.osm.pbf", SizeMB: 2300},
	{ID: "china", Label: "China", Continent: "Asien", URL: geofabrikBase + "asia/china-latest.osm.pbf", SizeMB: 11000},
	{ID: "southeast-asia", Label: "Südostasien", Continent: "Asien", URL: geofabrikBase + "asia/southeast-asia-latest.osm.pbf", SizeMB: 2500},
	// Nordamerika
	{ID: "us-northeast", Label: "USA Nordost", Continent: "Nordamerika", URL: geofabrikBase + "north-america/us/northeast-latest.osm.pbf", SizeMB: 1700},
	{ID: "us-south", Label: "USA Süd", Continent: "Nordamerika", URL: geofabrikBase + "north-america/us/south-latest.osm.pbf", SizeMB: 1600},
	{ID: "us-midwest", Label: "USA Mitte", Continent: "Nordamerika", URL: geofabrikBase + "north-america/us/midwest-latest.osm.pbf", SizeMB: 1700},
	{ID: "us-west", Label: "USA West", Continent: "Nordamerika", URL: geofabrikBase + "north-america/us/west-latest.osm.pbf", SizeMB: 2500},
	{ID: "canada", Label: "Kanada", Continent: "Nordamerika", URL: geofabrikBase + "north-america/canada-latest.osm.pbf", SizeMB: 3400},
	{ID: "mexico", Label: "Mexiko", Continent: "Nordamerika", URL: geofabrikBase + "north-america/mexico-latest.osm.pbf", SizeMB: 1400},
	// Südamerika
	{ID: "brazil", Label: "Brasilien", Continent: "Südamerika", URL: geofabrikBase + "south-america/brazil-latest.osm.pbf", SizeMB: 5000},
	{ID: "argentina", Label: "Argentinien", Continent: "Südamerika", URL: geofabrikBase + "south-america/argentina-latest.osm.pbf", SizeMB: 1200},
	{ID: "chile", Label: "Chile", Continent: "Südamerika", URL: geofabrikBase + "south-america/chile-latest.osm.pbf", SizeMB: 550},
	// Afrika
	{ID: "south-africa", Label: "Südafrika", Continent: "Afrika", URL: geofabrikBase + "africa/south-africa-and-lesotho-latest.osm.pbf", SizeMB: 800},
	{ID: "east-africa", Label: "Ostafrika", Continent: "Afrika", URL: geofabrikBase + "africa/east-africa-latest.osm.pbf", SizeMB: 700},
	{ID: "north-africa", Label: "Nordafrika", Continent: "Afrika", URL: geofabrikBase + "africa/north-africa-latest.osm.pbf", SizeMB: 600},
	// Ozeanien
	{ID: "australia", Label: "Australien", Continent: "Ozeanien", URL: geofabrikBase + "australia-oceania/australia-latest.osm.pbf", SizeMB: 1700},
	{ID: "new-zealand", Label: "Neuseeland", Continent: "Ozeanien", URL: geofabrikBase + "australia-oceania/new-zealand-latest.osm.pbf", SizeMB: 320},
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
