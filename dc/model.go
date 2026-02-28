package main

type Stack struct {
	Name       string          `json:"name"`
	Containers []DockerInspect `json:"containers"`
}

type ComposeFile struct {
	Services map[string]ComposeService `yaml:"services"`
	Volumes  map[string]ComposeVolume  `yaml:"volumes,omitempty"`
	Networks map[string]ComposeNetwork `yaml:"networks,omitempty"`
	Configs  map[string]ComposeConfig  `yaml:"configs,omitempty"`
	Secrets  map[string]ComposeSecret  `yaml:"secrets,omitempty"`
}

type ComposeVolume struct {
	External   bool              `yaml:"external,omitempty"`
	Name       string            `yaml:"name,omitempty"`
	Driver     string            `yaml:"driver,omitempty"`
	DriverOpts map[string]string `yaml:"driver_opts,omitempty"`
}

type ComposeNetwork struct {
	External   bool              `yaml:"external,omitempty"`
	Driver     string            `yaml:"driver,omitempty"`
	DriverOpts map[string]string `yaml:"driver_opts,omitempty"`
}

type ComposeConfig struct {
	Content string `yaml:"content,omitempty"`
	File    string `yaml:"file,omitempty"`
}

type ComposeSecret struct {
	Name        string `yaml:"name,omitempty"`
	Environment string `yaml:"environment,omitempty"`
	File        string `yaml:"file,omitempty"`
	External    bool   `yaml:"external,omitempty"`
}

type ComposeServiceConfig struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

type ComposeService struct {
	Image         string                 `yaml:"image"`
	ContainerName string                 `yaml:"container_name,omitempty"`
	User          string                 `yaml:"user,omitempty"`
	Restart       string                 `yaml:"restart,omitempty"`
	Volumes       []string               `yaml:"volumes,omitempty"`
	Ports         []string               `yaml:"ports,omitempty"`
	Environment   interface{}            `yaml:"environment,omitempty"` // Can be array or map
	Networks      interface{}            `yaml:"networks,omitempty"`    // Can be array or map
	Labels        interface{}            `yaml:"labels,omitempty"`      // Can be array or map
	Command       interface{}            `yaml:"command,omitempty"`     // Can be string or array
	Configs       []ComposeServiceConfig `yaml:"configs,omitempty"`
	CapAdd        []string               `yaml:"cap_add,omitempty"`
	Sysctls       interface{}            `yaml:"sysctls,omitempty"` // Can be array or map
	Secrets       []string               `yaml:"secrets,omitempty"`
	MemLimit      string                 `yaml:"mem_limit,omitempty"`
	MemswapLimit  int64                  `yaml:"memswap_limit,omitempty"`
	CPUs          interface{}            `yaml:"cpus,omitempty"` // Can be string or number
	Logging       *LoggingConfig         `yaml:"logging,omitempty"`
}

type LoggingConfig struct {
	Driver  string            `yaml:"driver"`
	Options map[string]string `yaml:"options,omitempty"`
}

type ComposeAction int

const (
	ComposeActionNone   ComposeAction = iota
	ComposeActionCreate ComposeAction = iota
	ComposeActionRemove ComposeAction = iota
	ComposeActionStart  ComposeAction = iota
	ComposeActionStop   ComposeAction = iota
	ComposeActionUp     ComposeAction = iota
	ComposeActionDown   ComposeAction = iota
)
