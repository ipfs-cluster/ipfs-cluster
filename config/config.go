// Package config provides interfaces and utilities for different Cluster
// components to register, read, write and validate configuration sections
// stored in a central configuration file.
package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"

	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("config")

// ConfigSaveInterval specifies how often to save the configuration file if
// it needs saving.
var ConfigSaveInterval = time.Second

// The ComponentConfig interface allows components to define configurations
// which can be managed as part of the ipfs-cluster configuration file by the
// Manager.
type ComponentConfig interface {
	// Returns a string identifying the section name for this configuration
	ConfigKey() string
	// Parses a JSON representation of this configuration
	LoadJSON([]byte) error
	// Provides a JSON representation of this configuration
	ToJSON() ([]byte, error)
	// Sets default working values
	Default() error
	// Sets values from environment variables
	ApplyEnvVars() error
	// Allows this component to work under a subfolder
	SetBaseDir(string)
	// Checks that the configuration is valid
	Validate() error
	// Provides a channel to signal the Manager that the configuration
	// should be persisted.
	SaveCh() <-chan struct{}
}

// These are the component configuration types
// supported by the Manager.
const (
	Cluster SectionType = iota
	Consensus
	API
	IPFSConn
	State
	PinTracker
	Monitor
	Allocator
	Informer
	Observations
	Datastore
	endTypes // keep this at the end
)

// SectionType specifies to which section a component configuration belongs.
type SectionType int

// SectionTypes returns the list of supported SectionTypes
func SectionTypes() []SectionType {
	var l []SectionType
	for i := Cluster; i < endTypes; i++ {
		l = append(l, i)
	}
	return l
}

// Section is a section of which stores
// component-specific configurations.
type Section map[string]ComponentConfig

// jsonSection stores component specific
// configurations. Component configurations depend on
// components themselves.
type jsonSection map[string]*json.RawMessage

// Manager represents an ipfs-cluster configuration which bundles
// different ComponentConfigs object together.
// Use RegisterComponent() to add a component configurations to the
// object. Once registered, configurations will be parsed from the
// central configuration file when doing LoadJSON(), and saved to it
// when doing SaveJSON().
type Manager struct {
	ctx    context.Context
	cancel func()
	wg     sync.WaitGroup

	// The Cluster configuration has a top-level
	// special section.
	clusterConfig ComponentConfig

	// Holds configuration objects for components.
	sections map[SectionType]Section

	// store originally parsed jsonConfig
	jsonCfg *jsonConfig

	// map of components which has empty configuration
	// in JSON file
	undefinedComps map[SectionType]map[string]bool

	// if a config has been loaded from disk, track the path
	// so it can be saved to the same place.
	path    string
	saveMux sync.Mutex
}

// NewManager returns a correctly initialized Manager
// which is ready to accept component configurations.
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		ctx:            ctx,
		cancel:         cancel,
		undefinedComps: make(map[SectionType]map[string]bool),
		sections:       make(map[SectionType]Section),
	}

}

// Shutdown makes sure all configuration save operations are finished
// before returning.
func (cfg *Manager) Shutdown() {
	cfg.cancel()
	cfg.wg.Wait()
}

// this watches a save channel which is used to signal that
// we need to store changes in the configuration.
// because saving can be called too much, we will only
// save at intervals of 1 save/second at most.
func (cfg *Manager) watchSave(save <-chan struct{}) {
	defer cfg.wg.Done()

	// Save once per second mostly
	ticker := time.NewTicker(ConfigSaveInterval)
	defer ticker.Stop()

	thingsToSave := false

	for {
		select {
		case <-save:
			thingsToSave = true
		case <-ticker.C:
			if thingsToSave {
				err := cfg.SaveJSON("")
				if err != nil {
					logger.Error(err)
				}
				thingsToSave = false
			}

			// Exit if we have to
			select {
			case <-cfg.ctx.Done():
				return
			default:
			}
		}
	}
}

// jsonConfig represents a Cluster configuration as it will look when it is
// saved using json. Most configuration keys are converted into simple types
// like strings, and key names aim to be self-explanatory for the user.
type jsonConfig struct {
	Cluster      *json.RawMessage `json:"cluster"`
	Consensus    jsonSection      `json:"consensus,omitempty"`
	API          jsonSection      `json:"api,omitempty"`
	IPFSConn     jsonSection      `json:"ipfs_connector,omitempty"`
	State        jsonSection      `json:"state,omitempty"`
	PinTracker   jsonSection      `json:"pin_tracker,omitempty"`
	Monitor      jsonSection      `json:"monitor,omitempty"`
	Allocator    jsonSection      `json:"allocator,omitempty"`
	Informer     jsonSection      `json:"informer,omitempty"`
	Observations jsonSection      `json:"observations,omitempty"`
	Datastore    jsonSection      `json:"datastore,omitempty"`
}

