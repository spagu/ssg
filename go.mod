module github.com/spagu/ssg

// Requires go1.26.5+: earlier 1.26.x stdlib is affected by GO-2026-5856
// (crypto/tls ECH privacy leak) and GO-2026-4970 (os), both fixed in go1.26.5.
go 1.26.5

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/aymerick/raymond v2.0.2+incompatible
	github.com/cbroglie/mustache v1.4.0
	github.com/flosch/pongo2/v6 v6.0.0
	github.com/yuin/goldmark v1.8.2
	github.com/yuin/goldmark-highlighting/v2 v2.0.0-20230729083705-37449abec8cc
	golang.org/x/net v0.57.0
	google.golang.org/grpc v1.79.3
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/alecthomas/chroma/v2 v2.27.0 // indirect
	github.com/dlclark/regexp2/v2 v2.2.1 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260330182312-d5a96adf58d8 // indirect
)
