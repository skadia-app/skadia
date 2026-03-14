-- skadia-osm.lua — osm2pgsql Flex configuration
-- Phase 8c: Unified osm_pois, extended osm_ways, pistes/glaciers/wetlands

-- ============================================================
-- Table definitions
-- ============================================================

tables = {}

tables.pois = osm2pgsql.define_table({
    name = 'osm_pois',
    ids = { type = 'any', id_column = 'id' },
    columns = {
        { column = 'osm_id',   type = 'int8',     not_null = true },
        { column = 'name',     type = 'text' },
        { column = 'category', type = 'text',     not_null = true },
        { column = 'type',     type = 'text',     not_null = true },
        { column = 'subtype',  type = 'text' },
        { column = 'ele',      type = 'real' },
        { column = 'tags',     type = 'jsonb' },
        { column = 'geom',     type = 'geometry', projection = 4326, not_null = true },
    },
})

tables.ways = osm2pgsql.define_way_table('osm_ways', {
    { column = 'osm_id',           type = 'int8',    not_null = true },
    { column = 'name',             type = 'text' },
    { column = 'ref',              type = 'text' },
    { column = 'highway',          type = 'text',    not_null = true },
    { column = 'surface',          type = 'text' },
    { column = 'tracktype',        type = 'text' },
    { column = 'smoothness',       type = 'text' },
    { column = 'sac_scale',        type = 'text' },
    { column = 'mtb_scale',        type = 'text' },
    { column = 'mtb_scale_uphill', type = 'text' },
    { column = 'via_ferrata_scale',type = 'text' },
    { column = 'trail_visibility', type = 'text' },
    { column = 'width',            type = 'text' },
    { column = 'incline',          type = 'text' },
    { column = 'lanes',            type = 'int2' },
    { column = 'cycleway',         type = 'text' },
    { column = 'cycleway_left',    type = 'text' },
    { column = 'cycleway_right',   type = 'text' },
    { column = 'cycleway_both',    type = 'text' },
    { column = 'bicycle_road',     type = 'boolean' },
    { column = 'segregated',       type = 'boolean' },
    { column = 'sidewalk',         type = 'text' },
    { column = 'maxspeed',         type = 'text' },
    { column = 'motor_vehicle',    type = 'text' },
    { column = 'access',           type = 'text' },
    { column = 'foot',             type = 'text' },
    { column = 'bicycle',          type = 'text' },
    { column = 'horse',            type = 'text' },
    { column = 'ski',              type = 'text' },
    { column = 'oneway',           type = 'boolean' },
    { column = 'oneway_bicycle',   type = 'boolean' },
    { column = 'bridge',           type = 'boolean' },
    { column = 'tunnel',           type = 'boolean' },
    { column = 'ford',             type = 'boolean' },
    { column = 'lit',              type = 'boolean' },
    { column = 'winter_service',   type = 'boolean' },
    { column = 'network',          type = 'text' },
    { column = 'operator',         type = 'text' },
    { column = 'tags',             type = 'jsonb' },
    { column = 'geom',             type = 'linestring', not_null = true },
})

tables.lifts = osm2pgsql.define_way_table('osm_lifts', {
    { column = 'osm_id',    type = 'int8',    not_null = true },
    { column = 'name',      type = 'text' },
    { column = 'aerialway', type = 'text' },
    { column = 'geom',      type = 'linestring', not_null = true },
})

tables.waterways = osm2pgsql.define_way_table('osm_waterways', {
    { column = 'osm_id',    type = 'int8',    not_null = true },
    { column = 'name',      type = 'text' },
    { column = 'waterway',  type = 'text' },
    { column = 'geom',      type = 'linestring', not_null = true },
})

tables.pistes = osm2pgsql.define_way_table('osm_pistes', {
    { column = 'osm_id',     type = 'int8',    not_null = true },
    { column = 'name',       type = 'text' },
    { column = 'piste_type', type = 'text',    not_null = true },
    { column = 'difficulty', type = 'text' },
    { column = 'grooming',   type = 'text' },
    { column = 'lit',        type = 'boolean' },
    { column = 'tags',       type = 'jsonb' },
    { column = 'geom',       type = 'linestring', not_null = true },
})

tables.forests = osm2pgsql.define_area_table('osm_forests', {
    { column = 'osm_id',    type = 'int8',    not_null = true },
    { column = 'name',      type = 'text' },
    { column = 'landuse',   type = 'text' },
    { column = 'geom',      type = 'geometry', not_null = true },
})

tables.waters = osm2pgsql.define_area_table('osm_waters', {
    { column = 'osm_id',    type = 'int8',    not_null = true },
    { column = 'name',      type = 'text' },
    { column = 'water',     type = 'text' },
    { column = 'geom',      type = 'geometry', not_null = true },
})

