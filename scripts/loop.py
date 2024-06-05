import subprocess
import time

# 命令行命令
command = "ffmpeg -re -stream_loop -1 -i /Users/dexter/Movies/jb-demo.mp4 -c copy -f flv rtmp://localhost/live/test"

# 循环执行命令
while True:
    # 启动命令行进程
    process = subprocess.Popen(command, shell=True)
    
    # 等待5秒
    time.sleep(5)
    
    # 关闭命令行进程
    process.terminate()
    
    # 等待1秒
    time.sleep(1)