package config

// `resources:` — a CPU and memory ceiling for every command (#384). Resolved
// and applied by internal/resources; the older keys stay as aliases:
// `mem_limit` is resources.memory when the block leaves it unset, and
// `build_workers` still sets the worker count, capped at resources.cpu.

// ResourcesConfig is the `resources:` block.
type ResourcesConfig struct {
	// CPU caps GOMAXPROCS and the worker pools (pages, images, WebP/AVIF,
	// TTS). 0 = no cap; Go already fits a cgroup CPU limit by itself.
	CPU int `yaml:"cpu" toml:"cpu" json:"cpu"`
	// Memory is a soft limit for the whole process, e.g. "512MiB". Unset
	// fits 90% of a cgroup memory limit; "none" turns that off.
	Memory string `yaml:"memory" toml:"memory" json:"memory"`
	// GCPercent trades heap headroom for collector work; nil = Go's 100.
	GCPercent *int `yaml:"gc_percent" toml:"gc_percent" json:"gc_percent"`
}
