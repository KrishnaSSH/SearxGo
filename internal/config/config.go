package config

import (
    "fmt"
    "os"
    "sort"
    "time"

    "github.com/BurntSushi/toml"
    en "searxgo/internal/engine"
    "searxgo/internal/engines"
)

// Config represents the main configuration structure
type Config struct {
    Server  ServerConfig            `toml:"server"`
    Engines map[string]EngineConfig `toml:"engines"`
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
    Port        int    `toml:"port"`
    Timeout     string `toml:"timeout"`
    StaticDir   string `toml:"static_dir"`
    DefaultSize int    `toml:"default_size"`
}

// EngineConfig holds configuration for individual search engines
type EngineConfig struct {
    Enabled  bool `toml:"enabled"`
    Priority int  `toml:"priority"`
}

// EngineInfo holds engine information for sorting
type EngineInfo struct {
    Name     string
    Engine   en.SearchEngine
    Priority int
}

// LoadConfig loads configuration from TOML file
func LoadConfig(filename string) (*Config, error) {
    // Set defaults
    config := &Config{
        Server: ServerConfig{
            Port:        9000,
            Timeout:     "5s",
            StaticDir:   "static",
            DefaultSize: 40,
        },
        Engines: make(map[string]EngineConfig),
    }

    // Check if config file exists
    if _, err := os.Stat(filename); os.IsNotExist(err) {
        fmt.Printf("Config file %s not found, using defaults\n", filename)
        return config, nil
    }

    // Load from file
    if _, err := toml.DecodeFile(filename, config); err != nil {
        return nil, fmt.Errorf("failed to load config: %v", err)
    }

    return config, nil
}

// GetTimeout returns the server timeout as a time.Duration
func (c *Config) GetTimeout() time.Duration {
    if duration, err := time.ParseDuration(c.Server.Timeout); err == nil {
        return duration
    }
    return 5 * time.Second // fallback default
}

// LoadEnabledEngines returns a slice of enabled search engines sorted by priority
func (c *Config) LoadEnabledEngines() []en.SearchEngine {
    var engineInfos []EngineInfo

    // Check each engine configuration
    for name, config := range c.Engines {
        if !config.Enabled {
            continue
        }

        var engine en.SearchEngine
        switch name {
        case "bing":
            engine = engines.NewBing()
        case "google":
            engine = engines.NewGoogle()
        case "duckduckgo":
            engine = engines.NewDuckDuckGo()
        case "wikipedia":
            engine = engines.NewWikipedia()
        case "reddit":
            engine = engines.NewReddit()
        default:
            fmt.Printf("Warning: Unknown engine '%s' in config\n", name)
            continue
        }

        engineInfos = append(engineInfos, EngineInfo{
            Name:     name,
            Engine:   engine,
            Priority: config.Priority,
        })
    }

    // Sort by priority (lower numbers = higher priority)
    sort.Slice(engineInfos, func(i, j int) bool {
        return engineInfos[i].Priority < engineInfos[j].Priority
    })

    // Extract engines in priority order
    var engines []en.SearchEngine
    for _, info := range engineInfos {
        engines = append(engines, info.Engine)
        fmt.Printf("Loaded engine: %s (priority: %d)\n", info.Name, info.Priority)
    }

    if len(engines) == 0 {
        fmt.Println("Warning: No engines enabled in configuration")
    }

    return engines
}

// Legacy function for backward compatibility
func Default() Config {
    return Config{
        Server: ServerConfig{
            Port:        9000,
            Timeout:     "5s",
            StaticDir:   "static",
            DefaultSize: 40,
        },
        Engines: make(map[string]EngineConfig),
    }
}