tables.glaciers = osm2pgsql.define_area_table('osm_glaciers', {
    { column = 'osm_id',    type = 'int8',    not_null = true },
    { column = 'name',      type = 'text' },
    { column = 'geom',      type = 'geometry', not_null = true },
})

tables.wetlands = osm2pgsql.define_area_table('osm_wetlands', {
    { column = 'osm_id',       type = 'int8',    not_null = true },
    { column = 'name',         type = 'text' },
    { column = 'wetland_type', type = 'text' },
    { column = 'geom',         type = 'geometry', not_null = true },
})

tables.boundaries = osm2pgsql.define_area_table('osm_boundaries', {
    { column = 'osm_id',         type = 'int8',    not_null = true },
    { column = 'name',           type = 'text' },
    { column = 'boundary_type',  type = 'text' },
    { column = 'admin_level',    type = 'int2' },
    { column = 'protect_class',  type = 'text' },
    { column = 'geom',           type = 'geometry', not_null = true },
})

tables.routes = osm2pgsql.define_relation_table('osm_routes', {
    { column = 'osm_id',      type = 'int8',    not_null = true },
    { column = 'name',        type = 'text' },
    { column = 'ref',         type = 'text' },
    { column = 'route',       type = 'text',    not_null = true },
    { column = 'network',     type = 'text' },
    { column = 'operator',    type = 'text' },
    { column = 'colour',      type = 'text' },
    { column = 'osmc_symbol', type = 'text' },
    { column = 'distance',    type = 'real' },
    { column = 'ascent',      type = 'int4' },
    { column = 'descent',     type = 'int4' },
    { column = 'description', type = 'text' },
    { column = 'website',     type = 'text' },
    { column = 'tags',        type = 'jsonb' },
    { column = 'geom',        type = 'multilinestring', not_null = true },
})

-- Route-Membership Junction-Table (non-geometry, populated via process_relation)
-- In --create mode osm2pgsql drops/recreates this table; migration 000157/000158
-- ensures it exists between imports for queries.
tables.route_members = osm2pgsql.define_table({
    name = 'osm_route_way_members',
    columns = {
        { column = 'route_osm_id', type = 'int8',  not_null = true },
        { column = 'way_osm_id',   type = 'int8',  not_null = true },
        { column = 'role',         type = 'text' },
        { column = 'source',       type = 'text',  not_null = true },
    },
})

-- ============================================================
-- Import flags (env vars)
-- ============================================================

local import_peaks         = os.getenv("SKADIA_IMPORT_PEAKS")         ~= nil
local import_passes        = os.getenv("SKADIA_IMPORT_PASSES")        ~= nil
local import_lifts         = os.getenv("SKADIA_IMPORT_LIFTS")         ~= nil
local import_settlements   = os.getenv("SKADIA_IMPORT_SETTLEMENTS")   ~= nil
local import_gastronomy    = os.getenv("SKADIA_IMPORT_GASTRONOMY")    ~= nil
local import_accommodation = os.getenv("SKADIA_IMPORT_ACCOMMODATION") ~= nil
local import_infrastructure= os.getenv("SKADIA_IMPORT_INFRASTRUCTURE")~= nil
local import_transport     = os.getenv("SKADIA_IMPORT_TRANSPORT")     ~= nil
local import_viewpoints    = os.getenv("SKADIA_IMPORT_VIEWPOINTS")    ~= nil
local import_shops         = os.getenv("SKADIA_IMPORT_SHOPS")         ~= nil
local import_sport         = os.getenv("SKADIA_IMPORT_SPORT")         ~= nil
local import_camping       = os.getenv("SKADIA_IMPORT_CAMPING")       ~= nil
local import_information   = os.getenv("SKADIA_IMPORT_INFORMATION")   ~= nil
local import_landmarks     = os.getenv("SKADIA_IMPORT_LANDMARKS")     ~= nil
local import_religion      = os.getenv("SKADIA_IMPORT_RELIGION")      ~= nil
local import_ways          = os.getenv("SKADIA_IMPORT_WAYS")          ~= nil
local import_waterways     = os.getenv("SKADIA_IMPORT_WATERWAYS")     ~= nil
local import_forests       = os.getenv("SKADIA_IMPORT_FORESTS")       ~= nil
local import_glaciers      = os.getenv("SKADIA_IMPORT_GLACIERS")      ~= nil
local import_wetlands      = os.getenv("SKADIA_IMPORT_WETLANDS")      ~= nil
local import_boundaries    = os.getenv("SKADIA_IMPORT_BOUNDARIES")    ~= nil
local import_pistes        = os.getenv("SKADIA_IMPORT_PISTES")        ~= nil
local import_ferries       = os.getenv("SKADIA_IMPORT_FERRIES")       ~= nil
local import_climbing      = os.getenv("SKADIA_IMPORT_CLIMBING")      ~= nil
local import_routes        = os.getenv("SKADIA_IMPORT_ROUTES")        ~= nil

