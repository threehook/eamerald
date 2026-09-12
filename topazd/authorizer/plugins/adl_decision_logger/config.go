package adl_decision_logger

type Config struct {
	Enabled bool `json:"enabled"`
}

func defaultConfig() *Config {
	return &Config{
		Enabled: false,
	}
}
