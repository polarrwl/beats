package filebeat_nacos

import (
	"fmt"
	"github.com/elastic/beats/v7/libbeat/logp"
	"github.com/nacos-group/nacos-sdk-go/clients"
	"github.com/nacos-group/nacos-sdk-go/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/common/constant"
	"github.com/nacos-group/nacos-sdk-go/vo"
	"github.com/pkg/errors"
	"net"
	syslog "log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var nacosModule *NacosModule

type NacosModule struct {
	nacosTool *NacosTool
	conf NacosConfig
	lb LoadBalancer
	log *logp.Logger
}


func(this *NacosModule) GetName() string {
	return "nacos"
}

func(this *NacosModule) InitModule(conf NacosConfig) {

	log := logp.NewLogger("nacos")
	this.log = log

	//初始化nacos
	if conf.Enable == true {
		log.Info("启动nacos")

		if conf.HttpStart == true {
			go func() {
				http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte("nacos纯监听端口 v1"))
				})
				log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", conf.HttpPort), nil))
			}()

		}

		time.Sleep(5 * time.Second)
		this.nacosTool = &NacosTool{}
		this.nacosTool.log = this.log
		this.lb = &RoundRobinLoadBalancer{}
		err := this.nacosTool.Init(conf)
		if err == nil {
			configStr := this.GetConfigStr(conf.ApplicationName + ".yml")
			log.Info("获取到配置，TODO：", configStr)
		} else {
			log.Error("nacos 初始化失败")
		}
	}

	nacosModule = this
}

func (this *NacosModule) GetConfigStr(name string) string {
	configStr, err := this.nacosTool.GetConfigClient().GetConfig(vo.ConfigParam{
		DataId: name,
		Group:  "DEFAULT_GROUP",
	})
	if err != nil {
		this.log.Error("获取配置失败:", err)
	} else {
		this.log.Info("获取到配置:", configStr)
	}
	return configStr
}

func (this *NacosModule) ListenerConfig(name string, callBack func(change string)) {
	//特别注意，这个监听必须要保证配置有的情况下调用，没有配置调用会导致大量循环请求
	this.nacosTool.cc.ListenConfig(vo.ConfigParam{
		// DataId: this.conf.Nacos.ApplicationName + ".yml",
		DataId: name,
		Group:  "DEFAULT_GROUP",
		OnChange: func(namespace, group, dataId, data string) {
			this.log.Info("监听到配置修改:", namespace, group, data, data)
			if callBack != nil {
				callBack(data)
			}
		},
	})
}

func (this *NacosModule) SelectInstance(serviceName string) *InstanceInfo {

	instanceInfos := this.GetInstances(serviceName)

	resInstance := GetRoundRobinLoadBalancer(serviceName).Choose(instanceInfos)
	this.log.Debug("轮训加权选取服务:", serviceName, " ", len(instanceInfos), " ", resInstance)
	return resInstance

}


func (this *NacosModule) GetAllService() []string {
	var param = vo.GetAllServiceInfoParam{
		PageNo: 1,
		PageSize: 1000,
	}
	resList, err := this.nacosTool.nc.GetAllServicesInfo(param)
	if err != nil {
		this.log.Error("获取所有服务失败:", err)
		return nil
	}
	return resList.Doms

}

func (this *NacosModule) GetInstances(serviceName string) []*InstanceInfo {
	ins, err := this.nacosTool.nc.SelectInstances(vo.SelectInstancesParam{
		ServiceName: serviceName,
		HealthyOnly: true,
	})
	if err != nil {
		this.log.Error("获取instance错误:", serviceName, " ", err)
		return nil
	}

	var instanceInfos []*InstanceInfo

	for _, v := range ins {
		instanceInfos = append(instanceInfos, &InstanceInfo{
			serviceName,
			fmt.Sprintf("%s:%d", v.Ip, v.Port),
			int(v.Weight * 100),
			v.Metadata,
		})
	}

	return instanceInfos

}


//
type NacosTool struct {
	cc config_client.IConfigClient
	nc naming_client.INamingClient
	log *logp.Logger
}