-- If NO flag is set → import everything (backward compat for shell scripts)
local import_all = not (import_peaks or import_passes or import_lifts or
    import_settlements or import_gastronomy or import_accommodation or
    import_infrastructure or import_transport or import_viewpoints or
    import_shops or import_sport or import_camping or import_information or
    import_landmarks or import_religion or import_ways or import_waterways or
    import_forests or import_glaciers or import_wetlands or
    import_boundaries or import_pistes or import_ferries or import_climbing or
    import_routes)

-- ============================================================
-- Helper functions
-- ============================================================

local function parse_ele(s)
    if s == nil then return nil end
    return tonumber(s)
end

local function build_tags(tags, keys)
    local result = {}
    local has_any = false
    for _, k in ipairs(keys) do
        if tags[k] ~= nil then
            result[k] = tags[k]
            has_any = true
        end
    end
    if has_any then return result else return nil end
end

-- ============================================================
-- Highway whitelist for osm_ways
-- ============================================================

local routable_highways = {
    path=true, footway=true, cycleway=true, track=true, bridleway=true,
    steps=true, pedestrian=true, living_street=true,
    residential=true, unclassified=true, service=true,
    tertiary=true, tertiary_link=true,
    secondary=true, secondary_link=true,
    primary=true, primary_link=true,
    trunk=true, trunk_link=true,
    motorway=true, motorway_link=true,
}

-- ============================================================
-- classify_poi: shared POI classification for nodes, ways, relations
-- Returns a list of match tables: { {category, poi_type, subtype, ele_val, extra_tags}, ... }
-- Returns empty table if no POI match.
--
-- Multi-Match-Strategie (spiegelt process_node-Verhalten):
-- peaks/passes/settlements/gastronomy → early return (exklusiv, max 1 Match)
-- Alle übrigen Kategorien → separate if-Blöcke, können mehrfach matchen
-- ============================================================

