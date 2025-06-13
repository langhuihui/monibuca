package pkg

import (
	"testing"
)

func TestParsePort(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Port
		hasError bool
	}{
		{
			name:  "TCP单端口",
			input: "tcp:8080",
			expected: Port{
				Protocol: "tcp",
				Ports:    [2]int{8080, 8080},
				Map:      [2]int{0, 0},
			},
			hasError: false,
		},
		{
			name:  "TCP端口范围",
			input: "tcp:8080-8090",
			expected: Port{
				Protocol: "tcp",
				Ports:    [2]int{8080, 8090},
				Map:      [2]int{0, 0},
			},
			hasError: false,
		},
		{
			name:  "TCP端口范围（反序）",
			input: "tcp:8090-8080",
			expected: Port{
				Protocol: "tcp",
				Ports:    [2]int{8080, 8090},
				Map:      [2]int{0, 0},
			},
			hasError: false,
		},
		{
			name:  "TCP单端口映射到单端口",
			input: "tcp:8080:9090",
			expected: Port{
				Protocol: "tcp",
				Ports:    [2]int{8080, 8080},
				Map:      [2]int{9090, 9090},
			},
			hasError: false,
		},
		{
			name:  "TCP单端口映射到端口范围",
			input: "tcp:8080:9000-9010",
			expected: Port{
				Protocol: "tcp",
				Ports:    [2]int{8080, 8080},
				Map:      [2]int{9000, 9010},
			},
			hasError: false,
		},
		{
			name:  "TCP端口范围映射到端口范围",
			input: "tcp:8080-8090:9000-9010",
			expected: Port{
				Protocol: "tcp",
				Ports:    [2]int{8080, 8090},
				Map:      [2]int{9000, 9010},
			},
			hasError: false,
		},
		{
			name:  "UDP单端口",
			input: "udp:5000",
			expected: Port{
				Protocol: "udp",
				Ports:    [2]int{5000, 5000},
				Map:      [2]int{0, 0},
			},
			hasError: false,
		},
		{
			name:  "UDP端口范围",
			input: "udp:5000-5010",
			expected: Port{
				Protocol: "udp",
				Ports:    [2]int{5000, 5010},
				Map:      [2]int{0, 0},
			},
			hasError: false,
		},
		{
			name:  "UDP端口映射",
			input: "udp:5000:6000",
			expected: Port{
				Protocol: "udp",
				Ports:    [2]int{5000, 5000},
				Map:      [2]int{6000, 6000},
			},
			hasError: false,
		},
		{
			name:  "UDP端口范围映射（映射范围反序）",
			input: "udp:5000-5010:6010-6000",
			expected: Port{
				Protocol: "udp",
				Ports:    [2]int{5000, 5010},
				Map:      [2]int{6000, 6010},
			},
			hasError: false,
		},
		// 错误情况
		{
			name:     "缺少协议",
			input:    "8080",
			expected: Port{},
			hasError: true,
		},
		{
			name:     "过多冒号",
			input:    "tcp:8080:9090:extra",
			expected: Port{},
			hasError: true,
		},
		{
			name:     "无效端口号",
			input:    "tcp:abc",
			expected: Port{},
			hasError: true,
		},
		{
			name:     "无效映射端口号",
			input:    "tcp:8080:abc",
			expected: Port{},
			hasError: true,
		},
		{
			name:     "无效端口范围",
			input:    "tcp:8080-abc",
			expected: Port{},
			hasError: true,
		},
		{
			name:     "无效映射端口范围",
			input:    "tcp:8080:9000-abc",
			expected: Port{},
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePort(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("期望有错误，但没有错误")
				}
				return
			}

			if err != nil {
				t.Errorf("意外的错误: %v", err)
				return
			}

			if result.Protocol != tt.expected.Protocol {
				t.Errorf("协议不匹配: 期望 %s, 得到 %s", tt.expected.Protocol, result.Protocol)
			}

			if result.Ports != tt.expected.Ports {
				t.Errorf("端口不匹配: 期望 %v, 得到 %v", tt.expected.Ports, result.Ports)
			}

			if result.Map != tt.expected.Map {
				t.Errorf("映射端口不匹配: 期望 %v, 得到 %v", tt.expected.Map, result.Map)
			}
		})
	}
}

