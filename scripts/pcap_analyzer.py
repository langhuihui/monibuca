#!/usr/bin/env python3
import argparse
from scapy.all import rdpcap, IP, TCP, UDP, Raw, PcapReader
import sys
from collections import defaultdict

def analyze_pcap(pcap_file, target_port=1935):
    """分析pcap文件的详细信息"""
    print(f"分析PCAP文件: {pcap_file}")
    print(f"目标端口: {target_port}")
    print("=" * 60)

    try:
        reader = PcapReader(pcap_file)

        # 统计信息
        total_packets = 0
        tcp_packets = 0
        udp_packets = 0
        other_packets = 0

        # 连接统计
        connections = defaultdict(lambda: {'packets': 0, 'bytes': 0, 'to_target': 0, 'from_target': 0})
        target_connections = defaultdict(lambda: {'packets': 0, 'bytes': 0})

        # 数据大小统计
        total_tcp_data = 0
        target_tcp_data = 0

        # IP统计
        ip_stats = defaultdict(lambda: {'send_bytes': 0, 'recv_bytes': 0, 'packets': 0})

        for packet in reader:
            total_packets += 1

            if IP in packet:
                src_ip = packet[IP].src
                dst_ip = packet[IP].dst

                if TCP in packet:
                    tcp_packets += 1
                    src_port = packet[TCP].sport
                    dst_port = packet[TCP].dport

                    conn_key = f"{src_ip}:{src_port} -> {dst_ip}:{dst_port}"
                    connections[conn_key]['packets'] += 1

                    if Raw in packet:
                        payload_size = len(packet[Raw].load)
                        connections[conn_key]['bytes'] += payload_size
                        total_tcp_data += payload_size

                        # 统计IP流量
                        ip_stats[src_ip]['send_bytes'] += payload_size
                        ip_stats[dst_ip]['recv_bytes'] += payload_size
                        ip_stats[src_ip]['packets'] += 1

                        # 检查是否与目标端口相关
                        if dst_port == target_port:
                            connections[conn_key]['to_target'] += payload_size
                            target_tcp_data += payload_size
                            target_key = f"{src_ip}:{src_port}"
                            target_connections[target_key]['packets'] += 1
                            target_connections[target_key]['bytes'] += payload_size
                        elif src_port == target_port:
                            connections[conn_key]['from_target'] += payload_size

                elif UDP in packet:
                    udp_packets += 1
                else:
                    other_packets += 1
            else:
                other_packets += 1

        reader.close()

        # 输出统计结果
        print(f"总包数: {total_packets}")
        print(f"TCP包数: {tcp_packets}")
        print(f"UDP包数: {udp_packets}")
        print(f"其他包数: {other_packets}")
        print(f"TCP总数据量: {total_tcp_data / (1024*1024):.2f} MB")
        print(f"目标端口({target_port})数据量: {target_tcp_data / (1024*1024):.2f} MB")
        print()

        # 显示IP统计
        print("IP地址统计:")
        print("-" * 60)
        for ip, stats in sorted(ip_stats.items(), key=lambda x: x[1]['send_bytes'], reverse=True)[:10]:
            send_mb = stats['send_bytes'] / (1024*1024)
            recv_mb = stats['recv_bytes'] / (1024*1024)
            print(f"{ip:15} 发送: {send_mb:8.2f}MB 接收: {recv_mb:8.2f}MB 包数: {stats['packets']}")
        print()

        # 显示目标端口连接统计
        print(f"目标端口({target_port})连接统计:")
        print("-" * 60)
        for conn, stats in sorted(target_connections.items(), key=lambda x: x[1]['bytes'], reverse=True):
            mb = stats['bytes'] / (1024*1024)
            print(f"{conn:25} 包数: {stats['packets']:6} 数据量: {mb:8.2f}MB")
        print()

        # 显示前10个最大的连接
        print("前10个最大的连接:")
        print("-" * 80)
        for conn, stats in sorted(connections.items(), key=lambda x: x[1]['bytes'], reverse=True)[:10]:
            mb = stats['bytes'] / (1024*1024)
            to_target_mb = stats['to_target'] / (1024*1024)
            from_target_mb = stats['from_target'] / (1024*1024)
            print(f"{conn:40} 总量: {mb:8.2f}MB 发往目标: {to_target_mb:6.2f}MB 来自目标: {from_target_mb:6.2f}MB")

    except Exception as e:
        print(f"分析PCAP文件时出错: {e}")
        import traceback
        traceback.print_exc()

def main():
    parser = argparse.ArgumentParser(description='PCAP文件分析工具')
    parser.add_argument('pcap_file', help='pcap文件路径')
    parser.add_argument('--target-port', type=int, default=1935, help='目标端口 (默认: 1935)')

    args = parser.parse_args()
    analyze_pcap(args.pcap_file, args.target_port)

if __name__ == '__main__':
    main()