function classify_poi(tags)
    local results = {}

    -- Peaks (natural=peak/volcano/saddle) — kein Name nötig
    if (import_all or import_peaks) and
       (tags.natural == 'peak' or tags.natural == 'volcano' or
        tags.natural == 'saddle') then
        table.insert(results, {
            category   = 'peak',
            poi_type   = 'natural',
            subtype    = tags.natural,
            ele_val    = parse_ele(tags.ele),
            extra_tags = build_tags(tags, {'prominence','wikidata','wikipedia'}),
        })
        return results  -- peaks are exclusive
    end

    -- Mountain Passes
    if (import_all or import_passes) and tags.mountain_pass == 'yes' then
        table.insert(results, {
            category   = 'pass',
            poi_type   = 'highway',
            subtype    = tags.highway or 'road',
            ele_val    = parse_ele(tags.ele),
            extra_tags = build_tags(tags, {'ref','wikidata'}),
        })
        return results  -- passes are exclusive
    end

    -- Settlements
    if (import_all or import_settlements) then
        local place_types = {city=true, town=true, village=true, hamlet=true, suburb=true, neighbourhood=true, isolated_dwelling=true}
        if tags.place and place_types[tags.place] and tags.name then
            table.insert(results, {
                category   = 'settlement',
                poi_type   = 'place',
                subtype    = tags.place,
                ele_val    = parse_ele(tags.ele),
                extra_tags = build_tags(tags, {'population','wikidata'}),
            })
            return results  -- settlements are exclusive
        end
    end

    -- Gastronomy (benötigt Name)
    if (import_all or import_gastronomy) then
        local gastro_types = {
            restaurant=true, cafe=true, pub=true, biergarten=true,
            fast_food=true, ice_cream=true, bar=true, food_court=true
        }
        if tags.amenity and gastro_types[tags.amenity] and tags.name then
            table.insert(results, {
                category   = 'gastronomy',
                poi_type   = 'amenity',
                subtype    = tags.amenity,
                extra_tags = build_tags(tags, {'cuisine','outdoor_seating','opening_hours','wheelchair'}),
            })
            return results  -- gastronomy returns early
        end
    end

    -- Accommodation (Hotels benötigen Name, Hütten immer)
    if (import_all or import_accommodation) then
        local accomm_types = {hotel=true, hostel=true, motel=true, guest_house=true}
        if tags.tourism and accomm_types[tags.tourism] and tags.name then
            table.insert(results, {
                category   = 'accommodation',
                poi_type   = 'tourism',
                subtype    = tags.tourism,
                extra_tags = build_tags(tags, {'rooms','stars','opening_hours'}),
            })
        elseif tags.tourism == 'alpine_hut' or tags.tourism == 'wilderness_hut' then
            table.insert(results, {
                category   = 'accommodation',
                poi_type   = 'tourism',
                subtype    = tags.tourism,
                ele_val    = parse_ele(tags.ele),
                extra_tags = build_tags(tags, {'beds','opening_hours','phone','website'}),
            })
        elseif tags.amenity == 'shelter' then
            table.insert(results, {
                category = 'shelter',
                poi_type = 'amenity',
                subtype  = tags.shelter_type or 'shelter',
            })
        end
    end

    -- Infrastructure: Trinkwasser, WC, Fahrradreparatur, Fahrradparken
    if (import_all or import_infrastructure) then
        if tags.amenity == 'drinking_water' then
            table.insert(results, {
                category = 'water_supply', poi_type = 'amenity', subtype = 'drinking_water',
                extra_tags = build_tags(tags, {'seasonal','bottle','access','fee'}),
            })
        elseif tags.natural == 'spring' then
            table.insert(results, {
                category = 'water_supply', poi_type = 'natural', subtype = 'spring',
                extra_tags = build_tags(tags, {'drinking_water','seasonal'}),
            })
        elseif tags.amenity == 'toilets' then
            table.insert(results, {
                category = 'infrastructure', poi_type = 'amenity', subtype = 'toilets',
                extra_tags = build_tags(tags, {'access','fee','wheelchair'}),
            })
        elseif tags.amenity == 'bicycle_repair_station' then
            table.insert(results, {
                category = 'infrastructure', poi_type = 'amenity', subtype = 'bicycle_repair_station',
                extra_tags = build_tags(tags, {'tools','pump'}),
            })
        elseif tags.amenity == 'bicycle_parking' then
            table.insert(results, {
                category = 'infrastructure', poi_type = 'amenity', subtype = 'bicycle_parking',
                extra_tags = build_tags(tags, {'capacity','covered','access','bicycle_parking'}),
            })
        elseif tags.amenity == 'compressed_air' then
            table.insert(results, {
                category = 'infrastructure', poi_type = 'amenity', subtype = 'compressed_air',
                extra_tags = build_tags(tags, {'fee','access'}),
            })
        elseif tags.amenity == 'charging_station' then
            table.insert(results, {
                category = 'infrastructure', poi_type = 'amenity', subtype = 'charging_station',
            })
        end
    end

    -- Transport
    if (import_all or import_transport) then
        if (tags.highway == 'bus_stop' or tags.railway == 'station' or
            tags.railway == 'halt' or tags.railway == 'tram_stop' or
            tags.amenity == 'bus_station' or tags.amenity == 'ferry_terminal' or
            tags.aerialway == 'station') and tags.name then
            local subtype = tags.highway or tags.railway or tags.aerialway or tags.amenity
            local ptype = tags.highway and 'highway' or
                          (tags.railway and 'railway' or
                           (tags.aerialway and 'aerialway' or 'amenity'))
            table.insert(results, {
                category = 'transport', poi_type = ptype, subtype = subtype,
                extra_tags = build_tags(tags, {'network','operator','ref'}),
            })
        end
    end

    -- Parking
    if (import_all or import_infrastructure) then
        if tags.amenity == 'parking' then
            table.insert(results, {
                category = 'parking', poi_type = 'amenity', subtype = tags.parking or 'surface',
                extra_tags = build_tags(tags, {'capacity','fee','access'}),
            })
        end
    end

    -- Viewpoints + Towers
    if (import_all or import_viewpoints) then
        if tags.tourism == 'viewpoint' then
            table.insert(results, {
                category = 'viewpoint', poi_type = 'tourism', subtype = 'viewpoint',
            })
        elseif (tags.man_made == 'tower' and tags['tower:type'] == 'observation') or
               tags.man_made == 'observation_tower' then
            if tags.name then
                table.insert(results, {
                    category = 'viewpoint', poi_type = 'man_made', subtype = 'tower',
                    extra_tags = build_tags(tags, {'tower:type','access'}),
                })
            end
        end
    end

    -- Shops
    if (import_all or import_shops) then
        local shop_types = {
            supermarket=true, convenience=true, bakery=true, butcher=true,
            outdoor=true, sports=true, bicycle=true
        }
        if tags.shop and shop_types[tags.shop] and tags.name then
            table.insert(results, {
                category = 'shop', poi_type = 'shop', subtype = tags.shop,
                extra_tags = build_tags(tags, {'opening_hours','wheelchair'}),
            })
        end
    end

    -- Sport & Klettern
    if (import_all or import_sport) then
        local leisure_sport_types = {
            fitness_station=true, sports_centre=true, swimming_pool=true,
            pitch=true, golf_course=true
        }
        if tags.sport == 'climbing' and tags.name then
            table.insert(results, {
                category = 'sport', poi_type = 'sport', subtype = 'climbing',
                extra_tags = build_tags(tags, {'climbing:grade:uiaa','rock','fee'}),
            })
        elseif tags.leisure and leisure_sport_types[tags.leisure] and tags.name then
            table.insert(results, {
                category = 'sport', poi_type = 'leisure', subtype = tags.leisure,
                extra_tags = build_tags(tags, {'sport','access'}),
            })
        end
    end

    -- Camping, Feuerstellen, Picknick
    if (import_all or import_camping) then
        if (tags.tourism == 'camp_site' or tags.tourism == 'caravan_site') and tags.name then
            table.insert(results, {
                category = 'camping', poi_type = 'tourism', subtype = tags.tourism,
                extra_tags = build_tags(tags, {'tents','fee','shower'}),
            })
        elseif tags.amenity == 'bbq' or tags.leisure == 'firepit' then
            table.insert(results, {
                category = 'camping',
                poi_type = tags.amenity and 'amenity' or 'leisure',
                subtype  = tags.amenity or tags.leisure,
            })
        elseif tags.leisure == 'picnic_table' or tags.tourism == 'picnic_site' then
            table.insert(results, {
                category = 'camping',
                poi_type = tags.leisure and 'leisure' or 'tourism',
                subtype  = 'picnic',
            })
        end
    end

    -- Information
    if (import_all or import_information) then
        local info_types = {guidepost=true, board=true, map=true, office=true}
        if tags.information and info_types[tags.information] then
            table.insert(results, {
                category = 'information', poi_type = 'tourism', subtype = tags.information,
            })
        end
    end

    -- Landmarks
    if (import_all or import_landmarks) then
        if tags.waterway == 'waterfall' then
            table.insert(results, {
                category = 'landmark', poi_type = 'waterway', subtype = 'waterfall',
            })
        elseif tags.natural == 'hot_spring' then
            table.insert(results, {
                category = 'landmark', poi_type = 'natural', subtype = 'hot_spring',
            })
        elseif tags.man_made == 'cross' or tags.historic == 'wayside_cross' then
            table.insert(results, {
                category = 'landmark',
                poi_type = tags.man_made and 'man_made' or 'historic',
                subtype  = 'cross',
            })
        elseif tags.man_made == 'survey_point' then
            table.insert(results, {
                category = 'landmark', poi_type = 'man_made', subtype = 'survey_point',
                ele_val  = parse_ele(tags.ele),
            })
        elseif tags.historic and tags.historic ~= 'yes' and tags.name then
            table.insert(results, {
                category = 'landmark', poi_type = 'historic', subtype = tags.historic,
            })
        end
    end

    -- Religion
    if (import_all or import_religion) then
        if tags.amenity == 'place_of_worship' or tags.building == 'chapel' or
           tags.historic == 'wayside_shrine' then
            if tags.name then
                table.insert(results, {
                    category = 'religious',
                    poi_type = tags.amenity and 'amenity' or (tags.building and 'building' or 'historic'),
                    subtype  = tags.religion or tags.amenity or 'shrine',
                })
            end
        end
    end

    -- Barrier
    if (import_all or import_infrastructure) then
        local barrier_types = {gate=true, lift_gate=true, cattle_grid=true, bollard=true, cycle_barrier=true}
        if tags.barrier and barrier_types[tags.barrier] then
            table.insert(results, {
                category   = 'barrier', poi_type = 'barrier', subtype = tags.barrier,
                extra_tags = build_tags(tags, {'access','bicycle','foot','locked'}),
            })
        end
    end

    return results
