// Package cli provides the Cobra-based command-line interface for llm-mux.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	configaccess "github.com/nghyane/llm-mux/internal/access/config_access"
	authlogin "github.com/nghyane/llm-mux/internal/auth/login"
	"github.com/nghyane/llm-mux/internal/buildinfo"
	"github.com/nghyane/llm-mux/internal/cli/env"
	"github.com/nghyane/llm-mux/internal/config"
	"github.com/nghyane/llm-mux/internal/json"
	log "github.com/nghyane/llm-mux/internal/logging"
	"github.com/nghyane/llm-mux/internal/provider"
	"github.com/nghyane/llm-mux/internal/store"
	"github.com/nghyane/llm-mux/internal/usage"
	"github.com/nghyane/llm-mux/internal/util"
)

// BootstrapResult contains the result of bootstrapping the application.
type BootstrapResult struct {
	Config         *config.Config
	ConfigFilePath string
}

// Bootstrap initializes the application configuration and stores.
// It should be called before any command that needs access to config or auth stores.
func Bootstrap(configPath string) (*BootstrapResult, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Load environment variables from .env if present.
	if errLoad := godotenv.Load(filepath.Join(wd, ".env")); errLoad != nil {
		if !errors.Is(errLoad, os.ErrNotExist) {
			log.WithError(errLoad).Warn("failed to load .env file")
		}
	}

	storeCfg := store.ParseFromEnv(env.LookupEnv)

	xdgConfigDir, _ := util.ResolveAuthDir("$XDG_CONFIG_HOME/llm-mux")
	defaultConfigPath := filepath.Join(xdgConfigDir, "config.yaml")
	defaultAuthDir := filepath.Join(xdgConfigDir, "auth")

	var cfg *config.Config
	var configFilePath string
	var storeResult *store.StoreResult

	if storeCfg.IsConfigured() {
		storeCfg.TargetConfigPath = defaultConfigPath
		storeCfg.TargetAuthDir = defaultAuthDir

		ctx := context.Background()
		storeResult, err = store.NewStore(ctx, storeCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize store: %w", err)
		}

		configFilePath = storeResult.ConfigPath
		cfg, err = config.LoadConfigOptional(configFilePath, false)
		if err == nil && cfg != nil {
			cfg.AuthDir = storeResult.AuthDir
		}

		switch storeCfg.Type {
		case store.TypePostgres:
			log.Infof("postgres-backed token store enabled")
		case store.TypeObject:
			log.Infof("object-backed token store enabled, bucket: %s", storeCfg.Object.Bucket)
		case store.TypeGit:
			log.Infof("git-backed token store enabled")
		}
	} else if configPath != "" {
		if resolved, errResolve := util.ResolveAuthDir(configPath); errResolve == nil {
			configPath = resolved
		}
		configFilePath = configPath

		if configPath == defaultConfigPath {
			if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
				autoInitConfig(configPath)
			}
		}

		cfg, err = config.LoadConfigOptional(configPath, true)
	} else {
		configFilePath = filepath.Join(wd, "config.yaml")
		cfg, err = config.LoadConfigOptional(configFilePath, true)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	if cfg == nil {
		cfg = config.NewDefaultConfig()
	}

	applyEnvOverrides(cfg)

	usage.SetStatisticsEnabled(cfg.Usage.DSN != "")

	// Initialize usage persistence if enabled
	if cfg.Usage.DSN != "" {
		initUsageBackend(cfg)
	}

	provider.SetQuotaCooldownDisabled(cfg.DisableCooling)

	if resolvedAuthDir, errResolveAuthDir := util.ResolveAuthDir(cfg.AuthDir); errResolveAuthDir != nil {
		return nil, fmt.Errorf("failed to resolve auth directory: %w", errResolveAuthDir)
	} else {
		cfg.AuthDir = resolvedAuthDir
	}

	// Register the shared token store
	if storeResult != nil && storeResult.Store != nil {
		authlogin.RegisterTokenStore(storeResult.Store)
	} else {
		authlogin.RegisterTokenStore(authlogin.NewFileTokenStore())
	}

	// Register built-in access providers
	configaccess.Register()

	return &BootstrapResult{
		Config:         cfg,
		ConfigFilePath: configFilePath,
	}, nil
}

// initUsageBackend initializes the usage persistence backend.
func initUsageBackend(cfg *config.Config) {
	var flushInterval time.Duration
	if cfg.Usage.FlushInterval != "" {
		if d, parseErr := time.ParseDuration(cfg.Usage.FlushInterval); parseErr == nil {
			flushInterval = d
		}
	}
	if flushInterval == 0 {
		flushInterval = 5 * time.Second
	}
	batchSize := cfg.Usage.BatchSize
	if batchSize == 0 {
		batchSize = 100
	}
	retentionDays := cfg.Usage.RetentionDays
	if retentionDays == 0 {
		retentionDays = 30
	}
	backendCfg := usage.BackendConfig{
		DSN:           cfg.Usage.DSN,
		BatchSize:     batchSize,
		FlushInterval: flushInterval,
		RetentionDays: retentionDays,
	}
	if initErr := usage.Initialize(backendCfg); initErr != nil {
		log.Warnf("Failed to initialize usage backend: %v", initErr)
	} else {
		log.Infof("Usage backend initialized: %s", cfg.Usage.DSN)
	}
}

// autoInitConfig silently creates config on first run
func autoInitConfig(configPath string) {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	authDir := filepath.Join(dir, "auth")
	_ = os.MkdirAll(authDir, 0o700)
	if err := os.WriteFile(configPath, config.GenerateDefaultConfigYAML(), 0o600); err != nil {
		return
	}
	fmt.Printf("First run: created config at %s\n", configPath)
}