func (nt *NacosTool) Init(conf NacosConfig) error {

	nt.log.Info("初始化nacos client")

	// 可以没有，采用默认值
	clientConfig := constant.ClientConfig{
		TimeoutMs:      6 * 1000,
		ListenInterval: 30 * 1000,
		BeatInterval:   5 * 1000,
		NotLoadCacheAtStart: true,
		UpdateCacheWhenEmpty: true,
		//NamespaceId:    "public", //nacos命名空间
		LogDir:         "./logs_nacos",
		CacheDir:       "./cache_nacos",
		LogLevel: 		"info", //日志级别, debug、info、warn、error
	}

	// 至少一个
	serverConfigs := []constant.ServerConfig{
		{
			IpAddr:      conf.Server,
			ContextPath: "/nacos",
			Port:        uint64(conf.Port),
		},
	}

	namingClient, err := clients.CreateNamingClient(map[string]interface{}{
		"serverConfigs": serverConfigs,
		"clientConfig":  clientConfig,
	})
	if err != nil {
		nt.log.Error("nacos创建失败", err)
		return err
	}

	configClient, err := clients.CreateConfigClient(map[string]interface{}{
		"serverConfigs": serverConfigs,
		"clientConfig":  clientConfig,
	})

	if err != nil {
		nt.log.Error("nacos配置失败", err)
		return err
	}

	//nacos的日志输出有问题，这里需要强行扭转过来
	syslog.SetOutput(os.Stdout)
	syslog.SetFlags(syslog.Lshortfile | syslog.LstdFlags)

	clientIp, _ := GetLocalIp()
	if strings.TrimSpace(conf.Clientip) != "" {
		clientIp = conf.Clientip
	}
	nt.log.Info("nacos client创建成功，开始注册服务: clientip=", clientIp)

	//调用nacso注册实例（这里会自动启动心跳检查线程）
	success, err := namingClient.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          clientIp,
		Port:        uint64(conf.HttpPort),
		ServiceName: conf.ApplicationName,
		Weight:      1,
		//ClusterName: "a",
		Enable:    true,
		Healthy:   true,
		Ephemeral: true,
		Metadata: map[string]string{
			"content":                   "/",
			"preserved.register.source": "GO",
		},
	})

	nt.log.Info("nacos客戶端注冊結果:", success, err)

	nt.cc = configClient
	nt.nc = namingClient

	return err
}

func (this *NacosTool) GetConfigClient() config_client.IConfigClient {
	return this.cc
}



/**
获取本机IP地址
*/
func GetLocalIp() (string, error) {
	addrs, err := net.InterfaceAddrs()

	if err != nil {
		return "", err
	}

	for _, address := range addrs {
		// 检查ip地址判断是否回环地址
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil && isInnerIp(ipnet.IP.String()) && "127.0.0.1" != ipnet.IP.String() {
				return ipnet.IP.String(), nil
			}

		}
	}
	return "", errors.New("Can not find the client ip address!")

}


func isInnerIp(ipStr string) bool {
	if !checkIp(ipStr) {
		return false
	}
	inputIpNum := inetAton(ipStr)
	innerIpA := inetAton("10.255.255.255")
	innerIpB := inetAton("172.16.255.255")
	innerIpC := inetAton("192.168.255.255")
	innerIpD := inetAton("100.64.255.255")
	innerIpF := inetAton("127.255.255.255")

	return inputIpNum>>24 == innerIpA>>24 || inputIpNum>>20 == innerIpB>>20 ||
		inputIpNum>>16 == innerIpC>>16 || inputIpNum>>22 == innerIpD>>22 ||
		inputIpNum>>24 == innerIpF>>24
}

func checkIp(ipStr string) bool {
	address := net.ParseIP(ipStr)
	if address == nil {
		//fmt.Println("ip地址格式不正确")
		return false
	} else {
		//fmt.Println("正确的ip地址", address.String())
		return true
	}
}

// ip to int64
func inetAton(ipStr string) int64 {
	bits := strings.Split(ipStr, ".")

	b0, _ := strconv.Atoi(bits[0])
	b1, _ := strconv.Atoi(bits[1])
	b2, _ := strconv.Atoi(bits[2])
	b3, _ := strconv.Atoi(bits[3])

	var sum int64

	sum += int64(b0) << 24
	sum += int64(b1) << 16
	sum += int64(b2) << 8
	sum += int64(b3)

	return sum
}