end

-- Insert all POI matches for a given OSM object
function insert_poi_matches(matches, osm_id, name, geom)
    for _, m in ipairs(matches) do
        tables.pois:insert({
            osm_id   = osm_id,
            name     = name,
            category = m.category,
            type     = m.poi_type,
            subtype  = m.subtype,
            ele      = m.ele_val,
            tags     = m.extra_tags,
            geom     = geom,
        })
    end
end

-- ============================================================
-- process_node: POIs
-- ============================================================

function osm2pgsql.process_node(object)
    local matches = classify_poi(object.tags)
    if #matches > 0 then
        insert_poi_matches(matches, object.id, object.tags.name, object:as_point())
    end
end

-- ============================================================
-- process_way: Wege, Lifte, Gewässer, Pisten
-- ============================================================

function osm2pgsql.process_way(object)
    local tags = object.tags

    -- Wege (Routing-Netzwerk)
    if (import_all or import_ways) then
        local hw = tags.highway
        if hw and routable_highways[hw] then
            local lanes_val = nil
            if tags.lanes then
                local n = tonumber(tags.lanes)
                if n and n > 0 then lanes_val = math.floor(n) end
            end
            tables.ways:insert({
                osm_id            = object.id,
                name              = tags.name,
                ref               = tags.ref,
                highway           = hw,
                surface           = tags.surface,
                tracktype         = tags.tracktype,
                smoothness        = tags.smoothness,
                sac_scale         = tags.sac_scale,
                mtb_scale         = tags['mtb:scale'],
                mtb_scale_uphill  = tags['mtb:scale:uphill'],
                via_ferrata_scale = tags.via_ferrata_scale,
                trail_visibility  = tags.trail_visibility,
                width             = tags.width,
                incline           = tags.incline,
                lanes             = lanes_val,
                cycleway          = tags.cycleway,
                cycleway_left     = tags['cycleway:left'],
                cycleway_right    = tags['cycleway:right'],
                cycleway_both     = tags['cycleway:both'],
                bicycle_road      = (tags.bicycle_road == 'yes' or tags.cyclestreet == 'yes') or false,
                segregated        = (tags.segregated == 'yes') or false,
                sidewalk          = tags.sidewalk,
                maxspeed          = tags.maxspeed,
                motor_vehicle     = tags.motor_vehicle,
                access            = tags.access,
                foot              = tags.foot,
                bicycle           = tags.bicycle,
                horse             = tags.horse,
                ski               = tags.ski,
                oneway            = (tags.oneway == 'yes' or tags.oneway == '1') or false,
                oneway_bicycle    = (tags['oneway:bicycle'] ~= 'no' and tags.oneway == 'yes') or false,
                bridge            = (tags.bridge == 'yes') or false,
                tunnel            = (tags.tunnel == 'yes') or false,
                ford              = (tags.ford == 'yes') or false,
                lit               = (tags.lit == 'yes') or false,
                winter_service    = (tags.winter_service == 'yes') or false,
                network           = tags.network,
                operator          = tags.operator,
                tags              = build_tags(tags, {'name:de','name:en','alt_name','description'}),
                geom              = object:as_linestring(),
            })
        end
    end

    -- Kletterrouten & Via Ferrata (highway=via_ferrata auf Ways)
    -- TODO: Mehrteilige Via-Ferrata-Routen in OSM sind oft als Relationen (type=route,
    -- route=via_ferrata) modelliert. process_relation unterstützt das aktuell nicht.
    if (import_all or import_ways or import_climbing) then
        if tags.highway == 'via_ferrata' then
            tables.ways:insert({
                osm_id            = object.id,
                name              = tags.name,
                ref               = tags.ref,
                highway           = 'via_ferrata',
                surface           = tags.surface,
                via_ferrata_scale = tags.via_ferrata_scale,
                sac_scale         = tags.sac_scale,
                trail_visibility  = tags.trail_visibility,
                access            = tags.access,
                foot              = tags.foot,
                tags              = build_tags(tags, {'name:de','name:en','alt_name','description','climbing:grade:uiaa','operator'}),
                geom              = object:as_linestring(),
            })
        end
    end

    -- Fährrouten (route=ferry auf Ways)
    -- TODO: Viele Fährlinien in OSM sind als Relationen (type=route, route=ferry) modelliert
    -- und werden hier nicht erfasst. process_relation unterstützt aktuell keine Fähr-Relationen.
    if (import_all or import_ferries) then
        if tags.route == 'ferry' then
            tables.ways:insert({
                osm_id   = object.id,
                name     = tags.name,
                ref      = tags.ref,
                highway  = 'ferry',
                operator = tags.operator,
                access   = tags.access,
                foot     = tags.foot,
                bicycle  = tags.bicycle,
                motor_vehicle = tags.motor_vehicle,
                tags     = build_tags(tags, {'name:de','name:en','alt_name','description','duration'}),
                geom     = object:as_linestring(),
            })
        end
    end

    -- Lifte & Seilbahnen
    if (import_all or import_lifts) then
        local aerialway_types = {
            gondola=true, cable_car=true, chair_lift=true, mixed_lift=true, drag_lift=true,
            ["t-bar"]=true, ["j-bar"]=true, platter=true, rope_tow=true, magic_carpet=true,
            goods=true, zip_line=true
        }
        if tags.aerialway and aerialway_types[tags.aerialway] then
            tables.lifts:insert({
                osm_id    = object.id,
                name      = tags.name,
                aerialway = tags.aerialway,
                geom      = object:as_linestring(),
            })
        end
    end

    -- Gewässer (Flüsse, Bäche)
    if (import_all or import_waterways) then
        local waterway_types = {river=true, stream=true, canal=true, drain=true}
        if tags.waterway and waterway_types[tags.waterway] then
            tables.waterways:insert({
                osm_id   = object.id,
                name     = tags.name,
                waterway = tags.waterway,
                geom     = object:as_linestring(),
            })
        end
    end

    -- Skipisten
    if (import_all or import_pistes) then
        local piste_types = {downhill=true, nordic=true, skitour=true, sled=true, hike=true, sleigh=true, ice_skate=true, snow_park=true, playground=true}
        if tags['piste:type'] and piste_types[tags['piste:type']] then
            tables.pistes:insert({
                osm_id     = object.id,
                name       = tags.name,
                piste_type = tags['piste:type'],
                difficulty = tags['piste:difficulty'],
                grooming   = tags['piste:grooming'],
                lit        = (tags.lit == 'yes') or false,
                tags       = build_tags(tags, {'ref','operator'}),
                geom       = object:as_linestring(),
            })
        end
    end

    -- Wälder & Natur (Flächen als geschlossene Ways)
    if (import_all or import_forests) then
        if (tags.landuse == 'forest' or tags.landuse == 'wood' or tags.natural == 'wood') and
           object.is_closed then
            tables.forests:insert({
                osm_id  = object.id,
                name    = tags.name,
                landuse = tags.landuse or tags.natural,
                geom    = object:as_polygon(),
            })
        end
    end

    -- Seen & Gewässerflächen (geschlossene Ways)
    if (import_all or import_waterways) then
        if (tags.natural == 'water' or tags.landuse == 'reservoir' or tags.waterway == 'pond') and
           object.is_closed then
            tables.waters:insert({
                osm_id = object.id,
                name   = tags.name,
                water  = tags.water or tags.natural or 'water',
                geom   = object:as_polygon(),
            })
        end
    end

    -- Gletscher (geschlossene Ways)
    if (import_all or import_glaciers) then
        if tags.natural == 'glacier' and object.is_closed then
            tables.glaciers:insert({
                osm_id = object.id,
                name   = tags.name,
                geom   = object:as_polygon(),
            })
        end
    end

    -- Feuchtgebiete (geschlossene Ways)
    if (import_all or import_wetlands) then
        if tags.natural == 'wetland' and object.is_closed then
            local wetland_types = {bog=true, marsh=true, swamp=true, fen=true, reedbed=true, wet_meadow=true, mangrove=true}
            tables.wetlands:insert({
                osm_id       = object.id,
                name         = tags.name,
                wetland_type = (tags.wetland and wetland_types[tags.wetland]) and tags.wetland or nil,
                geom         = object:as_polygon(),
            })
        end
    end

    -- POIs als geschlossene Ways (Gebäude, Flächen)
    if object.is_closed and object.tags.area ~= 'no' then
        local matches = classify_poi(object.tags)
        if #matches > 0 then
            insert_poi_matches(matches, object.id, object.tags.name, object:as_polygon())
        end
    end