// applyEnvOverrides applies environment variable overrides for cloud deployment.
func applyEnvOverrides(cfg *config.Config) {
	if port, ok := env.LookupEnvInt("LLM_MUX_PORT"); ok {
		cfg.Port = port
		log.Infof("Port overridden by env: %d", port)
	}

	if debug, ok := env.LookupEnvBool("LLM_MUX_DEBUG"); ok {
		cfg.Debug = debug
		log.Infof("Debug overridden by env: %v", debug)
	}

	if disableAuth, ok := env.LookupEnvBool("LLM_MUX_DISABLE_AUTH"); ok {
		cfg.DisableAuth = disableAuth
		log.Infof("DisableAuth overridden by env: %v", disableAuth)
	}

	if keys, ok := env.LookupEnv("LLM_MUX_API_KEYS"); ok {
		cfg.APIKeys = nil
		for _, k := range strings.Split(keys, ",") {
			if trimmed := strings.TrimSpace(k); trimmed != "" {
				cfg.APIKeys = append(cfg.APIKeys, trimmed)
			}
		}
		log.Infof("API keys overridden by env: %d keys", len(cfg.APIKeys))
	}

	if dsn, ok := env.LookupEnv("LLM_MUX_USAGE_DSN"); ok {
		cfg.Usage.DSN = dsn
		log.Infof("Usage DSN overridden by env")
	}

	if days, ok := env.LookupEnvInt("LLM_MUX_USAGE_RETENTION_DAYS"); ok {
		cfg.Usage.RetentionDays = days
		log.Infof("Usage retention days overridden by env: %d", days)
	}

	if proxyURL, ok := env.LookupEnv("LLM_MUX_PROXY_URL"); ok {
		cfg.ProxyURL = proxyURL
		log.Infof("Proxy URL overridden by env")
	}

	if authDir, ok := env.LookupEnv("LLM_MUX_AUTH_DIR"); ok {
		cfg.AuthDir = authDir
		log.Infof("Auth dir overridden by env: %s", authDir)
	}

	if loggingToFile, ok := env.LookupEnvBool("LLM_MUX_LOGGING_TO_FILE"); ok {
		cfg.LoggingToFile = loggingToFile
		log.Infof("Logging to file overridden by env: %v", loggingToFile)
	}

	if retry, ok := env.LookupEnvInt("LLM_MUX_REQUEST_RETRY"); ok {
		cfg.RequestRetry = retry
		log.Infof("Request retry overridden by env: %d", retry)
	}

	if maxRetryInterval, ok := env.LookupEnvInt("LLM_MUX_MAX_RETRY_INTERVAL"); ok {
		cfg.MaxRetryInterval = maxRetryInterval
		log.Infof("Max retry interval overridden by env: %d", maxRetryInterval)
	}
}

// DoInitConfig handles the init command with smart behavior.
func DoInitConfig(configPath string, force bool) error {
	configPath, _ = util.ResolveAuthDir(configPath)
	dir := filepath.Dir(configPath)
	credPath := config.CredentialsFilePath()

	configExists := fileExists(configPath)
	credExists := fileExists(credPath)

	// Ensure config directory exists
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	_ = os.MkdirAll(filepath.Join(dir, "auth"), 0o700)

	// Create config if missing
	if !configExists {
		if err := os.WriteFile(configPath, config.GenerateDefaultConfigYAML(), 0o600); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}
		fmt.Printf("Created: %s\n", configPath)
	}

	// Handle credentials
	if credExists && !force {
		key := config.GetManagementKey()
		if key != "" {
			fmt.Printf("Management key: %s\n", key)
			fmt.Printf("Location: %s\n", credPath)
			fmt.Println("Use --init --force to regenerate")
			return nil
		}
	}

	// Generate new key
	key, err := config.CreateCredentials()
	if err != nil {
		return fmt.Errorf("failed to create credentials: %w", err)
	}

	if credExists && force {
		fmt.Println("Regenerated management key:")
	} else {
		fmt.Println("Generated management key:")
	}
	fmt.Printf("  %s\n", key)
	fmt.Printf("Location: %s\n", credPath)
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DoUpdate checks for updates and installs if available.
func DoUpdate(checkOnly bool) error {
	fmt.Println("Checking for updates...")

	latestVersion, err := fetchLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	currentVersion := strings.TrimPrefix(buildinfo.Version, "v")
	latestVersion = strings.TrimPrefix(latestVersion, "v")

	if currentVersion == "dev" || currentVersion == "" {
		fmt.Println("Running development version, updating to latest release...")
	} else if compareVersions(currentVersion, latestVersion) >= 0 {
		fmt.Printf("Already up to date (current: v%s, latest: v%s)\n", currentVersion, latestVersion)
		return nil
	} else {
		fmt.Printf("Update available: v%s -> v%s\n", currentVersion, latestVersion)
	}

	if checkOnly {
		return nil
	}

	fmt.Println("Downloading and installing update...")
	if err := runInstallScript(); err != nil {
		return fmt.Errorf("failed to install update: %w", err)
	}
	fmt.Println("Update complete! Please restart llm-mux.")
	return nil
}

func fetchLatestVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/nghyane/llm-mux/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &n1)
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &n2)
		}
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	return 0
}

func runInstallScript() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://raw.githubusercontent.com/nghyane/llm-mux/main/install.sh", nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to download install script: status %d", resp.StatusCode)
	}

	scriptContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp("", "llm-mux-install-*.sh")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(scriptContent); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		return err
	}

	cmd := exec.Command("bash", tmpFile.Name(), "--no-service", "--force")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
