module github.com/spagu/ssg

// Requires go1.27.0+, which is what the language directive below asks for.
// The floor was go1.26.6 before that, and for a reason worth keeping: earlier
// 1.26.x stdlib carries seven advisories govulncheck finds reachable from this
// code — among them GO-2026-5972 (encoding/asn1, via ssh.ParsePrivateKey in the
// SFTP deploy) and GO-2026-5026 (net/http/idna, via every outbound request).
go 1.27.0

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/aymerick/raymond v2.0.2+incompatible
	github.com/cbroglie/mustache v1.4.0
	github.com/disintegration/imaging v1.6.2
	github.com/flosch/pongo2/v6 v6.1.0
	github.com/go-sql-driver/mysql v1.10.0
	github.com/jackc/pgx/v5 v5.10.0
	github.com/jlaffaye/ftp v0.2.2
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/pkg/sftp v1.13.11
	github.com/quic-go/quic-go v0.61.0
	github.com/ulikunitz/xz v0.5.16
	github.com/yuin/goldmark v1.8.5
	github.com/yuin/goldmark-highlighting/v2 v2.0.0-20230729083705-37449abec8cc
	golang.org/x/crypto v0.55.0
	golang.org/x/image v0.45.0
	golang.org/x/net v0.58.0
	golang.org/x/text v0.41.0
	google.golang.org/grpc v1.83.1
	google.golang.org/protobuf v1.36.12
	gopkg.in/yaml.v3 v3.0.1
	lukechampine.com/blake3 v1.4.1
	modernc.org/sqlite v1.57.0
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/dlclark/regexp2/v2 v2.2.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
