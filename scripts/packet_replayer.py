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
from datetime import datetime

class PacketReplayer:
    def __init__(self, pcap_file, target_ip, target_port):
        self.pcap_file = pcap_file
        self.target_ip = target_ip
        self.target_port = target_port
        self.connections = defaultdict(list)  # 存��每个连接的包序列
        self.response_queue = queue.Queue()
        self.stop_reading = threading.Event()
        self.socket = None
        self.total_packets_sent = 0  # 发送的数据包数量
        self.total_bytes_sent = 0  # 发送的总字节数
        # 添加时间控制相关属性
        self.first_packet_time = None  # 第一个包的时间戳
        self.use_original_timing = True  # 是否使用原始时间间隔
        self.start_time = None  # 重放开始时间
        self.last_activity_time = None  # 最后活动时间
        self.keepalive_interval = 30.0  # 保活间隔(秒)
        self.connection_timeout = 60.0  # 连接超时时间(秒)
        # 简化的数据包管理
        self.data_packets = []  # 按时间顺序存储所有数据包
        self.processed_count = 0

    def log_with_timestamp(self, message):
        """带时间戳的日志输出"""
        timestamp = datetime.now().strftime("%H:%M:%S.%f")[:-3]
        print(f"[{timestamp}] {message}")

    def establish_tcp_connection(self, src_port):
        """建立TCP连接"""
        self.log_with_timestamp(f"正在建���TCP连接 {self.target_ip}:{self.target_port}...")
        try:
            self.socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            # 设置socket选项
            self.socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            self.socket.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
            self.socket.setsockopt(socket.SOL_SOCKET, socket.SO_KEEPALIVE, 1)

            # 设置较大的缓冲区
            self.socket.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 2*1024*1024)  # 2MB发送缓冲区
            self.socket.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 2*1024*1024)  # 2MB接收缓冲区

            self.socket.settimeout(self.connection_timeout)
            self.socket.connect((self.target_ip, self.target_port))
            actual_port = self.socket.getsockname()[1]
            self.last_activity_time = time.time()
            self.log_with_timestamp(f"使用本地端口: {actual_port}")
            self.log_with_timestamp("TCP连接已建立")
            return True
        except Exception as e:
            self.log_with_timestamp(f"建立连接失败: {e}")
            if self.socket:
                self.socket.close()
                self.socket = None
            return False

    def load_packets(self, src_ip=None, src_port=None, protocol=None):
        """预加载所有数据包，改进数据包收集逻辑"""
        self.log_with_timestamp("开始加载数据包...")

        try:
            reader = PcapReader(self.pcap_file)
            packet_count = 0
            total_pcap_packets = 0
            handshake_packets = []

            for packet in reader:
                total_pcap_packets += 1

                if IP not in packet or TCP not in packet:
                    continue

                packet_count += 1

                # 更宽松的过滤条件
                if src_ip and packet[IP].src != src_ip:
                    continue

                if src_port and packet[TCP].sport != src_port:
                    continue

                # 检查是否是目标连接的数据包（双向）
                is_target_connection = (
                    (packet[TCP].dport == self.target_port) or  # 发送到目标端口
                    (packet[TCP].sport == self.target_port)     # 从目标端口返回
                )

                if is_target_connection and Raw in packet:
                    payload = packet[Raw].load
                    packet_info = {
                        'timestamp': float(packet.time),
                        'payload': payload,
                        'seq': packet[TCP].seq,
                        'packet_count': packet_count,
                        'direction': 'to_server' if packet[TCP].dport == self.target_port else 'from_server'
                    }

                    # 检查是否是RTMP握手包（前几个包通常是握手）
                    if len(self.data_packets) < 20:
                        handshake_packets.append(packet_info)

                    # 只收集发送到服务器的包用于重放
                    if packet[TCP].dport == self.target_port:
                        self.data_packets.append(packet_info)

            # 按时间戳排序
            self.data_packets.sort(key=lambda x: x['timestamp'])

            total_data_size = sum(len(p['payload']) for p in self.data_packets)

            self.log_with_timestamp(f"PCAP文件总包数: {total_pcap_packets}")
            self.log_with_timestamp(f"TCP包数: {packet_count}")
            self.log_with_timestamp(f"发现握手包: {len(handshake_packets)}")
            self.log_with_timestamp(f"待发送数据包: {len(self.data_packets)}")
            self.log_with_timestamp(f"总数据量: {total_data_size / (1024*1024):.2f} MB")

            # 分析第一个包，检查是否是RTMP握手
            if self.data_packets:
                first_packet = self.data_packets[0]
                if len(first_packet['payload']) >= 4:
                    first_bytes = first_packet['payload'][:4]
                    self.log_with_timestamp(f"第一个包前4字节: {first_bytes.hex()}")

                    # RTMP握手C0包通常是03开头
                    if first_bytes[0] == 0x03:
                        self.log_with_timestamp("检测到RTMP握手包")
                    else:
                        self.log_with_timestamp("警告：第一个包可能不是RTMP握手包")

            reader.close()
            return True

        except Exception as e:
            self.log_with_timestamp(f"加载数据包时出错: {e}")
            import traceback
            traceback.print_exc()
            return False

    def send_data_with_flow_control(self, data, max_chunk_size=1024):
        """发送数据，使用极其严格的流控制来避免RTMP协议错误"""
        if not self.socket:
            return False

        total_sent = 0
        data_len = len(data)

        # 对于大数据包，强制增加延迟
        if data_len > 2000:
            time.sleep(0.02)  # 大包延迟20ms
        elif data_len > 1000:
            time.sleep(0.01)  # 中包延迟10ms
        else:
            time.sleep(0.005)  # 小包延迟5ms

        while total_sent < data_len:
            try:
                # 使用很小的块大小，确保不会超过RTMP消息边界
                remaining = data_len - total_sent
                chunk_size = min(max_chunk_size, remaining)

                chunk = data[total_sent:total_sent + chunk_size]
                sent = self.socket.send(chunk)

                if sent == 0:
                    self.log_with_timestamp("连接已断开")
                    return False

                total_sent += sent
                self.last_activity_time = time.time()

                # 如果没有完全发送当前块，继续发送剩余部分
                if sent < len(chunk):
                    self.log_with_timestamp(f"部分发送: {sent}/{len(chunk)} 字节")

                # 每个块之间都要延迟，给RTMP协议栈处理时间
                if total_sent < data_len:
                    time.sleep(0.002)  # 每块之间2ms延迟

                # 每发送512字节检查一次服务器响应
                if total_sent % 512 == 0:
                    try:
                        self.socket.settimeout(0.001)
                        response = self.socket.recv(1024)
                        if response:
                            self.response_queue.put(response)
                            self.last_activity_time = time.time()
                            # 收到响应后稍微等待，让服务器处理
                            time.sleep(0.001)
                    except socket.timeout:
                        pass  # 没有响应数据
                    except Exception:
                        pass  # 忽略其他错误
                    finally:
                        self.socket.settimeout(self.connection_timeout)

            except socket.timeout:
                self.log_with_timestamp(f"发送超时，已发送 {total_sent}/{data_len} 字节")
                return False
            except socket.error as e:
                self.log_with_timestamp(f"发送错误: {e}, 已发送 {total_sent}/{data_len} 字节")
                return False

        # 发送完一个完整包后，给服务器更多处理时间
        time.sleep(0.005)  # 包间延迟5ms
        return True

    def response_reader(self):
        """持续读取服务器响应"""
        while not self.stop_reading.is_set() and self.socket:
            try:
                self.socket.settimeout(5.0)
                data = self.socket.recv(8192)
                if data:
                    self.response_queue.put(data)
                    self.last_activity_time = time.time()
                    self.log_with_timestamp(f"收到响应: {len(data)} 字节")
                else:
                    self.log_with_timestamp("服务器关闭了连接")
                    break
            except socket.timeout:
                # 检查连接是否长时间无活动
                current_time = time.time()
                if self.last_activity_time and (current_time - self.last_activity_time) > self.keepalive_interval:
                    self.log_with_timestamp("连接长时间无活动，可能已断开")
                    break
                continue
            except Exception as e:
                if not self.stop_reading.is_set():
                    self.log_with_timestamp(f"读取响应时出错: {e}")
                break

    def calculate_timing(self, current_packet_time):
        """计算时间间隔"""
        if self.first_packet_time is None:
            self.first_packet_time = current_packet_time
            self.start_time = time.time()
            return 0

        # 计算应该等待的时间
        elapsed_in_capture = current_packet_time - self.first_packet_time
        elapsed_in_replay = time.time() - self.start_time
        wait_time = elapsed_in_capture - elapsed_in_replay

        # 限制最大等待时间
        max_wait = 5.0
        if wait_time > max_wait:
            self.log_with_timestamp(f"等待时间过长 ({wait_time:.3f}s)，限制为 {max_wait}s")
            wait_time = max_wait

        return max(0, wait_time)

    def replay_all_connections(self, src_ip=None, protocol=None, delay=0):
        """重放所有连接，检测到新连接时自动重连"""
        self.log_with_timestamp("开始加载所有连接的数据包...")
        
        try:
            reader = PcapReader(self.pcap_file)
            # 按源端口分组所有数据包
            connections = defaultdict(list)
            
            for packet in reader:
                if IP not in packet or TCP not in packet:
                    continue
                
                # 只收集发送到目标端口的包
                if packet[TCP].dport == self.target_port and Raw in packet:
                    src_port = packet[TCP].sport
                    packet_info = {
                        'timestamp': float(packet.time),
                        'payload': packet[Raw].load,
                        'seq': packet[TCP].seq,
                        'src_port': src_port
                    }
                    connections[src_port].append(packet_info)
            
            reader.close()
            
            if not connections:
                self.log_with_timestamp("没有找到任何连接")
                return
            
            self.log_with_timestamp(f"发现 {len(connections)} 个连接")
            for port, packets in sorted(connections.items()):
                total_size = sum(len(p['payload']) for p in packets)
                self.log_with_timestamp(f"  端口 {port}: {len(packets)} 个包, {total_size / (1024*1024):.2f} MB")
            
            # 按时间顺序处理所有连接
            self.log_with_timestamp("\n开始按时间顺序重放所有连接...")
            
            # 将所有连接的包合并并按时间排序
            all_packets = []
            for src_port, packets in connections.items():
                for pkt in packets:
                    all_packets.append(pkt)
            
            all_packets.sort(key=lambda x: x['timestamp'])
            self.log_with_timestamp(f"总共 {len(all_packets)} 个数据包")
            
            # 按连接分组重放
            current_connection = None
            connection_packets = []
            
            for pkt in all_packets:
                src_port = pkt['src_port']
                
                # 检测到新连接
                if current_connection != src_port:
                    # 先发送之前连接的数据
                    if connection_packets:
                        self.log_with_timestamp(f"\n[连接 {current_connection}] 发送 {len(connection_packets)} 个包")
                        self._send_connection_packets(connection_packets)
                    
                    # 开始新连接
                    current_connection = src_port
                    connection_packets = []
                    self.log_with_timestamp(f"\n[新连接] 端口 {src_port}")
                
                connection_packets.append(pkt)
            
            # 发送最后一个连接的数据
            if connection_packets:
                self.log_with_timestamp(f"\n[连接 {current_connection}] 发送 {len(connection_packets)} 个包")
                self._send_connection_packets(connection_packets)
            
            self.log_with_timestamp("\n所有连接重放完成")
            
        except Exception as e:
            self.log_with_timestamp(f"重放所有连接时出错: {e}")
            import traceback
            traceback.print_exc()
    
    def _send_connection_packets(self, packets):
        """发送单个连接的所有数据包"""
        # 按序列号重组TCP流
        packets.sort(key=lambda x: x['seq'])
        
        tcp_segments = []
        chunk_sizes = []
        chunk_timestamps = []
        expected_seq = packets[0]['seq']
        
        for pkt in packets:
            seq = pkt['seq']
            payload = pkt['payload']
            timestamp = pkt['timestamp']
            
            if seq == expected_seq:
                tcp_segments.append(payload)
                chunk_sizes.append(len(payload))
                chunk_timestamps.append(timestamp)
                expected_seq += len(payload)
            elif seq < expected_seq:
                overlap = expected_seq - seq
                if len(payload) > overlap:
                    tcp_segments.append(payload[overlap:])
                    chunk_sizes.append(len(payload[overlap:]))
                    chunk_timestamps.append(timestamp)
                    expected_seq = seq + len(payload)
            else:
                tcp_segments.append(payload)
                chunk_sizes.append(len(payload))
                chunk_timestamps.append(timestamp)
                expected_seq = seq + len(payload)
        
        tcp_stream = b''.join(tcp_segments)
        
        if not tcp_stream:
            self.log_with_timestamp("  警告：没有数据可发送")
            return
        
        self.log_with_timestamp(f"  重组完成: {len(tcp_stream) / (1024*1024):.2f} MB, {len(chunk_sizes)} 个包")
        
        # 建立新连接
        if not self.establish_tcp_connection(None):
            self.log_with_timestamp("  无法建立连接")
            return
        
        # 启动响应读取线程
        self.stop_reading.clear()
        reader_thread = threading.Thread(target=self.response_reader)
        reader_thread.daemon = True
        reader_thread.start()
        
        # 发送数据
        try:
            sent_bytes = 0
            chunk_index = 0
            replay_start_time = time.time()
            first_packet_time = chunk_timestamps[0] if chunk_timestamps else 0
            
            while sent_bytes < len(tcp_stream) and chunk_index < len(chunk_sizes):
                # 计算等待时间
                if chunk_index < len(chunk_timestamps):
                    target_time = chunk_timestamps[chunk_index] - first_packet_time
                    elapsed_time = time.time() - replay_start_time
                    wait_time = target_time - elapsed_time
                    
                    if wait_time > 0:
                        if wait_time > 5.0:
                            wait_time = 5.0
                        time.sleep(wait_time)
                
                current_chunk_size = chunk_sizes[chunk_index]
                remaining = len(tcp_stream) - sent_bytes
                current_chunk_size = min(current_chunk_size, remaining)
                
                chunk = tcp_stream[sent_bytes:sent_bytes + current_chunk_size]
                chunk_sent = 0
                
                while chunk_sent < len(chunk):
                    try:
                        self.socket.settimeout(10)
                        bytes_sent = self.socket.send(chunk[chunk_sent:])
                        
                        if bytes_sent == 0:
                            break
                        
                        chunk_sent += bytes_sent
                        sent_bytes += bytes_sent
                        self.total_bytes_sent += bytes_sent
                        self.last_activity_time = time.time()
                        
                    except socket.timeout:
                        continue
                    except socket.error:
                        break
                
                if chunk_sent == len(chunk):
                    chunk_index += 1
                else:
                    break
            
            self.log_with_timestamp(f"  发送完成: {sent_bytes / (1024*1024):.2f} MB")
            time.sleep(1)  # 短暂等待
            
        finally:
            self.stop_reading.set()
            if self.socket:
                try:
                    self.socket.shutdown(socket.SHUT_RDWR)
                except:
                    pass
                self.socket.close()
                self.socket = None

    def replay_packets(self, src_ip=None, src_port=None, protocol=None, delay=0):
        """重放数据包，采用正确的TCP流重组方式"""
        # 首先加载所有数据包
        if not self.load_packets(src_ip, src_port, protocol):
            return

        if not self.data_packets:
            self.log_with_timestamp("没有找到可发送的数据包")
            return

        # 正确的TCP流重组 - 按序列号重组！
        self.log_with_timestamp("重组TCP流...")

        # 第一步：按序列号排序
        self.data_packets.sort(key=lambda x: x['seq'])
        self.log_with_timestamp(f"按序列号排序完成，共 {len(self.data_packets)} 个数据包")

        # 第二步：正确重组TCP流，同时保留每个包的大小和时间戳信息
        tcp_segments = []
        chunk_sizes = []  # 保存每个原始包的大小
        chunk_timestamps = []  # 保存每个原始包的时间戳
        expected_seq = self.data_packets[0]['seq']
        self.log_with_timestamp(f"开始TCP重组，初始序列号: {expected_seq}")

        processed_packets = 0
        skipped_packets = 0

        for packet_info in self.data_packets:
            seq = packet_info['seq']
            payload = packet_info['payload']
            timestamp = packet_info['timestamp']

            if seq == expected_seq:
                # 序列号正确，添加到流中
                tcp_segments.append(payload)
                chunk_sizes.append(len(payload))  # 记录原始包大小
                chunk_timestamps.append(timestamp)  # 记录时间戳
                expected_seq += len(payload)
                processed_packets += 1
            elif seq < expected_seq:
                # 处理重叠数据
                overlap = expected_seq - seq
                if len(payload) > overlap:
                    # 去除重叠部分，添加剩余数据
                    tcp_segments.append(payload[overlap:])
                    chunk_sizes.append(len(payload[overlap:]))  # 记录实际添加的大小
                    chunk_timestamps.append(timestamp)  # 记录时间戳
                    expected_seq = seq + len(payload)
                    processed_packets += 1
                else:
                    # 完全重叠，跳过
                    skipped_packets += 1
            else:
                # 序列号大于期望值，说明有数据包丢失
                gap = seq - expected_seq
                self.log_with_timestamp(f"检测到数据包间隙: {gap} 字节，从序列号 {expected_seq} 到 {seq}")
                # 跳过间隙，继续处理
                tcp_segments.append(payload)
                chunk_sizes.append(len(payload))  # 记录原始包大小
                chunk_timestamps.append(timestamp)  # 记录时间戳
                expected_seq = seq + len(payload)
                processed_packets += 1

        # 合并所有段
        tcp_stream = b''.join(tcp_segments)

        self.log_with_timestamp(f"TCP流重组完成:")
        self.log_with_timestamp(f"  - 处理了 {processed_packets} 个数据包")
        self.log_with_timestamp(f"  - 跳过了 {skipped_packets} 个重复包")
        self.log_with_timestamp(f"  - 重组后大小: {len(tcp_stream) / (1024*1024):.2f} MB")

        # 验证RTMP握手
        if len(tcp_stream) >= 4:
            first_bytes = tcp_stream[:4]
            self.log_with_timestamp(f"重组流前4字节: {first_bytes.hex()}")
            if first_bytes[0] == 0x03:
                self.log_with_timestamp("重组流包含正确的RTMP握手")
            else:
                self.log_with_timestamp("警告：重组流可能不是有效的RTMP流")

        # 建立连接
        if not self.establish_tcp_connection(None):
            self.log_with_timestamp("无法建立连接")
            return

        # 启动响应读取线程
        self.stop_reading.clear()
        reader_thread = threading.Thread(target=self.response_reader)
        reader_thread.daemon = True
        reader_thread.start()

        self.log_with_timestamp(f"开始发送TCP流数据（使用动态chunk大小和原始时间间隔）")
        self.log_with_timestamp(f"原始数据包数量: {len(chunk_sizes)}")
        if chunk_sizes:
            avg_size = sum(chunk_sizes) / len(chunk_sizes)
            min_size = min(chunk_sizes)
            max_size = max(chunk_sizes)
            self.log_with_timestamp(f"包大小统计 - 平均: {avg_size:.0f}, 最小: {min_size}, 最大: {max_size}")
        if chunk_timestamps:
            duration = chunk_timestamps[-1] - chunk_timestamps[0]
            self.log_with_timestamp(f"抓包时长: {duration:.2f} 秒")

        try:
            sent_bytes = 0
            chunk_count = 0
            last_progress_time = time.time()
            self.start_time = time.time()
            replay_start_time = time.time()  # 重放开始时间
            first_packet_time = chunk_timestamps[0] if chunk_timestamps else 0  # 第一个包的时间戳
            chunk_index = 0

            while sent_bytes < len(tcp_stream) and chunk_index < len(chunk_sizes):
                current_time = time.time()

                # 每5秒显示一次进度
                if current_time - last_progress_time >= 5.0:
                    progress = (sent_bytes / len(tcp_stream)) * 100
                    elapsed = current_time - replay_start_time
                    self.log_with_timestamp(f"发送进度: {progress:.1f}% ({sent_bytes / (1024*1024):.2f}/{len(tcp_stream) / (1024*1024):.2f} MB) 已耗时: {elapsed:.1f}秒")
                    last_progress_time = current_time

                # 计算应该等待的时间（按照原始时间间隔）
                if chunk_index < len(chunk_timestamps):
                    target_time = chunk_timestamps[chunk_index] - first_packet_time
                    elapsed_time = time.time() - replay_start_time
                    wait_time = target_time - elapsed_time
                    
                    if wait_time > 0:
                        # 限制最大等待时间，避免异常
                        if wait_time > 5.0:
                            self.log_with_timestamp(f"警告：等待时间过长 {wait_time:.3f}秒，限制为5秒")
                            wait_time = 5.0
                        time.sleep(wait_time)

                # 使用原始数据包的大小作为chunk_size
                current_chunk_size = chunk_sizes[chunk_index]
                remaining = len(tcp_stream) - sent_bytes
                current_chunk_size = min(current_chunk_size, remaining)

                # 发送数据块 - 确保完全发送
                chunk = tcp_stream[sent_bytes:sent_bytes + current_chunk_size]
                chunk_sent = 0

                # 循环直到当前chunk完全发送
                while chunk_sent < len(chunk):
                    try:
                        self.socket.settimeout(10)  # 10秒超时
                        bytes_sent = self.socket.send(chunk[chunk_sent:])

                        if bytes_sent == 0:
                            self.log_with_timestamp("连接已断开")
                            break

                        chunk_sent += bytes_sent
                        sent_bytes += bytes_sent
                        self.total_bytes_sent += bytes_sent
                        self.last_activity_time = time.time()

                        # 如果部分发送，记录日志
                        if chunk_sent < len(chunk):
                            self.log_with_timestamp(f"部分发送: {chunk_sent}/{len(chunk)} 字节，继续发送剩余部分...")

                    except socket.timeout:
                        self.log_with_timestamp(f"发送数据块超时，重试...")
                        continue
                    except socket.error as e:
                        self.log_with_timestamp(f"发送数据块失败: {e}")
                        break

                # 检查是否完全发送
                if chunk_sent == len(chunk):
                    chunk_count += 1
                    chunk_index += 1
                else:
                    # 未完全发送，停止
                    self.log_with_timestamp(f"无法完全发送数据块，已发送 {chunk_sent}/{len(chunk)} 字节")
                    break

                # 移除固定的流控制延迟，让TCP自己控制

            self.log_with_timestamp(f"数据发送完成，等待服务器处理...")
            time.sleep(10)  # 增加等待时间到10秒

            self.log_with_timestamp(f"\n=== 发送完成 ===")
            self.log_with_timestamp(f"成功发送了 {chunk_count} 个数据块")
            self.log_with_timestamp(f"总共发送了 {self.total_bytes_sent} 字节数据 ({self.total_bytes_sent / (1024*1024):.2f} MB)")
            if self.start_time:
                total_time = time.time() - self.start_time
                self.log_with_timestamp(f"总耗时: {total_time:.3f} 秒")
                if total_time > 0:
                    self.log_with_timestamp(f"平均发送速率: {(self.total_bytes_sent / total_time) / (1024*1024):.2f} MB/s")

        except Exception as e:
            self.log_with_timestamp(f"重放过程中出错: {e}")
            import traceback
            traceback.print_exc()
        finally:
            self.stop_reading.set()
            if self.socket:
                self.log_with_timestamp("关闭TCP连接...")
                try:
                    self.socket.shutdown(socket.SHUT_RDWR)
                except:
                    pass
                self.socket.close()
                self.socket = None

    def list_all_connections(self):
        """列出所有连接"""
        self.log_with_timestamp("分析PCAP文件，列出所有连接...")
        try:
            reader = PcapReader(self.pcap_file)
            connections = defaultdict(lambda: defaultdict(int))

            for packet in reader:
                if IP not in packet or TCP not in packet:
                    continue

                # 统计每个源端口和目标端口的流量
                src_port = packet[TCP].sport
                dst_port = packet[TCP].dport
                payload_size = len(packet[Raw].load) if Raw in packet else 0

                connections[dst_port]['sent'] += payload_size
                connections[src_port]['received'] += payload_size

            reader.close()

            # 按照发送和接收的字节数排序
            sorted_connections = sorted(connections.items(), key=lambda item: item[1]['sent'] + item[1]['received'], reverse=True)

            self.log_with_timestamp("发现的连接:")
            for port, stats in sorted_connections:
                self.log_with_timestamp(f"端口 {port}: 发送 {stats['sent']} 字节, 接收 {stats['received']} 字节")

        except Exception as e:
            self.log_with_timestamp(f"列出连接时出错: {e}")
            import traceback
            traceback.print_exc()

    def find_largest_connection(self, src_ip=None):
        """自动选择数据量最大的连接"""
        self.log_with_timestamp("正在查找数据量最大的连接...")
        try:
            reader = PcapReader(self.pcap_file)
            connections = defaultdict(int)

            for packet in reader:
                if IP not in packet or TCP not in packet:
                    continue

                # 只统计发送到目标IP的数据
                if packet[IP].dst != self.target_ip:
                    continue

                # 统计每个源端口的流量
                src_port = packet[TCP].sport
                payload_size = len(packet[Raw].load) if Raw in packet else 0
                connections[src_port] += payload_size

            reader.close()

            # 找到数据量最大的源端口
            if connections:
                largest_port = max(connections.items(), key=lambda item: item[1])[0]
                self.log_with_timestamp(f"数据量最大的连接: 源端口 {largest_port}")
                return largest_port
            else:
                self.log_with_timestamp("未找到合适的连接")
                return None

        except Exception as e:
            self.log_with_timestamp(f"查找最大连接时出错: {e}")
            import traceback
            traceback.print_exc()
            return None

