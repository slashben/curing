package config

type Config struct {
	AgentID            string        `json:"agent_id"`
	Server             ServerDetails `json:"server"`
	ConnectIntervalSec int           `json:"connect_interval_sec"`
	Groups             []string      `json:"groups"`
	UseTCPNetwork      bool          `json:"use_tcp_network"`
}

type ServerDetails struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}
