// +heroku goVersion go1.18

module github.com/penny-vault/pv-api

go 1.18

require (
	github.com/andybalholm/brotli v1.0.4 // indirect
	github.com/apache/thrift v0.16.0 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/go-co-op/gocron v1.16.1
	github.com/go-redis/redis/v8 v8.11.5
	github.com/goccy/go-json v0.9.10
	github.com/gofiber/fiber/v2 v2.35.0
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/uuid v1.3.0
	github.com/guptarohit/asciigraph v0.5.5 // indirect
	github.com/hashicorp/golang-lru v0.5.4
	github.com/jackc/pgsql v0.0.0-20220720200728-bf8deec3ac70
	github.com/jackc/pgx/v4 v4.16.1
	github.com/jdfergason/jwt/v2 v2.2.6
	github.com/lestrrat-go/backoff/v2 v2.0.8 // indirect
	github.com/lestrrat-go/jwx v1.2.25
	github.com/magefile/mage v1.13.0
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/olekukonko/tablewriter v0.0.5 // indirect
	github.com/onsi/gomega v1.20.0
	github.com/pelletier/go-toml/v2 v2.0.2
	github.com/pierrec/lz4/v4 v4.1.15
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/cobra v1.5.0
	github.com/spf13/viper v1.12.0
	github.com/valyala/fasthttp v1.38.0 // indirect
	github.com/xitongsys/parquet-go v1.6.2 // indirect
	github.com/zeebo/blake3 v0.2.3
	golang.org/x/crypto v0.0.0-20220622213112-05595931fe9d // indirect
	golang.org/x/exp v0.0.0-20220713135740-79cabaa25d75 // indirect
	golang.org/x/net v0.0.0-20220708220712-1185a9018129 // indirect
	golang.org/x/sys v0.0.0-20220721230656-c6bc011c0c49 // indirect
	golang.org/x/text v0.3.7 // indirect
	gonum.org/v1/gonum v0.11.0
	google.golang.org/protobuf v1.28.0 // indirect
)

require github.com/robfig/cron/v3 v3.0.1

require (
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/jdfergason/dataframe-go v0.2.0
	github.com/onsi/ginkgo/v2 v2.1.4
	github.com/pashagolub/pgxmock v1.6.0
	github.com/rs/zerolog v1.27.0
	github.com/sendgrid/sendgrid-go v3.11.1+incompatible
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.8.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.8.0
)

require (
	cloud.google.com/go v0.102.1 // indirect
	github.com/cenkalti/backoff/v4 v4.1.3 // indirect
	github.com/frankban/quicktest v1.14.3 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.11.0 // indirect
	github.com/jackc/pgx/v5 v5.0.0-alpha.5 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/ompluscator/dynamic-struct v1.3.0 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/rogpeppe/fastuuid v1.2.0 // indirect
	github.com/rogpeppe/go-internal v1.8.1 // indirect
	github.com/sendgrid/rest v2.6.9+incompatible // indirect
	github.com/shabbyrobe/xmlwriter v0.0.0-20220218224045-defe0ad214f6 // indirect
	github.com/tealeg/xlsx/v3 v3.2.4 // indirect
	github.com/xitongsys/parquet-go-source v0.0.0-20220624101223-5cb561a812f4 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.8.0 // indirect
	go.opentelemetry.io/proto/otlp v0.18.0 // indirect
	google.golang.org/genproto v0.0.0-20220720214146-176da50484ac // indirect
)

require (
	github.com/apache/arrow/go/arrow v0.0.0-20211112161151-bc219186db40 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/go-cmp v0.5.8 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgconn v1.12.1
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20200714003250-2b9c44734f2b // indirect
	github.com/jackc/pgtype v1.11.0
	github.com/jackc/puddle v1.2.2-0.20220404125616-4e959849469a // indirect
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/klauspost/cpuid/v2 v2.1.0 // indirect
	github.com/lestrrat-go/blackmagic v1.0.1 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/option v1.0.0 // indirect
	github.com/magiconair/properties v1.8.6 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/sandertv/go-formula/v2 v2.0.0-alpha.7 // indirect
	github.com/spf13/afero v1.9.2 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/subosito/gotenv v1.4.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	go.opentelemetry.io/otel v1.9.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.8.0
	go.opentelemetry.io/otel/sdk v1.8.0
	go.opentelemetry.io/otel/trace v1.9.0
	golang.org/x/sync v0.0.0-20220601150217-0de741cfad7f // indirect
	golang.org/x/xerrors v0.0.0-20220609144429-65e65417b02f // indirect
	google.golang.org/grpc v1.48.0 // indirect
	gopkg.in/ini.v1 v1.66.6 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
