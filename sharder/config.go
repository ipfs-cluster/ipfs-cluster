package sharder

import (
	"encoding/json"
	"errors"

	"github.com/ipfs/ipfs-cluster/config"
)

const configKey = "sharder"

// Default values for Config.
const (
	DefaultAllocSize = 5000000 // 5 MB, approx 20 standard ipfs chunks
	IPFSChunkSize = 262158
)

// Config allows to initialize a Sharder and customize some parameters
type Config struct {
	config.Saver

	// Bytes allocated in one sharding allocation round
	AllocSize uint64
}

type jsonConfig struct {
	AllocSize uint64 `json:"alloc_size"`
}

// ConfigKey provides a human-friendly identifer for this type of Config.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default sets the fields of this Config to sensible default values.
func (cfg *Config) Default() error {
	cfg.AllocSize = DefaultAllocSize
	return nil
}

// Validate checks that the fields of this Config have working values
func (cfg *Config) Validate() error {
	if cfg.AllocSize <= IPFSChunkSize {
		return errors.New("sharder.alloc_size lower than single file chunk")
	}
	return nil
}

// LoadJSON sets the fields of this Config to the values defined by the JSON
// representation.
func (cfg *Config) LoadJSON(raw []byte) error {
	jcfg := &jsonConfig{}
	err := json.Unmarshal(raw, jcfg)
	if err != nil {
		logger.Error("Error unmarshaling sharder config")
		return err
	}

	cfg.Default()

	config.SetIfNotDefault(jcfg.AllocSize, &cfg.AllocSize)

	return cfg.Validate()
}

// ToJSON generates a human-friendly JSON representation of this Config.
func (cfg *Config) ToJSON() ([]byte, error) {
	jcfg := &jsonConfig{}

	jcfg.AllocSize = cfg.AllocSize

	return config.DefaultJSONMarshal(jcfg)
}
