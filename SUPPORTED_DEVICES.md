<!-- Generated file — do not edit manually. -->
<!-- Run: go test ./internal/fitimport/ -run TestUpdateSupportedDevices -update -->

# Supported Devices

## How to Add a New Device

To add support for a new device, edit the appropriate source file below, then run the update command to regenerate this file:

```
go test ./internal/fitimport/ -run TestUpdateSupportedDevices -update
```

| Source File | When to Use | Mechanism |
|---|---|---|
| [`services/api/internal/fitimport/devicelookup.go`](services/api/internal/fitimport/devicelookup.go) | Hardware item with known `[manufacturer_id, product_id]` pair not in FIT SDK | Static map lookup |
| [`services/api/internal/fitimport/manufacturer.go`](services/api/internal/fitimport/manufacturer.go) | Development devices (mfr=255): Apple Watch, Samsung, Amazfit, etc. | String pattern matching on product_name |
| [`services/api/internal/fitimport/developer_app_registry.go`](services/api/internal/fitimport/developer_app_registry.go) | Third-party apps with known Application ID | Application ID hex lookup |

## Generic Support

Detected recording devices and most sensors with serial numbers appear on activity pages and are also listed under Gear (`/hardware/recorder` and `/hardware/sensor`). They can also be used in activity filters and automation rules.

FIT files from any device are imported. All ANT+ and Bluetooth Low Energy sensors are automatically recognized. Data is fully imported even if the specific model name is not listed below. GPX and TCX files can also be imported, but they usually contain less device and sensor metadata.

This catalog lists recognized recording devices, sensors, sync apps, developer apps, smart trainers, and virtual platforms.

- Sensor data (heart rate, power, cadence, speed, temperature, etc.) is imported regardless of whether the sensor model is known
- FIT Developer Field values are imported when present in activity streams; recognized developer apps are listed below.
- Unknown devices may show only the manufacturer name or no device name; the data itself is unaffected

### Sync Apps

FitnessSyncer, RunGap, HealthFit, and iSmoothRun are recognized as import sources. Activities imported via these apps retain the original recording device information where available.

## Recording Devices

