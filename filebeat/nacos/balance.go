package filebeat_nacos

import (
	"fmt"
	"sync"
)



// 封装服务实例信息
type InstanceInfo struct {
	ServiceName		string
	// 格式为 host:port
	Addr 			string
	//权重
	Weight int
	// 此实例附加信息
	Meta 			map[string]string
}


// 负载均衡接口
type LoadBalancer interface {
	// 从instance中选一个对象返回
	Choose(instances []*InstanceInfo) *InstanceInfo
}

// 轮询均衡器实现
type RoundRobinLoadBalancer struct {
	server string
	i int //表示上一次选择的服务器
	cw int //表示当前调度的权值
	gcd int //当前所有权重的最大公约数 比如 2，4，8 的最大公约数为：2
}

var mapRoundRobinLoadBalancer sync.Map

func GetRoundRobinLoadBalancer(_server string) LoadBalancer {
	res, load := mapRoundRobinLoadBalancer.LoadOrStore(_server, NewRoundRobinLoadBalancer(_server))
	if load == false {
		fmt.Println("缓存中添加新RoundRobinLoadBalancer实例:{}", _server)
	}
	return res.(LoadBalancer)
}

func NewRoundRobinLoadBalancer(_server string) LoadBalancer {
	return &RoundRobinLoadBalancer{
		i: -1,
		cw: 0,
		gcd: 0,
		server: _server,
	}
}


func (lb *RoundRobinLoadBalancer) Choose(instances []*InstanceInfo) *InstanceInfo {

	if len(instances) == 0 {
		return nil
	}

	if len(instances) == 1 {
		return instances[0]
	}

	lb.gcd = lb.getGCD(instances)
	for {
		lb.i = (lb.i + 1) % len(instances)
		if lb.i == 0 {
			lb.cw = lb.cw - lb.gcd
			if lb.cw <= 0 {
				lb.cw = lb.getMaxWeight(instances)
				if lb.cw == 0 {
					return nil
				}
			}
		}

		if instances[lb.i].Weight >= lb.cw {
			return instances[lb.i]
		}
	}

}

//根据instance列表计算获取到最大公约数
func (lb *RoundRobinLoadBalancer) getGCD(instances []*InstanceInfo) int {

	w := 0
	len := len(instances)

	for i := 0; i < len - 1; i++ {
		if w == 0 {
			w = gcd(instances[i].Weight, instances[i+1].Weight);
		} else {
			w = gcd(w, instances[i+1].Weight);
		}
	}
	return w
}

//计算最大公约数
func gcd(x, y int) int {
	var t int
	for {
		t = (x % y)
		if t > 0 {
			x = y
			y = t
		} else {
			return y
		}
	}
}


func (lb *RoundRobinLoadBalancer) getMaxWeight(instances []*InstanceInfo) int {
	max := 0
	for _, v := range instances {
		if v.Weight >= max {
			max = v.Weight
		}
	}

	return max
}