func (jcfg *jsonConfig) getSection(i SectionType) *jsonSection {
	switch i {
	case Consensus:
		return &jcfg.Consensus
	case API:
		return &jcfg.API
	case IPFSConn:
		return &jcfg.IPFSConn
	case State:
		return &jcfg.State
	case PinTracker:
		return &jcfg.PinTracker
	case Monitor:
		return &jcfg.Monitor
	case Allocator:
		return &jcfg.Allocator
	case Informer:
		return &jcfg.Informer
	case Observations:
		return &jcfg.Observations
	case Datastore:
		return &jcfg.Datastore
	default:
		return nil
	}
}

// Default generates a default configuration by generating defaults for all
// registered components.
func (cfg *Manager) Default() error {
	for _, section := range cfg.sections {
		for k, compcfg := range section {
			logger.Debugf("generating default conf for %s", k)
			err := compcfg.Default()
			if err != nil {
				return err
			}
		}
	}
	if cfg.clusterConfig != nil {
		logger.Debug("generating default conf for cluster")
		err := cfg.clusterConfig.Default()
		if err != nil {
			return err
		}
	}
	return nil
}

// ApplyEnvVars overrides configuration fields with any values found
// in environment variables.
func (cfg *Manager) ApplyEnvVars() error {
	for _, section := range cfg.sections {
		for k, compcfg := range section {
			logger.Debugf("applying environment variables conf for %s", k)
			err := compcfg.ApplyEnvVars()
			if err != nil {
				return err
			}
		}
	}

	if cfg.clusterConfig != nil {
		logger.Debugf("applying environment variables conf for cluster")
		err := cfg.clusterConfig.ApplyEnvVars()
		if err != nil {
			return err
		}
	}

	return nil
}

// RegisterComponent lets the Manager load and save component configurations
func (cfg *Manager) RegisterComponent(t SectionType, ccfg ComponentConfig) {
	cfg.wg.Add(1)
	go cfg.watchSave(ccfg.SaveCh())

	if t == Cluster {
		cfg.clusterConfig = ccfg
		return
	}

	if cfg.sections == nil {
		cfg.sections = make(map[SectionType]Section)
	}

	_, ok := cfg.sections[t]
	if !ok {
		cfg.sections[t] = make(Section)
	}

	cfg.sections[t][ccfg.ConfigKey()] = ccfg

	_, ok = cfg.undefinedComps[t]
	if !ok {
		cfg.undefinedComps[t] = make(map[string]bool)
	}
}

// Validate checks that all the registered components in this
// Manager have valid configurations. It also makes sure that
// the main Cluster compoenent exists.
func (cfg *Manager) Validate() error {
	if cfg.clusterConfig == nil {
		return errors.New("no registered cluster section")
	}

	if cfg.sections == nil {
		return errors.New("no registered components")
	}

	err := cfg.clusterConfig.Validate()
	if err != nil {
		return fmt.Errorf("cluster section failed to validate: %s", err)
	}

	for t, section := range cfg.sections {
		if section == nil {
			return fmt.Errorf("section %d is nil", t)
		}
		for k, compCfg := range section {
			if compCfg == nil {
				return fmt.Errorf("%s entry for section %d is nil", k, t)
			}
			err := compCfg.Validate()
			if err != nil {
				return fmt.Errorf("%s failed to validate: %s", k, err)
			}
		}
	}
	return nil
}

// LoadJSONFromFile reads a Configuration file from disk and parses
// it. See LoadJSON too.
func (cfg *Manager) LoadJSONFromFile(path string) error {
	cfg.path = path

	file, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Error("error reading the configuration file: ", err)
		return err
	}

	err = cfg.LoadJSON(file)
	return err
}

// LoadJSONFileAndEnv calls LoadJSONFromFile followed by ApplyEnvVars,
// reading and parsing a Configuration file and then overriding fields
// with any values found in environment variables.
func (cfg *Manager) LoadJSONFileAndEnv(path string) error {
	if err := cfg.LoadJSONFromFile(path); err != nil {
		return err
	}

	return cfg.ApplyEnvVars()
}