func TestPortMethods(t *testing.T) {
	tests := []struct {
		name           string
		port           Port
		expectTCP      bool
		expectUDP      bool
		expectRange    bool
		expectMapping  bool
		expectRangeMap bool
		expectString   string
	}{
		{
			name: "TCP单端口",
			port: Port{
				Protocol: "tcp",
				Ports:    [2]int{8080, 8080},
				Map:      [2]int{0, 0},
			},
			expectTCP:      true,
			expectUDP:      false,
			expectRange:    false,
			expectMapping:  false,
			expectRangeMap: false,
			expectString:   "tcp:8080",
		},
		{
			name: "TCP端口范围",
			port: Port{
				Protocol: "tcp",
				Ports:    [2]int{8080, 8090},
				Map:      [2]int{0, 0},
			},
			expectTCP:      true,
			expectUDP:      false,
			expectRange:    true,
			expectMapping:  false,
			expectRangeMap: false,
			expectString:   "tcp:8080-8090",
		},
		{
			name: "TCP单端口映射",
			port: Port{
				Protocol: "tcp",
				Ports:    [2]int{8080, 8080},
				Map:      [2]int{9090, 9090},
			},
			expectTCP:      true,
			expectUDP:      false,
			expectRange:    false,
			expectMapping:  true,
			expectRangeMap: false,
			expectString:   "tcp:8080:9090",
		},
		{
			name: "TCP端口范围映射",
			port: Port{
				Protocol: "tcp",
				Ports:    [2]int{8080, 8090},
				Map:      [2]int{9000, 9010},
			},
			expectTCP:      true,
			expectUDP:      false,
			expectRange:    true,
			expectMapping:  true,
			expectRangeMap: true,
			expectString:   "tcp:8080-8090:9000-9010",
		},
		{
			name: "UDP单端口映射到端口范围",
			port: Port{
				Protocol: "udp",
				Ports:    [2]int{5000, 5000},
				Map:      [2]int{6000, 6010},
			},
			expectTCP:      false,
			expectUDP:      true,
			expectRange:    false,
			expectMapping:  true,
			expectRangeMap: true,
			expectString:   "udp:5000:6000-6010",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.port.IsTCP() != tt.expectTCP {
				t.Errorf("IsTCP(): 期望 %v, 得到 %v", tt.expectTCP, tt.port.IsTCP())
			}

			if tt.port.IsUDP() != tt.expectUDP {
				t.Errorf("IsUDP(): 期望 %v, 得到 %v", tt.expectUDP, tt.port.IsUDP())
			}

			if tt.port.IsRange() != tt.expectRange {
				t.Errorf("IsRange(): 期望 %v, 得到 %v", tt.expectRange, tt.port.IsRange())
			}

			if tt.port.HasMapping() != tt.expectMapping {
				t.Errorf("HasMapping(): 期望 %v, 得到 %v", tt.expectMapping, tt.port.HasMapping())
			}

			if tt.port.IsRangeMapping() != tt.expectRangeMap {
				t.Errorf("IsRangeMapping(): 期望 %v, 得到 %v", tt.expectRangeMap, tt.port.IsRangeMapping())
			}

			if tt.port.String() != tt.expectString {
				t.Errorf("String(): 期望 %s, 得到 %s", tt.expectString, tt.port.String())
			}
		})
	}
}

func TestParsePort2(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedType string
		hasError     bool
	}{
		{
			name:         "TCP单端口",
			input:        "tcp:8080",
			expectedType: "TCPPort",
			hasError:     false,
		},
		{
			name:         "TCP端口范围",
			input:        "tcp:8080-8090",
			expectedType: "TCPRangePort",
			hasError:     false,
		},
		{
			name:         "UDP单端口",
			input:        "udp:5000",
			expectedType: "UDPPort",
			hasError:     false,
		},
		{
			name:         "UDP端口范围",
			input:        "udp:5000-5010",
			expectedType: "UDPRangePort",
			hasError:     false,
		},
		{
			name:     "无效输入",
			input:    "invalid",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePort2(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("期望有错误，但没有错误")
				}
				return
			}

			if err != nil {
				t.Errorf("意外的错误: %v", err)
				return
			}

			switch tt.expectedType {
			case "TCPPort":
				if _, ok := result.(TCPPort); !ok {
					t.Errorf("期望类型 TCPPort, 得到 %T", result)
				}
			case "TCPRangePort":
				if _, ok := result.(TCPRangePort); !ok {
					t.Errorf("期望类型 TCPRangePort, 得到 %T", result)
				}
			case "UDPPort":
				if _, ok := result.(UDPPort); !ok {
					t.Errorf("期望类型 UDPPort, 得到 %T", result)
				}
			case "UDPRangePort":
				if _, ok := result.(UDPRangePort); !ok {
					t.Errorf("期望类型 UDPRangePort, 得到 %T", result)
				}
			}
		})
	}
}
