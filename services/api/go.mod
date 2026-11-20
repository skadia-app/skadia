module github.com/skadia-app/skadia/services/api

go 1.27.0

require (
	github.com/alexedwards/argon2id v1.0.0
	github.com/aws/aws-sdk-go-v2 v1.48.0
	github.com/aws/aws-sdk-go-v2/credentials v1.21.3
	github.com/aws/aws-sdk-go-v2/service/s3 v1.116.0
	github.com/bep/imagemeta v0.17.0
	github.com/corona10/goimagehash v1.1.0
	github.com/disintegration/imaging v1.6.2
	github.com/fogleman/delaunay v0.0.0-20180910191513-63f09b4c883d
	github.com/fsnotify/fsnotify v1.10.1
	github.com/go-chi/chi/v5 v5.5.0
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.13.0
	github.com/lestrrat-go/jwx/v4 v4.6.0
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/muktihari/fit v0.28.7
	github.com/nats-io/nats.go v1.56.0
	github.com/paulmach/orb v0.14.0
	github.com/pdfcpu/pdfcpu v0.16.4
	github.com/prometheus/client_golang v1.25.0
	github.com/prometheus/client_model v0.6.3
	github.com/ringsaturn/tzf v1.2.5
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
	github.com/sixdouglas/suncalc v0.0.0-20250114185126-291b1938b70c
	github.com/wneessen/go-mail v0.8.2
	github.com/zeebo/xxh3 v1.1.0
	golang.org/x/image v0.48.0
	golang.org/x/oauth2 v0.38.0
	golang.org/x/sync v0.25.0
	golang.org/x/sys v0.50.0
	golang.org/x/text v0.44.0
	golang.org/x/time v0.16.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.27 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.12 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.12 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.12 // indirect
	github.com/aws/smithy-go v1.28.9 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/hhrutter/tiff v1.0.7 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/klauspost/compress v1.20.1 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/lestrrat-go/dsig v1.4.0 // indirect
	github.com/lestrrat-go/option/v3 v3.0.0-alpha1 // indirect
	github.com/mattn/go-runewidth v0.0.31 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nats-io/nkeys v0.4.16 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646 // indirect
	github.com/prometheus/common v0.72.0 // indirect
	github.com/prometheus/procfs v0.22.0 // indirect
	github.com/ringsaturn/orb v0.15.0 // indirect
	github.com/ringsaturn/tzf-dist v0.0.2026-c-fix1 // indirect
	github.com/tidwall/geoindex v1.7.0 // indirect
	github.com/tidwall/rtree v1.10.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	go.mongodb.org/mongo-driver/v2 v2.5.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/net v0.63.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

// Run `go mod tidy` after initial setup to resolve indirect dependencies
