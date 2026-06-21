package gmock

// Option configures a Server. Use the With* functions to create options.
type Option func(*ServerConfig)

// CORSOptions configures Cross-Origin Resource Sharing support.
type CORSOptions struct {
	// AllowedOrigins specifies which origins are allowed. Default: ["*"]
	AllowedOrigins []string
	// AllowedMethods specifies which methods are allowed. Default: standard HTTP methods
	AllowedMethods []string
	// AllowedHeaders specifies which headers are allowed. Default: ["Content-Type", "Authorization"]
	AllowedHeaders []string
	// ExposedHeaders specifies which headers are exposed to the browser.
	ExposedHeaders []string
	// AllowCredentials indicates whether credentials (cookies, auth) are allowed.
	AllowCredentials bool
	// MaxAge specifies how long the preflight result can be cached (seconds).
	MaxAge int
}

// ServerConfig holds all configurable server parameters.
type ServerConfig struct {
	// Port is the listen port. 0 means random port (recommended for tests).
	Port int

	// AdminPort enables a separate admin listener on this port.
	// 0 means admin API shares the main port under /__admin/ prefix.
	AdminPort int

	// ProxyURL enables proxy fallback for unmatched requests.
	ProxyURL string

	// RecordMode enables recording of proxied exchanges.
	RecordMode bool

	// StubFiles is a list of YAML/JSON files to load stubs from at startup.
	StubFiles []string

	// Verbose enables debug-level logging.
	Verbose bool

	// MaxRequests limits the number of logged requests (ring buffer).
	// Default is 1000.
	MaxRequests int

	// CORSOptions enables CORS support. nil means CORS is disabled.
	CORSOptions *CORSOptions

	// DisableGzip disables automatic gzip response compression.
	DisableGzip bool

	// RandSeed is the seed for the global pseudo-random number generator.
	// When set to a non-zero value, all chaos behavior (delays, fault injection,
	// probabilistic matching) produces identical sequences across runs.
	// When zero (default), the RNG is seeded from the current time, matching
	// the behavior of the unseeded math/rand global source.
	RandSeed int64
}

// WithPort sets the listen port. Use 0 for a random available port.
func WithPort(port int) Option {
	return func(c *ServerConfig) {
		c.Port = port
	}
}

// WithAdminPort enables a separate admin listener on the given port.
func WithAdminPort(port int) Option {
	return func(c *ServerConfig) {
		c.AdminPort = port
	}
}

// WithProxyURL enables proxy fallback for unmatched requests.
func WithProxyURL(url string) Option {
	return func(c *ServerConfig) {
		c.ProxyURL = url
	}
}

// WithRecordMode enables recording of proxied request/response pairs.
func WithRecordMode() Option {
	return func(c *ServerConfig) {
		c.RecordMode = true
	}
}

// WithStubFiles loads stubs from the specified YAML/JSON files at startup.
func WithStubFiles(files ...string) Option {
	return func(c *ServerConfig) {
		c.StubFiles = append(c.StubFiles, files...)
	}
}

// WithVerbose enables debug logging.
func WithVerbose() Option {
	return func(c *ServerConfig) {
		c.Verbose = true
	}
}

// WithMaxRequests sets the maximum number of logged requests.
func WithMaxRequests(n int) Option {
	return func(c *ServerConfig) {
		c.MaxRequests = n
	}
}

// DefaultConfig returns a ServerConfig with sensible defaults.
func DefaultConfig() ServerConfig {
	return ServerConfig{
		Port:        0,
		AdminPort:   0,
		MaxRequests: 1000,
	}
}

// WithCORS enables CORS support with the given options.
// Use WithCORSEnabled() for a quick default configuration.
func WithCORS(opts CORSOptions) Option {
	return func(c *ServerConfig) {
		c.CORSOptions = &opts
	}
}

// WithCORSEnabled enables CORS with permissive defaults (allow all origins).
func WithCORSEnabled() Option {
	return func(c *ServerConfig) {
		c.CORSOptions = &CORSOptions{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
			MaxAge:         86400,
		}
	}
}

// WithGzip enables or disables automatic gzip response compression.
func WithGzip(enabled bool) Option {
	return func(c *ServerConfig) {
		c.DisableGzip = !enabled
	}
}

// WithRandSeed sets the seed for the global pseudo-random number generator.
// A non-zero seed makes all chaos behavior (delays, fault injection,
// probabilistic matching) fully deterministic and reproducible across runs.
// Use 0 (the default) for non-deterministic behavior seeded from the clock.
func WithRandSeed(seed int64) Option {
	return func(c *ServerConfig) {
		c.RandSeed = seed
	}
}
