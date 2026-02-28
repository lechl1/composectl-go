package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// DockerInspect represents the complete Docker container inspect output
type DockerInspect struct {
	ID              string          `json:"id"`
	Created         string          `json:"created"`
	Path            string          `json:"path"`
	Args            []string        `json:"args"`
	State           ContainerState  `json:"state"`
	Image           string          `json:"image"`
	ResolvConfPath  string          `json:"resolvconfpath"`
	HostnamePath    string          `json:"hostnamepath"`
	HostsPath       string          `json:"hostspath"`
	LogPath         string          `json:"logpath"`
	Name            string          `json:"name"`
	RestartCount    int             `json:"restartcount"`
	Driver          string          `json:"driver"`
	Platform        string          `json:"platform"`
	MountLabel      string          `json:"mountlabel"`
	ProcessLabel    string          `json:"processlabel"`
	AppArmorProfile string          `json:"apparmorprofile"`
	ExecIDs         []string        `json:"execids"`
	HostConfig      HostConfig      `json:"hostconfig"`
	GraphDriver     GraphDriver     `json:"graphdriver"`
	Mounts          []Mount         `json:"mounts"`
	Config          ContainerConfig `json:"config"`
	NetworkSettings NetworkSettings `json:"networksettings"`
}

// ContainerState represents the state of a container
type ContainerState struct {
	Status     string `json:"status"`
	Running    bool   `json:"running"`
	Paused     bool   `json:"paused"`
	Restarting bool   `json:"restarting"`
	OOMKilled  bool   `json:"oomkilled"`
	Dead       bool   `json:"dead"`
	Pid        int    `json:"pid"`
	ExitCode   int    `json:"exitcode"`
	Error      string `json:"error"`
	StartedAt  string `json:"startedat"`
	FinishedAt string `json:"finishedat"`
}

// HostConfig represents the host configuration for a container
type HostConfig struct {
	Binds                []string                 `json:"binds"`
	ContainerIDFile      string                   `json:"containeridfile"`
	LogConfig            LogConfig                `json:"logconfig"`
	NetworkMode          string                   `json:"networkmode"`
	PortBindings         map[string][]PortBinding `json:"portbindings"`
	RestartPolicy        RestartPolicy            `json:"restartpolicy"`
	AutoRemove           bool                     `json:"autoremove"`
	VolumeDriver         string                   `json:"volumedriver"`
	VolumesFrom          []string                 `json:"volumesfrom"`
	CapabilityAdd        []string                 `json:"capabilityadd"`
	CapabilityDrop       []string                 `json:"capabilitydrop"`
	DNS                  []string                 `json:"dns"`
	DNSOptions           []string                 `json:"dnsoptions"`
	DNSSearch            []string                 `json:"dnssearch"`
	ExtraHosts           []string                 `json:"extrahosts"`
	GroupAdd             []string                 `json:"groupadd"`
	IpcMode              string                   `json:"ipcmode"`
	Cgroup               string                   `json:"cgroup"`
	Links                []string                 `json:"links"`
	OomScoreAdj          int                      `json:"oomscoreadj"`
	PidMode              string                   `json:"pidmode"`
	Privileged           bool                     `json:"privileged"`
	PublishAllPorts      bool                     `json:"publishallports"`
	ReadonlyRootfs       bool                     `json:"readonlyrootfs"`
	SecurityOpt          []string                 `json:"securityopt"`
	UTSMode              string                   `json:"utsmode"`
	UsernsMode           string                   `json:"usernsmode"`
	ShmSize              int64                    `json:"shmsize"`
	Runtime              string                   `json:"runtime"`
	ConsoleSize          []int                    `json:"consolesize"`
	Isolation            string                   `json:"isolation"`
	CPUShares            int64                    `json:"cpushares"`
	Memory               int64                    `json:"memory"`
	NanoCPUs             int64                    `json:"nanomemory"`
	CgroupParent         string                   `json:"cgroupparent"`
	BlkioWeight          uint16                   `json:"blkioweight"`
	BlkioWeightDevice    []WeightDevice           `json:"blkioweightdevice"`
	BlkioDeviceReadBps   []ThrottleDevice         `json:"blkiodevicereadbps"`
	BlkioDeviceWriteBps  []ThrottleDevice         `json:"blkiodevicewritebps"`
	BlkioDeviceReadIOps  []ThrottleDevice         `json:"blkiodevicereadiops"`
	BlkioDeviceWriteIOps []ThrottleDevice         `json:"blkiodevicewriteiops"`
	CPUPeriod            int64                    `json:"cpuperiod"`
	CPUQuota             int64                    `json:"cpuquota"`
	CPURealtimePeriod    int64                    `json:"cpurealtimeperiod"`
	CPURealtimeRuntime   int64                    `json:"cpurealtimeruntime"`
	CpusetCpus           string                   `json:"cpusetcpus"`
	CpusetMems           string                   `json:"cpusetmems"`
	Devices              []Device                 `json:"devices"`
	DeviceCgroupRules    []string                 `json:"devicecgrouprules"`
	DiskQuota            int64                    `json:"diskquota"`
	KernelMemory         int64                    `json:"kernelmemory"`
	MemoryReservation    int64                    `json:"memoryreservation"`
	MemorySwap           int64                    `json:"memoryswap"`
	MemorySwappiness     *int64                   `json:"memoryswappiness"`
	OomKillDisable       *bool                    `json:"oomkilldisable"`
	PidsLimit            *int64                   `json:"pidslimit"`
	Ulimits              []Ulimit                 `json:"ulimits"`
	CPUCount             int64                    `json:"cpucount"`
	CPUPercent           int64                    `json:"cpupercent"`
	IOMaximumIOps        int64                    `json:"iomaximumiops"`
	IOMaximumBandwidth   int64                    `json:"iomaximumbandwidth"`
}