end

-- ============================================================
-- Helpers
-- ============================================================

local function parse_distance(s)
    -- OSM distance tags: "42", "42.5", "42 km", "42km"
    if s == nil then return nil end
    return tonumber(s:match("^%s*([%d%.]+)"))
end

local function parse_elevation_int(s)
    if s == nil then return nil end
    local n = tonumber(s:match("^%s*(%d+)"))
    if n then return math.floor(n) end
    return nil
end

local valid_route_types = {
    hiking=true, bicycle=true, mtb=true, foot=true,
    ski=true, running=true, ferry=true, via_ferrata=true,
    horse=true,
}

-- ============================================================
-- process_relation: Grenzen, Flächen, Routen
-- ============================================================

function osm2pgsql.process_relation(object)
    local tags = object.tags

    -- Grenzen (administrative + Nationalparks + Schutzgebiete)
    if (import_all or import_boundaries) then
        if tags.type == 'boundary' and tags.boundary == 'administrative' then
            local level = tonumber(tags.admin_level)
            if level and level >= 2 and level <= 8 then
                local mp = object:as_multipolygon()
                if mp then
                    tables.boundaries:insert({
                        osm_id        = object.id,
                        name          = tags.name,
                        boundary_type = 'administrative',
                        admin_level   = level,
                        geom          = mp,
                    })
                end
            end
        elseif tags.type == 'boundary' and
               (tags.boundary == 'national_park' or tags.boundary == 'protected_area') and
               tags.name then
            local mp = object:as_multipolygon()
            if mp then
                tables.boundaries:insert({
                    osm_id        = object.id,
                    name          = tags.name,
                    boundary_type = tags.boundary,
                    protect_class = tags.protect_class,
                    geom          = mp,
                })
            end
        end
    end

    -- Wälder als Relationen
    if (import_all or import_forests) then
        if tags.type == 'multipolygon' and
           (tags.landuse == 'forest' or tags.landuse == 'wood' or tags.natural == 'wood') then
            local mp = object:as_multipolygon()
            if mp then
                tables.forests:insert({
                    osm_id  = object.id,
                    name    = tags.name,
                    landuse = tags.landuse or tags.natural,
                    geom    = mp,
                })
            end
        end
    end

    -- Seen & Gewässer als Relationen
    if (import_all or import_waterways) then
        if tags.type == 'multipolygon' and
           (tags.natural == 'water' or tags.landuse == 'reservoir') then
            local mp = object:as_multipolygon()
            if mp then
                tables.waters:insert({
                    osm_id = object.id,
                    name   = tags.name,
                    water  = tags.water or tags.natural or 'water',
                    geom   = mp,
                })
            end
        end
    end

    -- Gletscher
    if (import_all or import_glaciers) then
        if tags.type == 'multipolygon' and tags.natural == 'glacier' then
            local mp = object:as_multipolygon()
            if mp then
                tables.glaciers:insert({
                    osm_id = object.id,
                    name   = tags.name,
                    geom   = mp,
                })
            end
        end
    end

    -- Feuchtgebiete
    if (import_all or import_wetlands) then
        if tags.type == 'multipolygon' and tags.natural == 'wetland' then
            local mp = object:as_multipolygon()
            if mp then
                tables.wetlands:insert({
                    osm_id       = object.id,
                    name         = tags.name,
                    wetland_type = tags.wetland,
                    geom         = mp,
                })
            end
        end
    end

    -- Pisten als Relationen
    if (import_all or import_pistes) then
        local piste_types = {downhill=true, nordic=true, skitour=true, sled=true, hike=true, sleigh=true, ice_skate=true, snow_park=true, playground=true}
        if tags.type == 'multipolygon' and tags['piste:type'] and piste_types[tags['piste:type']] then
            -- TODO Phase 8c-II: import piste relation outlines as area
        end
    end

    -- Benannte Routen-Relationen (Wanderwege, Radrouten, MTB, Ski, Fähren, Via Ferrata)
    if (import_all or import_routes) then
        if tags.type == 'route' then
            local route_type = tags.route
            if route_type and valid_route_types[route_type] then
                -- Nur benannte Routen importieren — anonyme Connector-Relationen
                -- (keine Identität, duplizieren nur Geometrie) ausfiltern
                if tags.name or tags.ref then
                    local ml = object:as_multilinestring()
                    if ml then
                        tables.routes:insert({
                            osm_id      = object.id,
                            name        = tags.name,
                            ref         = tags.ref,
                            route       = route_type,
                            network     = tags.network,
                            operator    = tags.operator,
                            colour      = tags.colour,
                            osmc_symbol = tags['osmc:symbol'],
                            distance    = parse_distance(tags.distance),
                            ascent      = parse_elevation_int(tags.ascent),
                            descent     = parse_elevation_int(tags.descent),
                            description = tags.description,
                            website     = tags.website,
                            tags        = build_tags(tags, {
                                'from', 'to', 'wikidata', 'wikipedia',
                                'short_name', 'alt_name', 'roundtrip',
                                'duration', 'difficulty',
                            }),
                            geom        = ml,
                        })
                        -- Extract member way IDs into junction table
                        for _, member in ipairs(object.members) do
                            if member.type == 'w' then
                                tables.route_members:insert({
                                    route_osm_id = object.id,
                                    way_osm_id   = member.ref,
                                    role         = member.role,
                                    source       = 'relation',
                                })
                            end
                        end
                    end
                end
            end
        end
    end

    -- POIs als Multipolygon-Relationen (Campingplätze, Sportanlagen, Kirchen-Komplexe)
    if object.tags.type == 'multipolygon' then
        local matches = classify_poi(object.tags)
        if #matches > 0 then
            local mp = object:as_multipolygon()
            if mp then  -- nil bei broken Relations (fehlende Members)
                insert_poi_matches(matches, -object.id, object.tags.name, mp)
            end
        end
    end
end