def main():
    parser = argparse.ArgumentParser(description='RTMP数据包重放工具 - 支持自动选择最大连接')
    parser.add_argument('pcap_file', help='pcap文件路径')
    parser.add_argument('target_ip', help='目标IP地址')
    parser.add_argument('target_port', type=int, help='目标端口')
    parser.add_argument('--delay', type=float, default=0, help='数据包发送间隔(秒)')
    parser.add_argument('--src-ip', help='过滤源IP地址')
    parser.add_argument('--src-port', type=int, help='过滤源端口 (留空自动选择最大连接)')
    parser.add_argument('--protocol', choices=['tcp', 'udp'], help='过滤协议类型')
    parser.add_argument('--auto-select', action='store_true', help='自动选择数据量最大的连接')
    parser.add_argument('--list-connections', action='store_true', help='列出所有连接并退出')
    parser.add_argument('--all', action='store_true', help='推送所有连接的数据，不区分源端口')

    args = parser.parse_args()

    replayer = PacketReplayer(args.pcap_file, args.target_ip, args.target_port)

    # 如果指定了列出连接，就分析并显示所有连接
    if args.list_connections:
        replayer.list_all_connections()
        return

    # 如果指定了--all，推送所有连接（自动重连）
    if args.all:
        print("推送整个pcap文件的所有连接（检测到新连接时自动重连）")
        replayer.replay_all_connections(args.src_ip, args.protocol, args.delay)
    # 如果启用自动选择或没有指定源端口，选择最大的连接
    elif args.auto_select or args.src_port is None:
        best_port = replayer.find_largest_connection(args.src_ip)
        if best_port:
            print(f"自动选择数据量最大的连接: 端口 {best_port}")
            replayer.replay_packets(args.src_ip, best_port, args.protocol, args.delay)
        else:
            print("未找到合适的连接")
    else:
        replayer.replay_packets(args.src_ip, args.src_port, args.protocol, args.delay)

if __name__ == '__main__':
    main()
