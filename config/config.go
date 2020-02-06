package config

import (
	"io/ioutil"

	yaml "gopkg.in/yaml.v2"
)

// Config structure for server
type Config struct {
	App appStruct `yaml:"app"`
	TLS tlsStruct `yaml:"tls"`
	Log logStruct `yaml:"log"`
}

type appStruct struct {
	Heartbeat   int    `yaml:"heartbeat_interval"`
	Collect     int    `yaml:"collect_interval"`
	Gateway     string `yaml:"gateway_addr"`
	Server      string `yaml:"listen_addr"`
	BaseDir     string `yaml:"base_dir"`
	Application string `yaml:"application"`
}

type tlsStruct struct {
	Switch bool   `yaml:"switch"`
	Perm   string `yaml:"perm"`
	Key    string `yaml:"key"`
	Ca     string `yaml:"ca"`
}

// logStruct defines fields for log
type logStruct struct {
	OutputLevel        string `yaml:"output_level"`
	OutputPath         string `yaml:"output_path"`
	RotationPath       string `yaml:"rotation_path"`
	RotationMaxSize    int    `yaml:"rotation_max_size"`
	RotationMaxAge     int    `yaml:"rotation_max_age"`
	RotationMaxBackups int    `yaml:"rotation_max_backups"`
	JSONEncoding       bool   `yaml:"json_encoding"`
}

// ParseYamlFile the config file
func ParseYamlFile(filename string, c *Config) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, c)
}
