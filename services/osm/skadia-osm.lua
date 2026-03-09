-- Skadia OSM data import script for osm2pgsql Flex mode
-- Usage: osm2pgsql -d skadia -S skadia-osm.lua -O flex <file.osm.pbf>

-- Table definitions

local tables = {}

tables.boundaries = osm2pgsql.define_area_table('osm_boundaries', {
    { column = 'osm_id', type = 'bigint' },
    { column = 'name', type = 'text' },
    { column = 'admin_level', type = 'int' },
    { column = 'country_code', type = 'text' },
    { column = 'geom', type = 'multipolygon', projection = 4326 },
})

tables.peaks = osm2pgsql.define_node_table('osm_peaks', {
    { column = 'osm_id', type = 'bigint' },
    { column = 'name', type = 'text' },
    { column = 'ele', type = 'real' },
    { column = 'type', type = 'text' },
    { column = 'geom', type = 'point', projection = 4326 },
})

tables.passes = osm2pgsql.define_node_table('osm_passes', {
    { column = 'osm_id', type = 'bigint' },
    { column = 'name',   type = 'text' },
    { column = 'ele',    type = 'real' },
    { column = 'geom',   type = 'point', projection = 4326 },
})

tables.forests = osm2pgsql.define_area_table('osm_forests', {
    { column = 'osm_id', type = 'bigint' },
    { column = 'name', type = 'text' },
    { column = 'geom', type = 'multipolygon', projection = 4326 },
})

tables.waters = osm2pgsql.define_area_table('osm_waters', {
    { column = 'osm_id', type = 'bigint' },
    { column = 'name', type = 'text' },
    { column = 'geom', type = 'multipolygon', projection = 4326 },
})

tables.waterways = osm2pgsql.define_way_table('osm_waterways', {
    { column = 'osm_id', type = 'bigint' },
    { column = 'name', type = 'text' },
    { column = 'waterway', type = 'text' },
    { column = 'geom', type = 'linestring', projection = 4326 },
})

tables.settlements = osm2pgsql.define_node_table('osm_settlements', {
    { column = 'osm_id', type = 'bigint' },
    { column = 'name', type = 'text' },
    { column = 'place', type = 'text' },
    { column = 'population', type = 'int' },
    { column = 'geom', type = 'point', projection = 4326 },
})

tables.amenities = osm2pgsql.define_node_table('osm_amenities', {
    { column = 'osm_id', type = 'bigint' },
    { column = 'name', type = 'text' },
    { column = 'type', type = 'text' },
    { column = 'subtype', type = 'text' },
    { column = 'geom', type = 'point', projection = 4326 },
})

tables.lifts = osm2pgsql.define_way_table('osm_lifts', {
    { column = 'osm_id', type = 'bigint' },
    { column = 'name',   type = 'text' },
    { column = 'geom',   type = 'linestring', projection = 4326 },
})

-- Helper: parse elevation string to number
local function parse_ele(ele_str)
    if ele_str == nil then return nil end
    local num = tonumber(ele_str:match("([%d%.%-]+)"))
    return num
end

-- Helper: parse population
local function parse_population(pop_str)
    if pop_str == nil then return nil end
    return tonumber(pop_str)
end

-- Env-var-basierter Kategorie-Filter (gesetzt vom Go-Handler)
local import_peaks       = os.getenv("SKADIA_IMPORT_PEAKS")       ~= nil
local import_passes      = os.getenv("SKADIA_IMPORT_PASSES")      ~= nil
local import_lifts       = os.getenv("SKADIA_IMPORT_LIFTS")       ~= nil
local import_settlements = os.getenv("SKADIA_IMPORT_SETTLEMENTS") ~= nil
local import_amenities   = os.getenv("SKADIA_IMPORT_AMENITIES")   ~= nil
local import_waterways   = os.getenv("SKADIA_IMPORT_WATERWAYS")   ~= nil
local import_forests     = os.getenv("SKADIA_IMPORT_FORESTS")     ~= nil
local import_boundaries  = os.getenv("SKADIA_IMPORT_BOUNDARIES")  ~= nil
-- Wenn keine Flags gesetzt, alles importieren (Backward-Compat für das Shell-Script)
local import_all = not (import_peaks or import_passes or import_lifts or
    import_settlements or import_amenities or import_waterways or
    import_forests or import_boundaries)

