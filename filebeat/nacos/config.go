package filebeat_nacos

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)


type NacosConfig struct {
	ApplicationName string `yaml:"applicationName"`
	Server          string `yaml:"server"`
	Port            int    `yaml:"port"`
	Enable          bool   `yaml:"enable"`
	Clientip        string `yaml:"clientip"` //如果为空就用本地ip
	HttpStart		bool `yaml:"httpstart"` //是否启动http服务
	HttpPort        int64 `yaml:"httpport"` //HTTP监听端口
}

var currentPath string = ""
var conf NacosConfig
var env string

/**
启动添加参数：-env dev/sit/pro
*/
func InitConfig(flagNacosConfig string) NacosConfig {
	argsConfigFileName := flagNacosConfig
	fmt.Println("配置文件:", argsConfigFileName)
	configFileName := argsConfigFileName

	yamlFile, err := ioutil.ReadFile(configFileName)
	if err != nil {
		fmt.Println("yamlFile.Get err", err)
	}
	err = yaml.Unmarshal(yamlFile, &conf)
	if err != nil {
		fmt.Println("Unmarshal: %v", err)
	}

	fmt.Println("初始化配置成功:", conf)
	return conf
}
