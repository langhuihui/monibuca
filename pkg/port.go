package pkg

import (
	"strconv"
	"strings"
)

type (
	TCPPort      int
	UDPPort      int
	TCPRangePort [2]int
	UDPRangePort [2]int
	Port         struct {
		Protocol string
		Ports    [2]int
		Map      [2]int // 映射端口范围，通常用于 NAT 或端口转发
	}
	IPort interface {
		IsTCP() bool
		IsUDP() bool
		IsRange() bool
	}
)

func (p Port) String() string {
	var result string
	if p.Ports[0] == p.Ports[1] {
		result = p.Protocol + ":" + strconv.Itoa(p.Ports[0])
	} else {
		result = p.Protocol + ":" + strconv.Itoa(p.Ports[0]) + "-" + strconv.Itoa(p.Ports[1])
	}

	// 如果有端口映射，添加映射信息
	if p.HasMapping() {
		if p.Map[0] == p.Map[1] {
			result += ":" + strconv.Itoa(p.Map[0])
		} else {
			result += ":" + strconv.Itoa(p.Map[0]) + "-" + strconv.Itoa(p.Map[1])
		}
	}

	return result
}

func (p Port) IsTCP() bool {
	return p.Protocol == "tcp"
}

func (p Port) IsUDP() bool {
	return p.Protocol == "udp"
}

func (p Port) IsRange() bool {
	return p.Ports[0] != p.Ports[1]
}

func (p Port) HasMapping() bool {
	return p.Map[0] > 0 || p.Map[1] > 0
}

func (p Port) IsRangeMapping() bool {
	return p.HasMapping() && p.Map[0] != p.Map[1]
}

// ParsePort2 解析端口配置字符串并返回对应的端口类型实例
// 根据协议类型和端口范围返回不同的类型：
// - TCP单端口：返回 TCPPort
// - TCP端口范围：返回 TCPRangePort
// - UDP单端口：返回 UDPPort
// - UDP端口范围：返回 UDPRangePort
//
// 参数：
//
//	conf - 端口配置字符串，格式：protocol:port 或 protocol:port1-port2
//
// 返回值：
//
//	ret - 端口实例 (TCPPort/UDPPort/TCPRangePort/UDPRangePort)
//	err - 解析错误
//
// 示例：
//
//	ParsePort2("tcp:8080")       // 返回 TCPPort(8080)
//	ParsePort2("tcp:8080-8090")  // 返回 TCPRangePort([2]int{8080, 8090})
//	ParsePort2("udp:5000")       // 返回 UDPPort(5000)
//	ParsePort2("udp:5000-5010")  // 返回 UDPRangePort([2]int{5000, 5010})
func ParsePort2(conf string) (ret any, err error) {
	var port Port
	port, err = ParsePort(conf)
	if err != nil {
		return
	}
	if port.IsTCP() {
		if port.IsRange() {
			return TCPRangePort(port.Ports), nil
		}
		return TCPPort(port.Ports[0]), nil
	}
	if port.IsRange() {
		return UDPRangePort(port.Ports), nil
	}
	return UDPPort(port.Ports[0]), nil
}

// ParsePort 解析端口配置字符串为 Port 结构体
// 支持协议前缀、端口号/端口范围以及端口映射的解析
//
// 参数：
//
//	conf - 端口配置字符串，格式：
//	       - "protocol:port"           单端口，如 "tcp:8080"
//	       - "protocol:port1-port2"    端口范围，如 "tcp:8080-8090"
//	       - "protocol:port:mapPort"   单端口映射，如 "tcp:8080:9090"
//	       - "protocol:port:mapPort1-mapPort2" 单端口映射到端口范围，如 "tcp:8080:9000-9010"
//	       - "protocol:port1-port2:mapPort1-mapPort2" 端口范围映射，如 "tcp:8080-8090:9000-9010"
//
// 返回值：
//
//	ret - Port 结构体，包含协议、端口和映射端口信息
//	err - 解析错误
//
// 注意：
//   - 如果端口范围中 min > max，会自动交换顺序
//   - 单端口时，Ports[0] 和 Ports[1] 值相同
//   - 端口映射时，Map[0] 和 Map[1] 存储映射的目标端口范围
//   - 单个映射端口时，Map[0] 和 Map[1] 值相同
//
// 示例：
//
//	ParsePort("tcp:8080")              // Port{Protocol:"tcp", Ports:[2]int{8080, 8080}, Map:[2]int{0, 0}}
//	ParsePort("tcp:8080-8090")         // Port{Protocol:"tcp", Ports:[2]int{8080, 8090}, Map:[2]int{0, 0}}
//	ParsePort("tcp:8080:9090")         // Port{Protocol:"tcp", Ports:[2]int{8080, 8080}, Map:[2]int{9090, 9090}}
//	ParsePort("tcp:8080:9000-9010")    // Port{Protocol:"tcp", Ports:[2]int{8080, 8080}, Map:[2]int{9000, 9010}}
//	ParsePort("tcp:8080-8090:9000-9010") // Port{Protocol:"tcp", Ports:[2]int{8080, 8090}, Map:[2]int{9000, 9010}}
//	ParsePort("udp:5000")              // Port{Protocol:"udp", Ports:[2]int{5000, 5000}, Map:[2]int{0, 0}}
//	ParsePort("udp:5010-5000")         // Port{Protocol:"udp", Ports:[2]int{5000, 5010}, Map:[2]int{0, 0}}
func ParsePort(conf string) (ret Port, err error) {
	var port, mapPort string
	var min, max int

	// 按冒号分割，支持端口映射
	parts := strings.Split(conf, ":")
	if len(parts) < 2 || len(parts) > 3 {
		err = strconv.ErrSyntax
		return
	}

	ret.Protocol = parts[0]
	port = parts[1]

	// 处理端口映射
	if len(parts) == 3 {
		mapPort = parts[2]
		// 解析映射端口，支持单端口和端口范围
		if mapRange := strings.Split(mapPort, "-"); len(mapRange) == 2 {
			// 映射端口范围
			var mapMin, mapMax int
			mapMin, err = strconv.Atoi(mapRange[0])
			if err != nil {
				return
			}
			mapMax, err = strconv.Atoi(mapRange[1])
			if err != nil {
				return
			}
			if mapMin < mapMax {
				ret.Map[0], ret.Map[1] = mapMin, mapMax
			} else {
				ret.Map[0], ret.Map[1] = mapMax, mapMin
			}
		} else {
			// 单个映射端口
			var mapPortNum int
			mapPortNum, err = strconv.Atoi(mapPort)
			if err != nil {
				return
			}
			ret.Map[0], ret.Map[1] = mapPortNum, mapPortNum
		}
	}

	// 处理端口范围
	if r := strings.Split(port, "-"); len(r) == 2 {
		min, err = strconv.Atoi(r[0])
		if err != nil {
			return
		}
		max, err = strconv.Atoi(r[1])
		if err != nil {
			return
		}
		if min < max {
			ret.Ports[0], ret.Ports[1] = min, max
		} else {
			ret.Ports[0], ret.Ports[1] = max, min
		}
	} else {
		var p int
		p, err = strconv.Atoi(port)
		if err != nil {
			return
		}
		ret.Ports[0], ret.Ports[1] = p, p
	}
	return
}
