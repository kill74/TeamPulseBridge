package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	current  atomic.Pointer[Config]
	watcher  *fsnotify.Watcher
	logger   *slog.Logger
	onChange func(Config)
	stopCh   chan struct{}
	stopped  bool
}

func NewWatcher(cfg Config, logger *slog.Logger, onChange func(Config)) (*Watcher, error) {
	w := &Watcher{
		logger:   logger,
		onChange: onChange,
		stopCh:   make(chan struct{}),
	}
	w.current.Store(&cfg)

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create file watcher: %w", err)
	}
	w.watcher = fsWatcher

	return w, nil
}

func (w *Watcher) Start(path string) error {
	if path == "" {
		return errors.New("config path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("config file not found: %w", err)
	}

	if err := w.watcher.Add(filepath.Dir(absPath)); err != nil {
		return fmt.Errorf("watch config directory: %w", err)
	}

	go w.watchLoop(absPath)

	w.logger.Info("config watcher started", "path", absPath)
	return nil
}

func (w *Watcher) watchLoop(targetPath string) {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Name == targetPath && (event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create) {
				w.reload(targetPath)
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("config watcher error", "error", err)
		case <-w.stopCh:
			return
		}
	}
}

func (w *Watcher) reload(path string) {
	newCfg, err := LoadFromFile(path)
	if err != nil {
		w.logger.Error("config reload failed", "path", path, "error", err)
		return
	}

	if err := newCfg.Validate(); err != nil {
		w.logger.Error("reloaded config validation failed — keeping current config", "path", path, "error", err)
		return
	}

	current := w.Get()
	if configsEqual(current, newCfg) {
		w.logger.Debug("config unchanged, skipping reload", "path", path)
		return
	}

	w.current.Store(&newCfg)
	w.logger.Info("config reloaded successfully", "path", path)

	if w.onChange != nil {
		w.onChange(newCfg)
	}
}

func configsEqual(a, b Config) bool {
	return a.Environment == b.Environment &&
		a.Port == b.Port &&
		a.QueueBackend == b.QueueBackend &&
		a.QueueBuffer == b.QueueBuffer &&
		a.QueueWorkers == b.QueueWorkers &&
		a.RateLimitEnabled == b.RateLimitEnabled &&
		a.RateLimitRPM == b.RateLimitRPM &&
		a.DedupEnabled == b.DedupEnabled &&
		a.DedupTTLSeconds == b.DedupTTLSeconds &&
		a.SchemaValidationEnabled == b.SchemaValidationEnabled &&
		a.RetryEnabled == b.RetryEnabled &&
		a.RetryMaxAttempts == b.RetryMaxAttempts &&
		a.QueueBulkheadEnabled == b.QueueBulkheadEnabled &&
		a.QueueBackpressureEnabled == b.QueueBackpressureEnabled &&
		a.SourceRateLimitEnabled == b.SourceRateLimitEnabled &&
		a.PIIScrubbingEnabled == b.PIIScrubbingEnabled &&
		a.AdminAuthEnabled == b.AdminAuthEnabled &&
		a.RequestTimeoutSec == b.RequestTimeoutSec
}

func (w *Watcher) Get() Config {
	ptr := w.current.Load()
	if ptr == nil {
		return Config{}
	}
	return *ptr
}

func (w *Watcher) Stop() error {
	if w.stopped {
		return nil
	}
	w.stopped = true
	close(w.stopCh)
	return w.watcher.Close()
}

func LoadFromFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	cfg, err := ParseConfig(data)
	if err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

func ParseConfig(data []byte) (Config, error) {
	envOverrides := make(map[string]string)

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		envOverrides[key] = value
	}

	for key, value := range envOverrides {
		_ = os.Setenv(key, value)
	}

	return LoadFromEnv(), nil
}
