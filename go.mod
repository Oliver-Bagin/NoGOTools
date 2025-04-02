module github.com/tinygo-org/tinygo/x-tools

go 1.23.0

require (
	github.com/google/go-cmp v0.6.0
	github.com/tinygo-org/tinygo v0.0.0-00010101000000-000000000000
	github.com/yuin/goldmark v1.4.13
	golang.org/x/mod v0.24.0
	golang.org/x/net v0.37.0
	golang.org/x/sync v0.12.0
	golang.org/x/telemetry v0.0.0-20240521205824-bda55230c457
)

require golang.org/x/sys v0.31.0 // indirect

replace github.com/tinygo-org/tinygo => ../