// LoadJSON parses configurations for all registered components,
// In order to work, component configurations must have been registered
// beforehand with RegisterComponent.
func (cfg *Manager) LoadJSON(bs []byte) error {
	dir := filepath.Dir(cfg.path)

	jcfg := &jsonConfig{}
	err := json.Unmarshal(bs, jcfg)
	if err != nil {
		logger.Error("error parsing JSON: ", err)
		return err
	}

	cfg.jsonCfg = jcfg

	// Load Cluster section. Needs to have been registered
	if cfg.clusterConfig != nil && jcfg.Cluster != nil {
		cfg.clusterConfig.SetBaseDir(dir)
		err = cfg.clusterConfig.LoadJSON([]byte(*jcfg.Cluster))
		if err != nil {
			return err
		}
	}

	loadCompJSON := func(name string, component ComponentConfig, jsonSection jsonSection, t SectionType) error {
		raw, ok := jsonSection[name]
		if ok {
			component.SetBaseDir(dir)
			err := component.LoadJSON([]byte(*raw))
			if err != nil {
				return err
			}
			logger.Debugf("%s component configuration loaded", name)
		} else {
			cfg.undefinedComps[t][name] = true
			logger.Warningf("%s component is empty, generating default", name)
			component.SetBaseDir(dir)
			component.Default()
		}

		return nil
	}
	// Helper function to load json from each section in the json config
	loadSectionJSON := func(section Section, jsonSection jsonSection, t SectionType) error {
		for name, component := range section {
			err := loadCompJSON(name, component, jsonSection, t)
			if err != nil {
				logger.Error(err)
				return err
			}
		}
		return nil

	}

	sections := cfg.sections

	for _, t := range SectionTypes() {
		if t == Cluster {
			continue
		}
		err := loadSectionJSON(sections[t], *jcfg.getSection(t), t)
		if err != nil {
			return err
		}
	}

	return cfg.Validate()
}

// SaveJSON saves the JSON representation of the Config to
// the given path.
func (cfg *Manager) SaveJSON(path string) error {
	cfg.saveMux.Lock()
	defer cfg.saveMux.Unlock()

	logger.Info("Saving configuration")

	if path == "" {
		path = cfg.path
	}

	bs, err := cfg.ToJSON()
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, bs, 0600)
}

// ToJSON provides a JSON representation of the configuration by
// generating JSON for all componenents registered.
func (cfg *Manager) ToJSON() ([]byte, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	jcfg := cfg.jsonCfg
	if jcfg == nil {
		jcfg = &jsonConfig{}
	}

	if cfg.clusterConfig != nil {
		raw, err := cfg.clusterConfig.ToJSON()

		if err != nil {
			return nil, err
		}
		jcfg.Cluster = new(json.RawMessage)
		*jcfg.Cluster = raw
		logger.Debug("writing changes for cluster section")
	}

	// Given a Section and a *jsonSection, it updates the
	// component-configurations in the latter.
	updateJSONConfigs := func(section Section, dest *jsonSection) error {
		for k, v := range section {
			logger.Debugf("writing changes for %s section", k)
			j, err := v.ToJSON()
			if err != nil {
				return err
			}
			if *dest == nil {
				*dest = make(jsonSection)
			}
			jsonSection := *dest
			jsonSection[k] = new(json.RawMessage)
			*jsonSection[k] = j
		}
		return nil
	}

	for _, t := range SectionTypes() {
		if t == Cluster {
			continue
		}
		jsection := jcfg.getSection(t)
		err := updateJSONConfigs(cfg.sections[t], jsection)
		if err != nil {
			return nil, err
		}
	}

	return DefaultJSONMarshal(jcfg)
}

// IsLoadedFromJSON tells whether the given component belonging to
// the given section type is present in the cluster JSON
// config or not.
func (cfg *Manager) IsLoadedFromJSON(t SectionType, name string) bool {
	return !cfg.undefinedComps[t][name]
}

// GetClusterConfig extracts cluster config from the configuration file
// and returns bytes of it
func GetClusterConfig(configPath string) ([]byte, error) {
	file, err := ioutil.ReadFile(configPath)
	if err != nil {
		logger.Error("error reading the configuration file: ", err)
		return nil, err
	}

	jcfg := &jsonConfig{}
	err = json.Unmarshal(file, jcfg)
	if err != nil {
		logger.Error("error parsing JSON: ", err)
		return nil, err
	}
	return []byte(*jcfg.Cluster), nil
}
