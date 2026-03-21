package config

import "time"

// Config is the top-level PGPulse configuration.
type Config struct {
	Server             ServerConfig             `koanf:"server"`
	Storage            StorageConfig            `koanf:"storage"`
	Auth               AuthConfig               `koanf:"auth"`
	Alerting           AlertingConfig           `koanf:"alerting"`
	PlanCapture        PlanCaptureConfig        `koanf:"plan_capture"`
	SettingsSnapshot   SettingsSnapshotConfig   `koanf:"settings_snapshot"`
	StatementSnapshots StatementSnapshotsConfig `koanf:"statement_snapshots"`
	ML                 MLConfig                 `koanf:"ml"`
	Remediation        RemediationConfig        `koanf:"remediation"`
	RCA                RCAConfig                `koanf:"rca"`
	OSMetrics          OSMetricsConfig          `koanf:"os_metrics"`
	Instances          []InstanceConfig         `koanf:"instances"`
}

// RCAConfig holds root cause analysis engine settings (M14).
type RCAConfig struct {
	Enabled                 bool          `koanf:"enabled"`
	LookbackWindow          time.Duration `koanf:"lookback_window"`
	AutoTriggerSeverity     string        `koanf:"auto_trigger_severity"`
	MaxIncidentsPerHour     int           `koanf:"max_incidents_per_hour"`
	RetentionDays           int           `koanf:"retention_days"`
	MaxTraversalDepth       int           `koanf:"max_traversal_depth"`
	MaxCandidateChains      int           `koanf:"max_candidate_chains"`
	MaxMetricsPerRun        int           `koanf:"max_metrics_per_run"`
	MinEdgeScore            float64       `koanf:"min_edge_score"`
	MinChainScore           float64       `koanf:"min_chain_score"`
	DeferredForwardTail     time.Duration `koanf:"deferred_forward_tail"`
	QualityBannerEnabled    bool          `koanf:"quality_banner_enabled"`
	RemediationHooksEnabled bool          `koanf:"remediation_hooks_enabled"`
}

// StatementSnapshotsConfig holds PGSS snapshot capture settings (M11_01).
type StatementSnapshotsConfig struct {
	Enabled          bool          `koanf:"enabled"`
	Interval         time.Duration `koanf:"interval"`
	RetentionDays    int           `koanf:"retention_days"`
	CaptureOnStartup bool         `koanf:"capture_on_startup"`
	TopN             int           `koanf:"top_n"`
}

// RemediationConfig holds background advisor evaluation settings.
type RemediationConfig struct {
	Enabled            bool          `koanf:"enabled"`
	BackgroundInterval time.Duration `koanf:"background_interval"`
	RetentionDays      int           `koanf:"retention_days"`
}

// OSMetricsConfig holds global OS metrics collection settings.
type OSMetricsConfig struct {
	Method string `koanf:"method"` // "sql" (default), "agent", "disabled"
}

// AlertingConfig holds alerting engine settings.
type AlertingConfig struct {
	Enabled                 bool         `koanf:"enabled"`
	DefaultConsecutiveCount int          `koanf:"default_consecutive_count"`
	DefaultCooldownMinutes  int          `koanf:"default_cooldown_minutes"`
	DefaultChannels         []string     `koanf:"default_channels"`
	EvaluationTimeoutSec    int          `koanf:"evaluation_timeout_seconds"`
	HistoryRetentionDays    int          `koanf:"history_retention_days"`
	DashboardURL            string       `koanf:"dashboard_url"`
	Email                   *EmailConfig `koanf:"email"`
}

// EmailConfig holds SMTP email notification settings.
type EmailConfig struct {
	Host               string   `koanf:"host"`
	Port               int      `koanf:"port"`
	Username           string   `koanf:"username"`
	Password           string   `koanf:"password"`
	From               string   `koanf:"from"`
	Recipients         []string `koanf:"recipients"`
	TLSSkipVerify      bool     `koanf:"tls_skip_verify"`
	SendTimeoutSeconds int      `koanf:"send_timeout_seconds"`
}

// AuthConfig holds JWT authentication settings.
type AuthConfig struct {
	Enabled         bool                `koanf:"enabled"`           // default false
	JWTSecret       string              `koanf:"jwt_secret"`        // required when enabled
	RefreshSecret   string              `koanf:"refresh_secret"`    // separate secret for refresh tokens; defaults to jwt_secret
	AccessTokenTTL  time.Duration       `koanf:"access_token_ttl"`  // default 24h
	RefreshTokenTTL time.Duration       `koanf:"refresh_token_ttl"` // default 168h (7d)
	BcryptCost      int                 `koanf:"bcrypt_cost"`       // default 12
	InitialAdmin    *InitialAdminConfig `koanf:"initial_admin"`
}