| Manufacturer | Model | Source |
|---|---|---|
| Amazfit | All models<sup>1</sup> | Pattern |
| Apple | All Watch models<sup>1</sup> | Pattern |
| COROS | APEX | Map |
| COROS | APEX 2 | Map |
| COROS | APEX 2 Pro | Map |
| COROS | APEX Pro | Map |
| COROS | DURA | Map |
| COROS | PACE | Map |
| COROS | PACE 2 | Map |
| COROS | PACE 3 | Map |
| COROS | PACE Pro | Map |
| COROS | VERTIX | Map |
| COROS | VERTIX 2 | Map |
| COROS | VERTIX 2S | Map |
| Coospo | BC107 | Map |
| Coospo | BC200 | Map |
| Cycplus | M1 | Map |
| Cycplus | M2 | Map |
| Garmin | Approach G 10 | SDK |
| Garmin | Approach G 12 | SDK |
| Garmin | Approach G 80 | SDK |
| Garmin | Approach S 10 | SDK |
| Garmin | Approach S 12 | SDK |
| Garmin | Approach S 20 | SDK |
| Garmin | Approach S 40 | SDK |
| Garmin | Approach S 42 | SDK |
| Garmin | Approach S 6 | SDK |
| Garmin | Approach S 60 | SDK |
| Garmin | Approach S 70 | SDK |
| Garmin | Approach S44 | SDK |
| Garmin | Approach S50 | SDK |
| Garmin | Approach X 10 | SDK |
| Garmin | Approach X 40 | SDK |
| Garmin | Bounce 2 | SDK |
| Garmin | D2 Air Venu | SDK |
| Garmin | D2 Air X10 | SDK |
| Garmin | D2 Air X15 | SDK |
| Garmin | D2 Bravo | SDK |
| Garmin | D2 Bravo Titanium | SDK |
| Garmin | D2 Charlie | SDK |
| Garmin | D2 Mach 1 Pro | SDK |
| Garmin | D2 Mach 2 | SDK |
| Garmin | Descent | SDK |
| Garmin | Descent G 1 | SDK |
| Garmin | Descent G 2 | SDK |
| Garmin | Descent Mk 2 | SDK |
| Garmin | Descent Mk 2S | SDK |
| Garmin | Descent Mk 3 | SDK |
| Garmin | Descent Mk3i | SDK |
| Garmin | Descent T 1 | SDK |
| Garmin | Descent T 2 | SDK |
| Garmin | Edge 1000 | SDK |
| Garmin | Edge 1030 | SDK |
| Garmin | Edge 1030 Plus | SDK |
| Garmin | Edge 1040 | SDK |
| Garmin | Edge 1050 | SDK |
| Garmin | Edge 130 | SDK |
| Garmin | Edge 130 Plus | SDK |
| Garmin | Edge 20 | SDK |
| Garmin | Edge 200 | SDK |
| Garmin | Edge 25 | SDK |
| Garmin | Edge 500 | SDK |
| Garmin | Edge 510 | SDK |
| Garmin | Edge 520 | SDK |
| Garmin | Edge 520 Plus | SDK |
| Garmin | Edge 530 | SDK |
| Garmin | Edge 540 | SDK |
| Garmin | Edge 550 | SDK |
| Garmin | Edge 800 | SDK |
| Garmin | Edge 810 | SDK |
| Garmin | Edge 820 | SDK |
| Garmin | Edge 830 | SDK |
| Garmin | Edge 840 | SDK |
| Garmin | Edge 850 | SDK |
| Garmin | Edge Explore | SDK |
| Garmin | Edge Explore 1000 | SDK |
| Garmin | Edge Explore 2 | SDK |
| Garmin | Edge Explore 820 | SDK |
| Garmin | Edge MTB | SDK |
| Garmin | Edge Touring | SDK |
| Garmin | Enduro | SDK |
| Garmin | Enduro 2 | SDK |
| Garmin | Enduro 3 | SDK |
| Garmin | Epix | SDK |
| Garmin | Epix Gen 2 | SDK |
| Garmin | Epix Gen 2 Pro 42 | SDK |
| Garmin | Epix Gen 2 Pro 47 | SDK |
| Garmin | Epix Gen 2 Pro 51 | SDK |
| Garmin | Etrex Touch | SDK |
| Garmin | FR10 | SDK |
| Garmin | FR110 | SDK |
| Garmin | FR15 | SDK |
| Garmin | FR165 | SDK |
| Garmin | FR165 Music | SDK |
| Garmin | FR210 Japan | SDK |
| Garmin | FR220 | SDK |
| Garmin | FR225 | SDK |
| Garmin | FR230 | SDK |
| Garmin | FR235 Asia | SDK |
| Garmin | FR235 China Nfc | SDK |
| Garmin | FR235 Lite | SDK |
| Garmin | FR245 | SDK |
| Garmin | FR245 Music | SDK |
| Garmin | FR245M Asia | SDK |
| Garmin | FR25 | SDK |
| Garmin | FR255 | SDK |
| Garmin | FR255 Music | SDK |
| Garmin | FR255 Small | SDK |
| Garmin | FR255 Small Music | SDK |
| Garmin | FR265 Large | SDK |
| Garmin | FR265 Small | SDK |
| Garmin | FR30 | SDK |
| Garmin | FR301 China | SDK |
| Garmin | FR310XT | SDK |
| Garmin | FR310XT 4T | SDK |
| Garmin | FR35 | SDK |
| Garmin | FR405 | SDK |
| Garmin | FR45 | SDK |
| Garmin | FR50 | SDK |
| Garmin | FR55 | SDK |
| Garmin | FR570 | Map |
| Garmin | FR60 | SDK |
| Garmin | FR610 | SDK |
| Garmin | FR620 | SDK |
| Garmin | FR630 | SDK |
| Garmin | FR645 | SDK |
| Garmin | FR645M | SDK |
| Garmin | FR70 | SDK |
| Garmin | FR735XT | SDK |
| Garmin | FR745 | SDK |
| Garmin | FR910XT | SDK |
| Garmin | FR920XT | SDK |
| Garmin | FR935 | SDK |
| Garmin | FR945 | SDK |
| Garmin | FR945 LTE | SDK |
| Garmin | FR955 | SDK |
| Garmin | FR965 | SDK |
| Garmin | FR970 | Map |
| Garmin | Fenix | SDK |
| Garmin | Fenix 2 | SDK |
| Garmin | Fenix 3 | SDK |
| Garmin | Fenix 3 Chronos | SDK |
| Garmin | Fenix 3 HR | SDK |
| Garmin | Fenix 5 | SDK |
| Garmin | Fenix 5 Plus | SDK |
| Garmin | Fenix 5S | SDK |
| Garmin | Fenix 5S Plus | SDK |
| Garmin | Fenix 5X | SDK |
| Garmin | Fenix 5X Plus | SDK |
| Garmin | Fenix 6 | SDK |
| Garmin | Fenix 6 Sport | SDK |
| Garmin | Fenix 6S | SDK |
| Garmin | Fenix 6S Sport | SDK |
| Garmin | Fenix 6X | SDK |
| Garmin | Fenix 7 | SDK |
| Garmin | Fenix 7 Pro Solar | SDK |
| Garmin | Fenix 7S | SDK |
| Garmin | Fenix 7S Pro Solar | SDK |
| Garmin | Fenix 7X | SDK |
| Garmin | Fenix 7X Pro Solar | SDK |
| Garmin | Fenix 8 | SDK |
| Garmin | Fenix 8 Pro | SDK |
| Garmin | Fenix 8 Small | SDK |
| Garmin | Fenix 8 Solar | SDK |
| Garmin | Fenix 8 Solar Large | SDK |
| Garmin | Fenix E | SDK |
| Garmin | Foretrex 601/701 | SDK |
| Garmin | GPSMAP 66 | SDK |
| Garmin | GPSMAP 66i | SDK |
| Garmin | Instinct 2 | SDK |
| Garmin | Instinct 2S | SDK |
| Garmin | Instinct 2X | SDK |
| Garmin | Instinct 3 AMOLED 45mm | SDK |
| Garmin | Instinct 3 AMOLED 50mm | SDK |
| Garmin | Instinct 3 Solar 45mm | SDK |
| Garmin | Instinct 3 Solar 50mm | SDK |
| Garmin | Instinct Crossover | SDK |
| Garmin | Instinct Crossover AMOLED | SDK |
| Garmin | Instinct E 40mm | SDK |
| Garmin | Instinct E 45mm | SDK |
| Garmin | Instinct Esports | SDK |
| Garmin | Instinct Solar | SDK |
| Garmin | Legacy Captain Marvel | SDK |
| Garmin | Legacy Darth Vader | SDK |
| Garmin | Legacy First Avenger | SDK |
| Garmin | Legacy Rey | SDK |
| Garmin | Lily | SDK |
| Garmin | Lily 2 | SDK |
| Garmin | Lily Athlete | SDK |
| Garmin | MARQ Adventurer | SDK |
| Garmin | MARQ Athlete | SDK |
| Garmin | MARQ Aviator | SDK |
| Garmin | MARQ Captain | SDK |
| Garmin | MARQ Commander | SDK |
| Garmin | MARQ Driver | SDK |
| Garmin | MARQ Expedition | SDK |
| Garmin | MARQ Gen 2 | SDK |
| Garmin | MARQ Gen 2 Aviator | SDK |
| Garmin | MARQ Gen 2 Commander | SDK |
| Garmin | MARQ Golfer | SDK |
| Garmin | Oregon 7XX | SDK |
| Garmin | Rino 7XX | SDK |
| Garmin | Swim | SDK |
| Garmin | Swim 2 | SDK |
| Garmin | Tactix 7 | SDK |
| Garmin | Tactix 8 AMOLED | SDK |
| Garmin | Tactix 8 Solar | SDK |
| Garmin | Venu | SDK |
| Garmin | Venu 2 | SDK |
| Garmin | Venu 2 Plus | SDK |
| Garmin | Venu 2S | SDK |
| Garmin | Venu 3 | SDK |
| Garmin | Venu 3S | SDK |
| Garmin | Venu 4 | SDK |
| Garmin | Venu 4S | SDK |
| Garmin | Venu SQ | SDK |
| Garmin | Venu SQ 2 | SDK |
| Garmin | Venu SQ 2 Music | SDK |
| Garmin | Venu SQ Music | SDK |
| Garmin | Venu SQ Music V2 | SDK |
| Garmin | Venu X 1 | SDK |
| Garmin | Vivo Active | SDK |
| Garmin | Vivo Active 3M Apac | SDK |
| Garmin | Vivo Active 3T Chn | SDK |
| Garmin | Vivo Active 4 Large Asia | SDK |
| Garmin | Vivo Active 4 Oled Asia | SDK |
| Garmin | Vivo Active 4 Small Asia | SDK |
| Garmin | Vivo Active HR | SDK |
| Garmin | Vivo Fit | SDK |
| Garmin | Vivo Fit 2 | SDK |
| Garmin | Vivo Fit 3 | SDK |
| Garmin | Vivo Fit 4 | SDK |
| Garmin | Vivo Fit JR | SDK |
| Garmin | Vivo KI | SDK |
| Garmin | Vivo Move | SDK |
| Garmin | Vivo Move 3 | SDK |
| Garmin | Vivo Move 3 Premium | SDK |
| Garmin | Vivo Move HR | SDK |
| Garmin | Vivo Move Sport | SDK |
| Garmin | Vivo Smart | SDK |
| Garmin | Vivo Smart 2 | SDK |
| Garmin | Vivo Smart 3 | SDK |
| Garmin | Vivo Smart 4 Asia | SDK |
| Garmin | Vivo Smart GPS HR | SDK |
| Garmin | Vivo Smart HR | SDK |
| Garmin | Vivo Sport | SDK |
| Garmin | Vivoactive 3 | SDK |
| Garmin | Vivoactive 3 Music | SDK |
| Garmin | Vivoactive 3 Music LTE | SDK |
| Garmin | Vivoactive 4 Large | SDK |
| Garmin | Vivoactive 4 Small | SDK |
| Garmin | Vivoactive 5 | SDK |
| Garmin | Vivoactive 6 | SDK |
| Garmin | Vivomove Trend | SDK |
| Garmin | Vivosmart 4 | SDK |
| Garmin | Vivosmart 5 | SDK |
| Google | All Pixel Watch models<sup>1</sup> | Pattern |
| Hammerhead | Karoo | Map |
| Hammerhead | Karoo 2 | Map |
| Hammerhead | Karoo 3 | Map |
| Huawei | All wearable models<sup>2</sup> | Pattern |
| IpBike | App | Map |
| Lezyne | Macro GPS | Map |
| Lezyne | Mega XL GPS | Map |
| Lezyne | Micro C GPS | Map |
| Lezyne | Micro GPS | Map |
| Lezyne | Super GPS | Map |
| Lezyne | Super Pro GPS | Map |
| Magene | C406 | Map |
| Magene | C606 | Map |
| Magene | L508 | Map |
| Morpheus | M7 | Map |
| Polar | A300 | Map |
| Polar | A360 | Map |
| Polar | A370 | Map |
| Polar | All models with product_name metadata<sup>1</sup> | Pattern |
| Polar | Grit X | Map |
| Polar | Grit X Pro | Map |
| Polar | Grit X2 | Map |
| Polar | Ignite | Map |
| Polar | Ignite 2 | Map |
| Polar | Ignite 3 | Map |
| Polar | Loop | Map |
| Polar | Loop 2 | Map |
| Polar | M200 | Map |
| Polar | M400 | Map |
| Polar | M430 | Map |
| Polar | M450 | Map |
| Polar | M460 | Map |
| Polar | M600 | Map |
| Polar | Pacer | Map |
| Polar | Pacer Pro | Map |
| Polar | Unite | Map |
| Polar | V650 | Map |
| Polar | V800 | Map |
| Polar | Vantage M | Map |
| Polar | Vantage M2 | Map |
| Polar | Vantage M3 | Map |
| Polar | Vantage V | Map |
| Polar | Vantage V2 | Map |
| Polar | Vantage V3 | Map |
| Powertap | Joule | Map |
| Powertap | Joule 2.0 | Map |
| Powertap | Joule GPS | Map |
| Powertap | Joule GPS+ | Map |
| Samsung | All Galaxy Watch models<sup>1</sup> | Pattern |
| Sigma Sport | PURE GPS | Map |
| Sigma Sport | ROX 10.0 | Map |
| Sigma Sport | ROX 11.0 | Map |
| Sigma Sport | ROX 11.1 | Map |
| Sigma Sport | ROX 12.0 | Map |
| Sigma Sport | ROX 12.1 EVO | Map |
| Sigma Sport | ROX 2.0 | Map |
| Sigma Sport | ROX 4.0 | Map |
| Sigma Sport | ROX 4.0 SE | Map |
| Sigma Sport | ROX 7.0 | Map |
| Sigma Sport | iD.FREE | Map |
| Sigma Sport | iD.RUN | Map |
| Sigma Sport | iD.RUN HR | Map |
| Sigma Sport | iD.TRI | Map |
| Suunto | 3 Fitness | Map |
| Suunto | 5 | Map |
| Suunto | 7 | Map |
| Suunto | 9 Baro | Map |
| Suunto | 9 Peak | Map |
| Suunto | 9 Peak Pro | Map |
| Suunto | Ambit3 Peak | Map |
| Suunto | Ambit3 Run | Map |
| Suunto | Ambit3 Sport | Map |
| Suunto | Ambit3 Vertical | Map |
| Suunto | Ocean | Map |
| Suunto | Race | Map |
| Suunto | Race S | Map |
| Suunto | Spartan Sport | Map |
| Suunto | Spartan Sport Wrist HR | Map |
| Suunto | Spartan Trainer | Map |
| Suunto | Spartan Ultra | Map |
| Suunto | Vertical | Map |
| Suunto | Vertical 2 | Map |
| Timex | Ironman GPS | Map |
| TomTom | Runner 3 Cardio | Map |
| Wahoo | ELEMNT | Map |
| Wahoo | ELEMNT BOLT | Map |
| Wahoo | ELEMNT BOLT v2 | Map |
| Wahoo | ELEMNT BOLT v3 | Map |
| Wahoo | ELEMNT ROAM | Map |
| Wahoo | ELEMNT ROAM v2 | Map |
| Wahoo | ELEMNT ROAM v3 | Map |
| Wahoo | ELEMNT Rival | Map |
| XOSS | G+N3 | Map |
| XOSS | G+N4 | Map |
| Xplova | X5 EVO | Map |
| o_synce | navi2coach | Map |