// LogConfig represents logging configuration
type LogConfig struct {
	Type   string            `json:"type"`
	Config map[string]string `json:"config"`
}

// PortBinding represents a port binding
type PortBinding struct {
	HostIP   string `json:"hostip"`
	HostPort string `json:"hostport"`
}

// RestartPolicy represents the restart policy for a container
type RestartPolicy struct {
	Name              string `json:"name"`
	MaximumRetryCount int    `json:"maximumretrycount"`
}

// WeightDevice represents a weight device
type WeightDevice struct {
	Path   string `json:"path"`
	Weight uint16 `json:"weight"`
}

// ThrottleDevice represents a throttle device
type ThrottleDevice struct {
	Path string `json:"path"`
	Rate uint64 `json:"rate"`
}

// Device represents a device mapping
type Device struct {
	PathOnHost        string `json:"pathonhost"`
	PathInContainer   string `json:"pathincontainer"`
	CgroupPermissions string `json:"cgrouppermissions"`
}

// Ulimit represents a ulimit setting
type Ulimit struct {
	Name string `json:"name"`
	Soft int64  `json:"soft"`
	Hard int64  `json:"hard"`
}

// GraphDriver represents the graph driver information
type GraphDriver struct {
	Name string            `json:"name"`
	Data map[string]string `json:"data"`
}

// Mount represents a mount point
type Mount struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
	RW          bool   `json:"rw"`
	Propagation string `json:"propagation"`
	Name        string `json:"name,omitempty"`
	Driver      string `json:"driver,omitempty"`
}

// ContainerConfig represents the container configuration
type ContainerConfig struct {
	Hostname     string                 `json:"hostname"`
	Domainname   string                 `json:"domainname"`
	User         string                 `json:"user"`
	AttachStdin  bool                   `json:"attachstdin"`
	AttachStdout bool                   `json:"attachstdout"`
	AttachStderr bool                   `json:"attachstderr"`
	ExposedPorts map[string]interface{} `json:"exposedports"`
	Tty          bool                   `json:"tty"`
	OpenStdin    bool                   `json:"openstdin"`
	StdinOnce    bool                   `json:"stdinonce"`
	Env          []string               `json:"env"`
	Cmd          []string               `json:"cmd"`
	Image        string                 `json:"image"`
	Volumes      map[string]interface{} `json:"volumes"`
	WorkingDir   string                 `json:"workingdir"`
	Entrypoint   []string               `json:"entrypoint"`
	OnBuild      []string               `json:"onbuild"`
	Labels       map[string]string      `json:"labels"`
}