// InitialAdminConfig holds credentials for the first admin user seeded on startup.
type InitialAdminConfig struct {
	Username string `koanf:"username"`
	Password string `koanf:"password"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Listen          string        `koanf:"listen"`
	Port            int           `koanf:"port"`
	LogLevel        string        `koanf:"log_level"`
	CORSEnabled     bool          `koanf:"cors_enabled"`
	CORSOrigin      string        `koanf:"cors_origin"`
	ReadTimeout     time.Duration `koanf:"read_timeout"`
	WriteTimeout    time.Duration `koanf:"write_timeout"`
	ShutdownTimeout time.Duration `koanf:"shutdown_timeout"`
}

// StorageConfig holds the PGPulse metadata storage settings.
type StorageConfig struct {
	DSN            string `koanf:"dsn"`
	UseTimescaleDB bool   `koanf:"use_timescaledb"`
	RetentionDays  int    `koanf:"retention_days"`
}

// InstanceConfig holds configuration for a single monitored PostgreSQL instance.
type InstanceConfig struct {
	ID          string         `koanf:"id"`
	Name        string         `koanf:"name"`
	DSN         string         `koanf:"dsn"`
	Description string         `koanf:"description"` // optional human-readable label
	Enabled     *bool          `koanf:"enabled"`     // pointer: nil means "not set" → default true
	MaxConns    int            `koanf:"max_conns"`   // max pool connections per instance; default 5
	Intervals   IntervalConfig `koanf:"intervals"`

	// OS Agent (M6)
	AgentURL string `koanf:"agent_url"` // optional: "http://host:9187"

	// Patroni (M6)
	PatroniURL     string `koanf:"patroni_url"`      // optional: "http://host:8008"
	PatroniConfig  string `koanf:"patroni_config"`   // optional: "/etc/patroni/patroni.yml"
	PatroniCtlPath string `koanf:"patroni_ctl_path"` // optional: "/usr/bin/patronictl"

	// ETCD (M6)
	ETCDEndpoints []string `koanf:"etcd_endpoints"` // optional: ["http://host:2379"]
	ETCDCtlPath   string   `koanf:"etcd_ctl_path"`  // optional: "/usr/local/bin/etcdctl"

	// Per-database analysis (M7)
	IncludeDatabases []string `koanf:"include_databases"`   // glob patterns; empty = include all
	ExcludeDatabases []string `koanf:"exclude_databases"`   // glob patterns; empty = exclude none
	MaxConcurrentDBs int      `koanf:"max_concurrent_dbs"`  // default 5 if zero

	// OS metrics method override (M8_11) — overrides global os_metrics.method for this instance.
	OSMetricsMethod string `koanf:"os_metrics_method"` // "sql", "agent", "disabled"; empty = use global
}

// AgentConfig holds configuration for the pgpulse-agent binary.
type AgentConfig struct {
	ListenAddr     string   `koanf:"listen_addr"`      // default: "0.0.0.0:9187"
	PatroniURL     string   `koanf:"patroni_url"`
	PatroniConfig  string   `koanf:"patroni_config"`
	PatroniCtlPath string   `koanf:"patroni_ctl_path"`
	ETCDEndpoints  []string `koanf:"etcd_endpoints"`
	ETCDCtlPath    string   `koanf:"etcd_ctl_path"`
	MountPoints    []string `koanf:"mount_points"` // empty = all non-tmpfs
}

// IntervalConfig defines collection frequency tiers for an instance.
type IntervalConfig struct {
	High   time.Duration `koanf:"high"`   // default 10s  — connections, locks, wait events
	Medium time.Duration `koanf:"medium"` // default 60s  — replication, statements, checkpoint
	Low    time.Duration `koanf:"low"`    // default 300s — server info, sizes, settings
}

// PlanCaptureConfig holds auto-capture plan settings (M8_02).
type PlanCaptureConfig struct {
	Enabled               bool          `koanf:"enabled"`
	DurationThresholdMs   int64         `koanf:"duration_threshold_ms"`
	DedupWindowSeconds    int           `koanf:"dedup_window_seconds"`
	ScheduledTopNCount    int           `koanf:"scheduled_topn_count"`
	ScheduledTopNInterval time.Duration `koanf:"scheduled_topn_interval"`
	MaxPlanBytes          int           `koanf:"max_plan_bytes"`
	RetentionDays         int           `koanf:"retention_days"`
}

// SettingsSnapshotConfig holds temporal settings snapshot settings (M8_02).
type SettingsSnapshotConfig struct {
	Enabled           bool          `koanf:"enabled"`
	ScheduledInterval time.Duration `koanf:"scheduled_interval"`
	CaptureOnStartup  bool          `koanf:"capture_on_startup"`
	RetentionDays     int           `koanf:"retention_days"`
}

// MLMetricConfig defines a single metric to track with ML anomaly detection.
type MLMetricConfig struct {
	Key             string `koanf:"key"`
	Period          int    `koanf:"period"`
	Enabled         bool   `koanf:"enabled"`
	ForecastHorizon int    `koanf:"forecast_horizon"`
}

// ForecastConfig holds forecast horizon settings (M8_04).
type ForecastConfig struct {
	Horizon             int     `koanf:"horizon"`
	ConfidenceZ         float64 `koanf:"confidence_z"`
	AlertMinConsecutive int     `koanf:"alert_min_consecutive"`
}

// MLConfig holds ML anomaly detection settings (M8_02).
type MLConfig struct {
	Enabled            bool                `koanf:"enabled"`
	ZScoreWarn         float64             `koanf:"zscore_threshold_warning"`
	ZScoreCrit         float64             `koanf:"zscore_threshold_critical"`
	AnomalyLogic       string              `koanf:"anomaly_logic"`
	CollectionInterval time.Duration       `koanf:"collection_interval"`
	Metrics            []MLMetricConfig    `koanf:"metrics"`
	Persistence        MLPersistenceConfig `koanf:"persistence"`
	Forecast           ForecastConfig      `koanf:"forecast"`
}

// MLPersistenceConfig holds ML baseline persistence settings (M8_03).
type MLPersistenceConfig struct {
	Enabled bool `koanf:"enabled"`
}
