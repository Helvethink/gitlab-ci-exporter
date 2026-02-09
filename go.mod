module github.com/helvethink/gitlab-ci-exporter

go 1.24.0

require (
	dario.cat/mergo v1.0.2
	github.com/alicebob/miniredis/v2 v2.35.0
	github.com/charmbracelet/bubbles v0.21.0
	github.com/charmbracelet/bubbletea v1.3.6
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/creasty/defaults v1.8.0
	github.com/go-logr/stdr v1.2.2
	github.com/go-playground/validator/v10 v10.27.0
	github.com/go-redis/redis_rate/v10 v10.0.1
	github.com/google/uuid v1.6.0
	github.com/heptiolabs/healthcheck v0.0.0-20211123025425-613501dd5deb
	github.com/paulbellamy/ratecounter v0.2.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.23.2
	github.com/redis/go-redis/extra/redisotel/v9 v9.12.1
	github.com/redis/go-redis/v9 v9.16.0
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.11.1
	github.com/uptrace/opentelemetry-go-extra/otellogrus v0.3.2
	github.com/urfave/cli/v2 v2.27.7
	github.com/vmihailenco/msgpack/v5 v5.4.1
	github.com/vmihailenco/taskq/memqueue/v4 v4.0.0-beta.4
	github.com/vmihailenco/taskq/redisq/v4 v4.0.0-beta.4
	github.com/vmihailenco/taskq/v4 v4.0.0-beta.4
	github.com/xeonx/timeago v1.0.0-rc5
	gitlab.com/gitlab-org/api/client-go v0.148.1
	go.openly.dev/pointy v1.3.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.63.0
	go.opentelemetry.io/otel v1.38.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.37.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.37.0
	go.opentelemetry.io/otel/sdk v1.38.0
	go.opentelemetry.io/otel/trace v1.38.0
	golang.org/x/exp v0.0.0-20250811191247-51f88131bc50
	golang.org/x/mod v0.29.0
	golang.org/x/oauth2 v0.30.0
	golang.org/x/time v0.14.0
	google.golang.org/grpc v1.75.1
	google.golang.org/protobuf v1.36.10
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bsm/redislock v0.9.4 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/colorprofile v0.3.1 // indirect
	github.com/charmbracelet/harmonica v0.2.0 // indirect
	github.com/charmbracelet/x/ansi v0.10.1 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.13 // indirect
	github.com/charmbracelet/x/term v0.2.1 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-farm v0.0.0-20240924180020-3414d57e47da // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.1 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.17.0 // indirect
	github.com/redis/go-redis/extra/rediscmd/v9 v9.12.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/uptrace/opentelemetry-go-extra/otelutil v0.3.2 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/log v0.13.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/proto/otlp v1.7.1 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250811230008-5f3141c8851a // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250811230008-5f3141c8851a // indirect
	gopkg.in/DATA-DOG/go-sqlmock.v1 v1.3.0 // indirect
)