**Recording device notes:**

<sup>1</sup> Detected via product name.

<sup>2</sup> Detected via file descriptor; exact model unavailable.


## Sensors

Some sensor-like devices and apps, including Stryd, use FIT Developer Fields instead of standard ANT+/BLE sensor records. If a sensor is not listed here, also check [Developer Apps](#developer-apps).

| Manufacturer | Model | Type | Source |
|---|---|---|---|
| BSX Athletics | BSX Insight 2 | Muscle Oxygen | Map |
| COROS | POD | Running Dynamics | Map |
| COROS | POD 2 | Running Dynamics | Map |
| Favero | Assioma Duo | Power Meter | SDK |
| Favero | Assioma Pro MX | Power Meter | Map |
| Favero | Assioma Uno | Power Meter | SDK |
| Garmin | BCM | Cadence | SDK |
| Garmin | BSM | Speed | SDK |
| Garmin | Gen 3 BCM | Cadence | SDK |
| Garmin | Gen 3 BSM | Speed | SDK |
| Garmin | HRM 200 | Heart Rate | SDK |
| Garmin | HRM Dual | Heart Rate | SDK |
| Garmin | HRM Fit | Heart Rate | SDK |
| Garmin | HRM Pro | Heart Rate | SDK |
| Garmin | HRM Pro Plus | Heart Rate | SDK |
| Garmin | HRM Run | Heart Rate | SDK |
| Garmin | HRM Tri | Heart Rate | SDK |
| Garmin | HRM1 | Heart Rate | SDK |
| Garmin | HRM2SS | Heart Rate | SDK |
| Garmin | HRM3SS | Heart Rate | SDK |
| Garmin | HRM4 Run | Heart Rate | SDK |
| Garmin | Index Smart Scale | Weight Scale | SDK |
| Garmin | Index Smart Scale 2 | Weight Scale | SDK |
| Garmin | OHR | Heart Rate | SDK |
| Garmin | Rally 200 | Power Meter | SDK |
| Garmin | Rally X 10 | Power Meter | SDK |
| Garmin | Running Dynamics Pod | Running Dynamics | SDK |
| Garmin | SDM4 | Running Dynamics | SDK |
| Garmin | Tempe | Temperature | SDK |
| Garmin | Varia Headlight | Lighting | SDK |
| Garmin | Varia RCT715 | Radar | SDK |
| Garmin | Varia RTL515 | Radar | Map |
| Garmin | Varia Radar Display | Radar | SDK |
| Garmin | Varia Radar Taillight | Radar | SDK |
| Garmin | Varia Taillight | Lighting | SDK |
| Garmin | Varia Taillight Old | Lighting | SDK |
| Garmin | Varia Ut 800 | Lighting | SDK |
| Garmin | Varia Vision | Radar | SDK |
| Garmin | Vector 2 | Power Meter | SDK |
| Garmin | Vector 2S | Power Meter | SDK |
| Garmin | Vector 3 | Power Meter | SDK |
| Garmin | Vector CP | Power Meter | SDK |
| Garmin | Vector S | Power Meter | SDK |
| Garmin | Vector SS | Power Meter | SDK |
| Polar | H10 | Heart Rate | Map |
| Polar | H7 | Heart Rate | Map |
| Polar | H9 | Heart Rate | Map |
| Polar | OH1 | Heart Rate | Map |
| Polar | Verity Sense | Heart Rate | Map |
| Power2max | S | Power Meter | Map |
| Powertap | C1 | Power Meter | Map |
| Powertap | G3 | Power Meter | Map |
| Powertap | P1 | Power Meter | Map |
| Powertap | Powercal | Power Meter | Map |
| Quarq | Cinqo | Power Meter | Map |
| Quarq | DZero | Power Meter | Map |
| SRAM | Rival AXS | Shifting | Map |
| SRAM | Rival eTap AXS | Shifting | Map |
| SRM | PC6 | Power Meter | Map |
| SRM | PC7 | Power Meter | Map |
| SRM | PC8 | Power Meter | Map |
| Scosche | Rhythm+ | Heart Rate | Map |
| Stages | Power Gen 1 | Power Meter | Map |
| Stages | Power Gen 2 | Power Meter | Map |
| Stages | Power Gen 3 | Power Meter | Map |
| Wahoo | PowrLink | Power Meter | Map |
| Wahoo | TICKR | Heart Rate | Map |

## Developer Apps

Developer Apps use FIT Developer Fields to embed custom data and are detected by Application ID. Multiple application IDs can resolve to the same app entry.

| App | Category | Source |
|---|---|---|
| CORE | Body Temperature | Registry |
| COROS | Running Power | Registry |
| Daefore | Bike Power | Registry |
| FORM | Swim Goggle | Registry |
| Intervals CIQ | Training App | Registry |
| RideWithPower | Bike Power | Registry |
| Stryd | Running Power | Registry |
| SuuntoFitExport1 | Sport Watch | Registry |
| SuuntoplusFitExt | Sport Watch | Registry |
| Varia Radar | Radar | Registry |
| Zepp | Wearable | Registry |

## Smart Trainers & Virtual Platforms

### Smart Trainers

| Manufacturer | Model | Source |
|---|---|---|
| Garmin | Tacx Bushido Smart | SDK |
| Garmin | Tacx Flow Smart | SDK |
| Garmin | Tacx Flux 2 Smart | SDK |
| Garmin | Tacx Flux/Flux S Smart | SDK |
| Garmin | Tacx Genius Smart | SDK |
| Garmin | Tacx Magnum | SDK |
| Garmin | Tacx Neo 2 Smart | SDK |
| Garmin | Tacx Neo 2 T Smart | SDK |
| Garmin | Tacx Neo Smart | SDK |
| Garmin | Tacx Neo Smart Bike | SDK |
| Garmin | Tacx Satori Smart | SDK |
| Garmin | Tacx Vortex Smart | SDK |
| Wattbike | Atom | Map |
| Wattbike | Atom X | Map |
| Wattbike | Icon | Map |
| Xplova | Noza S | Map |

### Virtual Platforms

| Platform | Source |
|---|---|
| Bkool | Pattern |
| MyWhoosh | Pattern |
| Peloton | Pattern |
| RGT Cycling | Pattern |
| Rouvy | Pattern |
| TrainerRoad | Pattern |
| Virtugo | Pattern |
| Wahoo SYSTM | Pattern |
| Zwift | Pattern |
| iCTrainer | Pattern |

---

**Source column legend:**
- **SDK** — Resolved via FIT SDK enum (GarminProduct, FaveroProduct)
- **Map** — Resolved via static lookup in [devicelookup.go](services/api/internal/fitimport/devicelookup.go)
- **Pattern** — Resolved via string pattern matching in [manufacturer.go](services/api/internal/fitimport/manufacturer.go)
- **Registry** — Resolved via Application ID in [developer_app_registry.go](services/api/internal/fitimport/developer_app_registry.go)