-- Process nodes
function osm2pgsql.process_node(object)
    local tags = object.tags

    -- Passstraßen (mountain_pass=yes) — Rennrad-relevante Pässe wie Stilfser Joch
    if (import_all or import_passes) and tags.mountain_pass == 'yes' then
        tables.passes:insert({
            osm_id = object.id,
            name   = tags.name,
            ele    = parse_ele(tags.ele),
            geom   = object:as_point(),
        })
    end

    -- Peaks, Vulkane und Sättel
    if (import_all or import_peaks) and
       (tags.natural == 'peak' or tags.natural == 'volcano'
        or tags.natural == 'saddle' or tags.natural == 'pass') then
        tables.peaks:insert({
            osm_id = object.id,
            name = tags.name,
            ele = parse_ele(tags.ele),
            type = tags.natural,
            geom = object:as_point(),
        })
    end

    -- Settlements
    if import_all or import_settlements then
        local place = tags.place
        if place == 'city' or place == 'town' or place == 'village' or place == 'hamlet' or place == 'suburb' then
            tables.settlements:insert({
                osm_id = object.id,
                name = tags.name,
                place = place,
                population = parse_population(tags.population),
                geom = object:as_point(),
            })
        end
    end

    -- Amenities (gastronomic, tourism, leisure)
    -- Relevant tags: amenity=restaurant/cafe/bar/pub/fast_food/biergarten/food_court/ice_cream,
    --   tourism=alpine_hut/guest_house/hostel/hotel,
    --   leisure=fitness_station/sports_centre etc.
    -- The generic catch-all (tourism or amenity or leisure) is intentional — it captures
    -- a wide range of POIs relevant for activity enrichment without enumerating every subtype.
    if import_all or import_amenities then
        local amenity_type = tags.tourism or tags.amenity or tags.leisure
        if amenity_type and tags.name then
            local subtype = nil
            if tags.tourism then subtype = tags.tourism
            elseif tags.amenity then subtype = tags.amenity
            elseif tags.leisure then subtype = tags.leisure
            end
            tables.amenities:insert({
                osm_id = object.id,
                name = tags.name,
                type = amenity_type,
                subtype = subtype,
                geom = object:as_point(),
            })
        end

        -- Shops: kiosk, bakery, butcher (alle mit Namen, kein takeaway-Filter --
        -- viele Bäckereien haben Sitzplätze aber keinen takeaway-Tag in OSM)
        if tags.name then
            if tags.shop == 'kiosk' or tags.shop == 'bakery' or tags.shop == 'butcher' then
                tables.amenities:insert({
                    osm_id = object.id,
                    name = tags.name,
                    type = 'shop',
                    subtype = tags.shop,
                    geom = object:as_point(),
                })
            end
        end
    end
end

-- Process ways
function osm2pgsql.process_way(object)
    local tags = object.tags

    -- Waterways
    if import_all or import_waterways then
        local waterway = tags.waterway
        if waterway == 'river' or waterway == 'stream' or waterway == 'canal' then
            if not object.is_closed then
                tables.waterways:insert({
                    osm_id = object.id,
                    name = tags.name,
                    waterway = waterway,
                    geom = object:as_linestring(),
                })
            end
        end
    end

    -- Lifte (Aerialways, Standseilbahnen, Zahnradbahnen)
    if import_all or import_lifts then
        local passenger_aerialways = {
            cable_car=true, gondola=true, chair_lift=true, mixed_lift=true,
            drag_lift=true, ['t-bar']=true, ['j-bar']=true, platter=true,
            rope_tow=true, magic_carpet=true
        }
        if passenger_aerialways[tags.aerialway] then
            tables.lifts:insert({
                osm_id = object.id,
                name   = tags.name,
                geom   = object:as_linestring(),
            })
        end
        if tags.railway == 'funicular' then
            tables.lifts:insert({
                osm_id = object.id,
                name   = tags.name,
                geom   = object:as_linestring(),
            })
        end
        if tags.railway == 'rail' and tags.rack == 'yes' then
            tables.lifts:insert({
                osm_id = object.id,
                name   = tags.name,
                geom   = object:as_linestring(),
            })
        end
    end

    -- Forests (closed ways)
    if object.is_closed then
        if (import_all or import_forests) and
           (tags.natural == 'wood' or tags.landuse == 'forest') then
            if tags.name then
                tables.forests:insert({
                    osm_id = object.id,
                    name = tags.name,
                    geom = object:as_multipolygon(),
                })
            end
        end

        -- Waters (closed ways)
        if (import_all or import_waterways) and tags.natural == 'water' then
            tables.waters:insert({
                osm_id = object.id,
                name = tags.name,
                geom = object:as_multipolygon(),
            })
        end
    end
end

-- Process relations
function osm2pgsql.process_relation(object)
    local tags = object.tags
    local rtype = tags.type

    -- Admin boundaries
    if (import_all or import_boundaries) and
       rtype == 'boundary' and tags.boundary == 'administrative' then
        local admin_level = tonumber(tags.admin_level)
        if admin_level and (admin_level == 2 or admin_level == 4 or admin_level == 6 or admin_level == 8) then
            local country_code = tags['ISO3166-1:alpha2'] or tags['ISO3166-1'] or nil
            tables.boundaries:insert({
                osm_id = object.id,
                name = tags.name,
                admin_level = admin_level,
                country_code = country_code,
                geom = object:as_multipolygon(),
            })
        end
    end

    -- Forests (multipolygon relations)
    if rtype == 'multipolygon' then
        if (import_all or import_forests) and
           (tags.natural == 'wood' or tags.landuse == 'forest') then
            if tags.name then
                tables.forests:insert({
                    osm_id = object.id,
                    name = tags.name,
                    geom = object:as_multipolygon(),
                })
            end
        end

        -- Waters
        if (import_all or import_waterways) and tags.natural == 'water' then
            tables.waters:insert({
                osm_id = object.id,
                name = tags.name,
                geom = object:as_multipolygon(),
            })
        end
    end
end
