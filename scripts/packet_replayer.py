#!/usr/bin/env python3
import argparse
from scapy.all import rdpcap, IP, TCP, UDP, Raw, send, sr1, sr, PcapReader
import sys
import time
from collections import defaultdict
import random
import threading
import queue
import socket

class PacketReplayer:
    def __init__(self, pcap_file, target_ip, target_port):
        self.pcap_file = pcap_file
        self.target_ip = target_ip
        self.target_port = target_port
        self.connections = defaultdict(list)  # 存储每个连接的包序列
        self.response_queue = queue.Queue()
        self.stop_reading = threading.Event()
        self.socket = None

    def establish_tcp_connection(self, src_port):
        """建立TCP连接"""
        print(f"正在建立TCP连接 {self.target_ip}:{self.target_port}...")
        try:
            # 创建socket对象
            self.socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            # 绑定源端口（如果指定了端口）
            if src_port > 0:
                try:
                    self.socket.bind(('0.0.0.0', src_port))
                except socket.error as e:
                    print(f"指定端口 {src_port} 被占用，将使用随机端口")
                    self.socket.bind(('0.0.0.0', 0))  # 使用随机可用端口
            else:
                self.socket.bind(('0.0.0.0', 0))  # 使用随机可用端口
            
            # 获取实际使用的端口
            actual_port = self.socket.getsockname()[1]
            print(f"使用本地端口: {actual_port}")
            
            # 设置超时
            self.socket.settimeout(5)
            # 连接目标
            self.socket.connect((self.target_ip, self.target_port))
            print("TCP连接已建立")
            return True
        except Exception as e:
            print(f"建立连接失败: {e}")
            if self.socket:
                self.socket.close()
                self.socket = None
            return False

    def process_packet(self, packet, src_ip=None, src_port=None, protocol=None):
        """处理单个数据包"""
        if IP not in packet:
            return

        # 检查源IP
        if src_ip and packet[IP].src != src_ip:
            return

        # 检查协议和源端口
        if protocol == 'tcp' and TCP in packet:
            if src_port and packet[TCP].sport != src_port:
                return
            conn_id = (packet[IP].src, packet[TCP].sport)
            self.connections[conn_id].append(packet)
        elif protocol == 'udp' and UDP in packet:
            if src_port and packet[UDP].sport != src_port:
                return
            conn_id = (packet[IP].src, packet[UDP].sport)
            self.connections[conn_id].append(packet)
        elif not protocol:  # 如果没有指定协议，则包含所有IP包
            if TCP in packet:
                if src_port and packet[TCP].sport != src_port:
                    return
                conn_id = (packet[IP].src, packet[TCP].sport)
                self.connections[conn_id].append(packet)
            elif UDP in packet:
                if src_port and packet[UDP].sport != src_port:
                    return
                conn_id = (packet[IP].src, packet[UDP].sport)
                self.connections[conn_id].append(packet)

    def response_reader(self, src_port):
        """持续读取服务器响应的线程函数"""
        while not self.stop_reading.is_set() and self.socket:
            try:
                # 使用socket接收数据
                data = self.socket.recv(4096)
                if data:
                    self.response_queue.put(data)
                    print(f"收到响应: {len(data)} 字节")
            except socket.timeout:
                continue
            except Exception as e:
                if not self.stop_reading.is_set():
                    print(f"读取响应时出错: {e}")
                break
            time.sleep(0.1)

    def replay_packets(self, src_ip=None, src_port=None, protocol=None, delay=0):
        """边读取边重放数据包"""
        print(f"开始读取并重放数据包到 {self.target_ip}:{self.target_port}")
        
        try:
            # 使用PcapReader逐包读取
            reader = PcapReader(self.pcap_file)
            packet_count = 0
            connection_established = False
            
            # 读取并处理数据包
            for packet in reader:
                packet_count += 1
                
                if IP not in packet:
                    continue
                    
                # 检查源IP
                if src_ip and packet[IP].src != src_ip:
                    continue
                    
                # 检查协议和源端口
                current_src_port = None
                if protocol == 'tcp' and TCP in packet:
                    if src_port and packet[TCP].sport != src_port:
                        continue
                    current_src_port = packet[TCP].sport
                elif protocol == 'udp' and UDP in packet:
                    if src_port and packet[UDP].sport != src_port:
                        continue
                    current_src_port = packet[UDP].sport
                elif not protocol:  # 如果没有指定协议，则包含所有IP包
                    if TCP in packet:
                        if src_port and packet[TCP].sport != src_port:
                            continue
                        current_src_port = packet[TCP].sport
                    elif UDP in packet:
                        if src_port and packet[UDP].sport != src_port:
                            continue
                        current_src_port = packet[UDP].sport
                    else:
                        continue
                else:
                    continue
                
                # 找到第一个符合条件的包，建立连接
                if not connection_established:
                    if not self.establish_tcp_connection(current_src_port):
                        print("无法建立连接，退出")
                        return
                    # 启动响应读取线程
                    self.stop_reading.clear()
                    reader_thread = threading.Thread(target=self.response_reader, args=(current_src_port,))
                    reader_thread.daemon = True
                    reader_thread.start()
                    connection_established = True
                
                # 发送当前数据包
                try:
                    if Raw in packet:
                        self.socket.send(packet[Raw].load)
                        packet_time = time.strftime("%H:%M:%S", time.localtime(float(packet.time)))
                        print(f"[{packet_time}] [序号:{packet_count}] 已发送数据包 (负载大小: {len(packet[Raw].load)} 字节)")
                        if delay > 0:
                            time.sleep(delay)
                except Exception as e:
                    print(f"发送数据包 {packet_count} 时出错: {e}")
                    sys.exit(1)  # 发送失败直接退出进程
            
            print(f"总共处理了 {packet_count} 个数据包")
            
        except Exception as e:
            print(f"处理数据包时出错: {e}")
            sys.exit(1)  # 其他错误也直接退出进程
        finally:
            # 关闭连接和停止读取线程
            self.stop_reading.set()
            if self.socket:
                self.socket.close()
                self.socket = None
            reader.close()

def main():
    parser = argparse.ArgumentParser(description='Wireshark数据包重放工具')
    parser.add_argument('pcap_file', help='pcap文件路径')
    parser.add_argument('target_ip', help='目标IP地址')
    parser.add_argument('target_port', type=int, help='目标端口')
    parser.add_argument('--delay', type=float, default=0, help='数据包发送间隔(秒)')
    parser.add_argument('--src-ip', help='过滤源IP地址')
    parser.add_argument('--src-port', type=int, help='过滤源端口')
    parser.add_argument('--protocol', choices=['tcp', 'udp'], help='过滤协议类型')

    args = parser.parse_args()

    replayer = PacketReplayer(args.pcap_file, args.target_ip, args.target_port)
    replayer.replay_packets(args.src_ip, args.src_port, args.protocol, args.delay)

if __name__ == '__main__':
    main() 