// NetworkSettings represents network settings for a container
type NetworkSettings struct {
	Bridge                 string                      `json:"bridge"`
	SandboxID              string                      `json:"sandboxid"`
	HairpinMode            bool                        `json:"hairpinmode"`
	LinkLocalIPv6Address   string                      `json:"linklocalipv6address"`
	LinkLocalIPv6PrefixLen int                         `json:"linklocalipv6prefixlen"`
	Ports                  map[string][]PortBinding    `json:"ports"`
	SandboxKey             string                      `json:"sandboxkey"`
	SecondaryIPAddresses   []string                    `json:"secondaryipaddresses"`
	SecondaryIPv6Addresses []string                    `json:"secondaryipv6addresses"`
	EndpointID             string                      `json:"endpointid"`
	Gateway                string                      `json:"gateway"`
	GlobalIPv6Address      string                      `json:"globalipv6address"`
	GlobalIPv6PrefixLen    int                         `json:"globalipv6prefixlen"`
	IPAddress              string                      `json:"ipaddress"`
	IPPrefixLen            int                         `json:"ipprefixlen"`
	IPv6Gateway            string                      `json:"ipv6gateway"`
	MacAddress             string                      `json:"macaddress"`
	Networks               map[string]EndpointSettings `json:"networks"`
}

// EndpointSettings represents network endpoint settings
type EndpointSettings struct {
	IPAMConfig          *EndpointIPAMConfig `json:"ipamconfig"`
	Links               []string            `json:"links"`
	Aliases             []string            `json:"aliases"`
	NetworkID           string              `json:"networkid"`
	EndpointID          string              `json:"endpointid"`
	Gateway             string              `json:"gateway"`
	IPAddress           string              `json:"ipaddress"`
	IPPrefixLen         int                 `json:"ipprefixlen"`
	IPv6Gateway         string              `json:"ipv6gateway"`
	GlobalIPv6Address   string              `json:"globalipv6address"`
	GlobalIPv6PrefixLen int                 `json:"globalipv6prefixlen"`
	MacAddress          string              `json:"macaddress"`
}

// EndpointIPAMConfig represents IPAM configuration for an endpoint
type EndpointIPAMConfig struct {
	IPv4Address string `json:"ipv4address"`
	IPv6Address string `json:"ipv6address"`
}

// getAllContainers executes docker inspect and returns all containers (running and stopped)
func getAllContainers() ([]map[string]interface{}, error) {
	// Get all container IDs using docker ps -a -q
	cmd := exec.Command("docker", "ps", "-a", "-q", "--no-trunc")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute docker ps: %w", err)
	}

	// Parse container IDs from output
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var containerIDs []string
	for _, line := range lines {
		if line != "" {
			containerIDs = append(containerIDs, line)
		}
	}

	// If no containers found, return empty list
	if len(containerIDs) == 0 {
		return []map[string]interface{}{}, nil
	}

	// Use existing inspectContainers function to get full details
	inspectData, err := inspectContainers(containerIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect containers: %w", err)
	}

	// Convert []DockerInspect to []map[string]interface{} for compatibility
	var containers []map[string]interface{}
	for _, inspect := range inspectData {
		// Marshal to JSON and back to get map representation
		jsonData, err := json.Marshal(inspect)
		if err != nil {
			log.Printf("Error marshaling inspect data: %v", err)
			continue
		}

		var container map[string]interface{}
		if err := json.Unmarshal(jsonData, &container); err != nil {
			log.Printf("Error unmarshaling to map: %v", err)
			continue
		}

		containers = append(containers, container)
	}

	return containers, nil
}
