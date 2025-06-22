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
import heapq

class PacketReplayer:
    def __init__(self, pcap_file, target_ip, target_port):
        self.pcap_file = pcap_file
        self.target_ip = target_ip
        self.target_port = target_port
        self.connections = defaultdict(list)  # 存储每个连接的包序列
        self.response_queue = queue.Queue()
        self.stop_reading = threading.Event()
        self.socket = None
        self.next_seq = None  # 下一个期望的序列号
        self.pending_packets = []  # 使用优先队列存储待发送的包
        self.seen_packets = set()  # 用于去重
        self.initial_seq = None  # 初始序列号
        self.initial_ack = None  # 初始确认号
        self.client_ip = None  # 客户端IP
        self.client_port = None  # 客户端端口
        self.first_data_packet = True  # 标记是否是第一个数据包
        self.total_packets_sent = 0  # 发送的数据包数量
        self.total_bytes_sent = 0  # 发送的总字节数
        # 添加时间控制相关属性
        self.first_packet_time = None  # 第一个包的时间戳
        self.use_original_timing = True  # 是否使用原始时间间隔
        self.last_packet_time = None  # 上一个包的时间戳

    def establish_tcp_connection(self, src_port):
        """建立TCP连接"""
        print(f"正在建立TCP连接 {self.target_ip}:{self.target_port}...")
        try:
            self.socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            # 不绑定源端口，让系统自动分配
            self.socket.settimeout(5)
            self.socket.connect((self.target_ip, self.target_port))
            actual_port = self.socket.getsockname()[1]
            print(f"使用本地端口: {actual_port}")
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

        if src_ip and packet[IP].src != src_ip:
            return

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
        elif not protocol:
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

    def send_packet(self, packet, packet_count):
        """发送单个数据包，处理序列号"""
        if TCP not in packet or IP not in packet:
            return True

        try:
            # 检查是否是发送到目标端口的包
            if packet[TCP].dport == self.target_port:
                # 记录客户端信息
                if self.client_ip is None:
                    self.client_ip = packet[IP].src
                    self.client_port = packet[TCP].sport
                    print(f"识别到客户端: {self.client_ip}:{self.client_port}")

                # 获取TCP序列号和确认号
                seq = packet[TCP].seq
                ack = packet[TCP].ack
                flags = packet[TCP].flags

                # 打印数据包信息
                print(f"[序号:{packet_count}] 处理数据包: src={packet[IP].src}:{packet[TCP].sport} -> dst={packet[IP].dst}:{packet[TCP].dport}, seq={seq}, ack={ack}, flags={flags}")

                # 发送当前包
                if Raw in packet:
                    # 如果是第一个数据包，记录初始序列号
                    if self.first_data_packet:
                        self.initial_seq = seq
                        self.next_seq = seq
                        self.first_data_packet = False
                        print(f"第一个数据包，初始序列号: {seq}")

                    # 如果是重传包，跳过
                    if seq in self.seen_packets:
                        print(f"跳过重传包，序列号: {seq}")
                        return True

                    # 如果序列号大于期望的序列号，将包放入待发送队列
                    if seq > self.next_seq:
                        print(f"包乱序，放入队列，序列号: {seq}, 期望序列号: {self.next_seq}")
                        heapq.heappush(self.pending_packets, (seq, packet))
                        return True

                    payload = packet[Raw].load
                    print(f"准备发送数据包，负载大小: {len(payload)} 字节")
                    self.socket.send(payload)
                    self.seen_packets.add(seq)
                    old_seq = self.next_seq
                    self.next_seq = self.next_seq + len(payload)
                    print(f"更新序列号: {old_seq} -> {self.next_seq}")
                    
                    # 更新统计信息
                    self.total_packets_sent += 1
                    self.total_bytes_sent += len(payload)
                    
                    # 检查并发送待发送队列中的包
                    while self.pending_packets and self.pending_packets[0][0] == self.next_seq:
                        _, next_packet = heapq.heappop(self.pending_packets)
                        if Raw in next_packet:
                            next_payload = next_packet[Raw].load
                            print(f"发送队列中的包，负载大小: {len(next_payload)} 字节")
                            self.socket.send(next_payload)
                            self.seen_packets.add(self.next_seq)
                            old_seq = self.next_seq
                            self.next_seq += len(next_payload)
                            print(f"更新序列号: {old_seq} -> {self.next_seq}")
                            
                            # 更新统计信息
                            self.total_packets_sent += 1
                            self.total_bytes_sent += len(next_payload)
                    
                    packet_time = time.strftime("%H:%M:%S", time.localtime(float(packet.time)))
                    print(f"[{packet_time}] [序号:{packet_count}] 已发送数据包 (序列号: {seq}, 负载大小: {len(payload)} 字节)")
                else:
                    # 对于控制包，只记录到已处理集合
                    if flags & 0x02:  # SYN
                        print(f"[序号:{packet_count}] 处理SYN包")
                    elif flags & 0x10:  # ACK
                        print(f"[序号:{packet_count}] 处理ACK包")
                    else:
                        print(f"[序号:{packet_count}] 跳过无负载包")
            else:
                print(f"[序号:{packet_count}] 跳过非目标端口的包: src={packet[IP].src}:{packet[TCP].sport} -> dst={packet[IP].dst}:{packet[TCP].dport}")
            return True
        except Exception as e:
            print(f"发送数据包 {packet_count} 时出错: {e}")
            return False

    def response_reader(self, src_port):
        """持续读取服务器响应的线程函数"""
        while not self.stop_reading.is_set() and self.socket:
            try:
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
        print(f"使用原始时间间隔发送数据包")
        
        try:
            reader = PcapReader(self.pcap_file)
            packet_count = 0
            connection_established = False
            
            for packet in reader:
                packet_count += 1
                
                if IP not in packet:
                    continue
                    
                if src_ip and packet[IP].src != src_ip:
                    continue
                    
                current_src_port = None
                if protocol == 'tcp' and TCP in packet:
                    if src_port and packet[TCP].sport != src_port:
                        continue
                    current_src_port = packet[TCP].sport
                elif protocol == 'udp' and UDP in packet:
                    if src_port and packet[UDP].sport != src_port:
                        continue
                    current_src_port = packet[UDP].sport
                elif not protocol:
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
                
                if not connection_established:
                    if not self.establish_tcp_connection(current_src_port):
                        print("无法建立连接，退出")
                        return
                    self.stop_reading.clear()
                    reader_thread = threading.Thread(target=self.response_reader, args=(current_src_port,))
                    reader_thread.daemon = True
                    reader_thread.start()
                    connection_established = True
                
                # 处理时间间隔
                current_time = float(packet.time)
                if self.first_packet_time is None:
                    self.first_packet_time = current_time
                    self.last_packet_time = current_time
                elif self.use_original_timing:
                    # 计算与上一个包的时间差
                    time_diff = current_time - self.last_packet_time
                    if time_diff > 0:
                        print(f"等待 {time_diff:.3f} 秒后发送下一个包...")
                        time.sleep(time_diff)
                    self.last_packet_time = current_time
                
                if not self.send_packet(packet, packet_count):
                    print("发送数据包失败，退出")
                    return
                
                if delay > 0:
                    time.sleep(delay)
            
            print(f"\n统计信息:")
            print(f"总共处理了 {packet_count} 个数据包")
            print(f"成功发送了 {self.total_packets_sent} 个数据包")
            print(f"总共发送了 {self.total_bytes_sent} 字节数据")
            if self.first_packet_time is not None:
                total_time = float(self.last_packet_time - self.first_packet_time)
                print(f"总耗时: {total_time:.3f} 秒")
                print(f"平均发送速率: {self.total_bytes_sent / total_time:.2f} 字节/秒")
            
        except Exception as e:
            print(f"处理数据包时出错: {e}")
            sys.exit(1)
        finally:
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