// Package cli provides the Cobra-based command-line interface for llm-mux.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	configaccess "github.com/nghyane/llm-mux/internal/access/config_access"
	authlogin "github.com/nghyane/llm-mux/internal/auth/login"
	"github.com/nghyane/llm-mux/internal/buildinfo"
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

	lookupEnv := func(keys ...string) (string, bool) {
		for _, key := range keys {
			if value, ok := os.LookupEnv(key); ok {
				if trimmed := strings.TrimSpace(value); trimmed != "" {
					return trimmed, true
				}
			}
		}
		return "", false
	}

	var (
		usePostgresStore     bool
		pgStoreDSN           string
		pgStoreSchema        string
		pgStoreLocalPath     string
		pgStoreInst          *store.PostgresStore
		useGitStore          bool
		gitStoreRemoteURL    string
		gitStoreUser         string
		gitStorePassword     string
		gitStoreLocalPath    string
		gitStoreInst         *store.GitTokenStore
		gitStoreRoot         string
		useObjectStore       bool
		objectStoreEndpoint  string
		objectStoreAccess    string
		objectStoreSecret    string
		objectStoreBucket    string
		objectStoreLocalPath string
		objectStoreInst      *store.ObjectTokenStore
	)

	writableBase := util.WritablePath()

	// Check for Postgres store
	if value, ok := lookupEnv("PGSTORE_DSN", "pgstore_dsn"); ok {
		usePostgresStore = true
		pgStoreDSN = value
	}
	if usePostgresStore {
		if value, ok := lookupEnv("PGSTORE_SCHEMA", "pgstore_schema"); ok {
			pgStoreSchema = value
		}
		if value, ok := lookupEnv("PGSTORE_LOCAL_PATH", "pgstore_local_path"); ok {
			pgStoreLocalPath = value
		}
		if pgStoreLocalPath == "" {
			if writableBase != "" {
				pgStoreLocalPath = writableBase
			} else {
				pgStoreLocalPath = wd
			}
		}
		useGitStore = false
	}

	// Check for Git store
	if value, ok := lookupEnv("GITSTORE_GIT_URL", "gitstore_git_url"); ok {
		useGitStore = true
		gitStoreRemoteURL = value
	}
	if value, ok := lookupEnv("GITSTORE_GIT_USERNAME", "gitstore_git_username"); ok {
		gitStoreUser = value
	}
	if value, ok := lookupEnv("GITSTORE_GIT_TOKEN", "gitstore_git_token"); ok {
		gitStorePassword = value
	}
	if value, ok := lookupEnv("GITSTORE_LOCAL_PATH", "gitstore_local_path"); ok {
		gitStoreLocalPath = value
	}

	// Check for Object store
	if value, ok := lookupEnv("OBJECTSTORE_ENDPOINT", "objectstore_endpoint"); ok {
		useObjectStore = true
		objectStoreEndpoint = value
	}
	if value, ok := lookupEnv("OBJECTSTORE_ACCESS_KEY", "objectstore_access_key"); ok {
		objectStoreAccess = value
	}
	if value, ok := lookupEnv("OBJECTSTORE_SECRET_KEY", "objectstore_secret_key"); ok {
		objectStoreSecret = value
	}
	if value, ok := lookupEnv("OBJECTSTORE_BUCKET", "objectstore_bucket"); ok {
		objectStoreBucket = value
	}
	if value, ok := lookupEnv("OBJECTSTORE_LOCAL_PATH", "objectstore_local_path"); ok {
		objectStoreLocalPath = value
	}

	var cfg *config.Config
	var configFilePath string

	if usePostgresStore {
		if pgStoreLocalPath == "" {
			pgStoreLocalPath = wd
		}
		pgStoreLocalPath = filepath.Join(pgStoreLocalPath, "pgstore")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		pgStoreInst, err = store.NewPostgresStore(ctx, store.PostgresStoreConfig{
			DSN:      pgStoreDSN,
			Schema:   pgStoreSchema,
			SpoolDir: pgStoreLocalPath,
		})
		cancel()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize postgres token store: %w", err)
		}
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		if errBootstrap := pgStoreInst.Bootstrap(ctx); errBootstrap != nil {
			cancel()
			return nil, fmt.Errorf("failed to bootstrap postgres-backed config: %w", errBootstrap)
		}
		cancel()
		configFilePath = pgStoreInst.ConfigPath()
		cfg, err = config.LoadConfigOptional(configFilePath, false)
		if err == nil {
			cfg.AuthDir = pgStoreInst.AuthDir()
			log.Infof("postgres-backed token store enabled, workspace path: %s", pgStoreInst.WorkDir())
		}
	} else if useObjectStore {
		if objectStoreLocalPath == "" {
			if writableBase != "" {
				objectStoreLocalPath = writableBase
			} else {
				objectStoreLocalPath = wd
			}
		}
		objectStoreRoot := filepath.Join(objectStoreLocalPath, "objectstore")
		resolvedEndpoint := strings.TrimSpace(objectStoreEndpoint)
		useSSL := true
		if strings.Contains(resolvedEndpoint, "://") {
			parsed, errParse := url.Parse(resolvedEndpoint)
			if errParse != nil {
				return nil, fmt.Errorf("failed to parse object store endpoint %q: %w", objectStoreEndpoint, errParse)
			}
			switch strings.ToLower(parsed.Scheme) {
			case "http":
				useSSL = false
			case "https":
				useSSL = true
			default:
				return nil, fmt.Errorf("unsupported object store scheme %q (only http and https are allowed)", parsed.Scheme)
			}
			if parsed.Host == "" {
				return nil, fmt.Errorf("object store endpoint %q is missing host information", objectStoreEndpoint)
			}
			resolvedEndpoint = parsed.Host
			if parsed.Path != "" && parsed.Path != "/" {
				resolvedEndpoint = strings.TrimSuffix(parsed.Host+parsed.Path, "/")
			}
		}
		resolvedEndpoint = strings.TrimRight(resolvedEndpoint, "/")
		objCfg := store.ObjectStoreConfig{
			Endpoint:  resolvedEndpoint,
			Bucket:    objectStoreBucket,
			AccessKey: objectStoreAccess,
			SecretKey: objectStoreSecret,
			LocalRoot: objectStoreRoot,
			UseSSL:    useSSL,
			PathStyle: true,
		}
		objectStoreInst, err = store.NewObjectTokenStore(objCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize object token store: %w", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if errBootstrap := objectStoreInst.Bootstrap(ctx); errBootstrap != nil {
			cancel()
			return nil, fmt.Errorf("failed to bootstrap object-backed config: %w", errBootstrap)
		}
		cancel()
		configFilePath = objectStoreInst.ConfigPath()
		cfg, err = config.LoadConfigOptional(configFilePath, false)
		if err == nil {
			if cfg == nil {
				cfg = &config.Config{}
			}
			cfg.AuthDir = objectStoreInst.AuthDir()
			log.Infof("object-backed token store enabled, bucket: %s", objectStoreBucket)
		}
	} else if useGitStore {
		if gitStoreLocalPath == "" {
			if writableBase != "" {
				gitStoreLocalPath = writableBase
			} else {
				gitStoreLocalPath = wd
			}
		}
		gitStoreRoot = filepath.Join(gitStoreLocalPath, "gitstore")
		authDir := filepath.Join(gitStoreRoot, "auths")
		gitStoreInst = store.NewGitTokenStore(gitStoreRemoteURL, gitStoreUser, gitStorePassword)
		gitStoreInst.SetBaseDir(authDir)
		if errRepo := gitStoreInst.EnsureRepository(); errRepo != nil {
			return nil, fmt.Errorf("failed to prepare git token store: %w", errRepo)
		}
		configFilePath = gitStoreInst.ConfigPath()
		if configFilePath == "" {
			configFilePath = filepath.Join(gitStoreRoot, "config", "config.yaml")
		}
		if _, statErr := os.Stat(configFilePath); errors.Is(statErr, fs.ErrNotExist) {
			if errDir := os.MkdirAll(filepath.Dir(configFilePath), 0o700); errDir != nil {
				return nil, fmt.Errorf("failed to create config directory: %w", errDir)
			}
			if errWrite := os.WriteFile(configFilePath, config.GenerateDefaultConfigYAML(), 0o600); errWrite != nil {
				return nil, fmt.Errorf("failed to write config from template: %w", errWrite)
			}
			if errCommit := gitStoreInst.PersistConfig(context.Background()); errCommit != nil {
				return nil, fmt.Errorf("failed to commit initial git-backed config: %w", errCommit)
			}
			log.Infof("git-backed config initialized from template: %s", configFilePath)
		} else if statErr != nil {
			return nil, fmt.Errorf("failed to inspect git-backed config: %w", statErr)
		}
		cfg, err = config.LoadConfigOptional(configFilePath, false)
		if err == nil {
			cfg.AuthDir = gitStoreInst.AuthDir()
			log.Infof("git-backed token store enabled, repository path: %s", gitStoreRoot)
		}
	} else if configPath != "" {
		// Expand environment variables and ~ to absolute path
		if resolved, errResolve := util.ResolveAuthDir(configPath); errResolve == nil {
			configPath = resolved
		}
		configFilePath = configPath

		// Auto-init on first run
		defaultExpanded, _ := util.ResolveAuthDir("$XDG_CONFIG_HOME/llm-mux/config.yaml")
		if configPath == defaultExpanded {
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

	provider.SetQuotaCooldownDisabled(cfg.DisableCooling)

	if resolvedAuthDir, errResolveAuthDir := util.ResolveAuthDir(cfg.AuthDir); errResolveAuthDir != nil {
		return nil, fmt.Errorf("failed to resolve auth directory: %w", errResolveAuthDir)
	} else {
		cfg.AuthDir = resolvedAuthDir
	}

	// Register the shared token store
	if usePostgresStore {
		authlogin.RegisterTokenStore(pgStoreInst)
	} else if useObjectStore {
		authlogin.RegisterTokenStore(objectStoreInst)
	} else if useGitStore {
		authlogin.RegisterTokenStore(gitStoreInst)
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
	lookupEnv := func(keys ...string) (string, bool) {
		for _, key := range keys {
			if value, ok := os.LookupEnv(key); ok {
				if trimmed := strings.TrimSpace(value); trimmed != "" {
					return trimmed, true
				}
			}
		}
		return "", false
	}

	lookupEnvInt := func(keys ...string) (int, bool) {
		if value, ok := lookupEnv(keys...); ok {
			if n, err := strconv.Atoi(value); err == nil {
				return n, true
			}
		}
		return 0, false
	}

	lookupEnvBool := func(keys ...string) (bool, bool) {
		if value, ok := lookupEnv(keys...); ok {
			v := strings.ToLower(value)
			return v == "true" || v == "1" || v == "yes", true
		}
		return false, false
	}

	if port, ok := lookupEnvInt("LLM_MUX_PORT"); ok {
		cfg.Port = port
		log.Infof("Port overridden by env: %d", port)
	}

	if debug, ok := lookupEnvBool("LLM_MUX_DEBUG"); ok {
		cfg.Debug = debug
		log.Infof("Debug overridden by env: %v", debug)
	}

	if disableAuth, ok := lookupEnvBool("LLM_MUX_DISABLE_AUTH"); ok {
		cfg.DisableAuth = disableAuth
		log.Infof("DisableAuth overridden by env: %v", disableAuth)
	}

	if keys, ok := lookupEnv("LLM_MUX_API_KEYS"); ok {
		cfg.APIKeys = nil
		for _, k := range strings.Split(keys, ",") {
			if trimmed := strings.TrimSpace(k); trimmed != "" {
				cfg.APIKeys = append(cfg.APIKeys, trimmed)
			}
		}
		log.Infof("API keys overridden by env: %d keys", len(cfg.APIKeys))
	}

	if dsn, ok := lookupEnv("LLM_MUX_USAGE_DSN"); ok {
		cfg.Usage.DSN = dsn
		log.Infof("Usage DSN overridden by env")
	}

	if days, ok := lookupEnvInt("LLM_MUX_USAGE_RETENTION_DAYS"); ok {
		cfg.Usage.RetentionDays = days
		log.Infof("Usage retention days overridden by env: %d", days)
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
