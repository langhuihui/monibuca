#!/bin/bash

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # 无颜色

# 脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "${SCRIPT_DIR}"

# 配置参数
EXECUTABLE="./monibuca"
CONFIG_DIR="configs"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
LOG_FILE="monibuca.log"

# 检查可执行文件是否存在
if [ ! -f "${EXECUTABLE}" ]; then
    echo -e "${RED}错误: 可执行文件 ${EXECUTABLE} 不存在。${NC}"
    exit 1
fi

# 确保配置目录存在
mkdir -p "${CONFIG_DIR}"

# 检查配置文件是否存在
if [ ! -f "${CONFIG_FILE}" ]; then
    echo -e "${YELLOW}警告: 配置文件 ${CONFIG_FILE} 不存在。将使用默认配置。${NC}"
fi

# 创建日志目录
mkdir -p logs

# 函数：启动应用
start_app() {
    echo -e "${YELLOW}正在启动 Monibuca 服务...${NC}"
    
    # 检查服务是否已在运行
    if pgrep -f "${EXECUTABLE}" > /dev/null; then
        echo -e "${YELLOW}服务已在运行，先停止已有服务...${NC}"
        stop_app
    fi
    
    if [ -f "${CONFIG_FILE}" ]; then
        echo -e "${YELLOW}使用配置文件: ${CONFIG_FILE}${NC}"
        nohup ${EXECUTABLE} -c ${CONFIG_FILE} > ${LOG_FILE} 2>&1 &
    else
        echo -e "${YELLOW}使用默认配置${NC}"
        nohup ${EXECUTABLE} > ${LOG_FILE} 2>&1 &
    fi
    
    PID=$!
    echo -e "${GREEN}服务已启动，PID: ${PID}${NC}"
    echo -e "${YELLOW}查看日志: tail -f ${LOG_FILE}${NC}"
}

# 函数：停止应用
stop_app() {
    echo -e "${YELLOW}正在停止 Monibuca 服务...${NC}"
    
    PID=$(pgrep -f "${EXECUTABLE}")
    if [ -z "${PID}" ]; then
        echo -e "${YELLOW}未发现正在运行的服务。${NC}"
    else
        echo -e "${YELLOW}停止PID为 ${PID} 的服务${NC}"
        kill -15 ${PID}
        sleep 2
        
        # 检查服务是否已停止
        if pgrep -f "${EXECUTABLE}" > /dev/null; then
            echo -e "${YELLOW}服务未能正常停止，强制终止...${NC}"
            kill -9 ${PID}
        fi
        
        echo -e "${GREEN}服务已停止${NC}"
    fi
}

# 函数：检查状态
status_app() {
    PID=$(pgrep -f "${EXECUTABLE}")
    if [ -z "${PID}" ]; then
        echo -e "${YELLOW}Monibuca 服务未运行${NC}"
    else
        echo -e "${GREEN}Monibuca 服务正在运行，PID: ${PID}${NC}"
        
        # 显示运行时间
        UPTIME=$(ps -p ${PID} -o etime= | tr -d ' ')
        echo -e "${YELLOW}运行时间: ${UPTIME}${NC}"
        
        # 显示内存使用情况
        MEM=$(ps -p ${PID} -o %mem= | tr -d ' ')
        echo -e "${YELLOW}内存使用: ${MEM}%${NC}"
    fi
}

# 函数：查看日志
view_logs() {
    if [ -f "${LOG_FILE}" ]; then
        tail -n 100 -f ${LOG_FILE}
    else
        echo -e "${RED}日志文件不存在${NC}"
    fi
}

# 根据命令行参数执行相应操作
case "$1" in
    start)
        start_app
        ;;
    stop)
        stop_app
        ;;
    restart)
        stop_app
        sleep 2
        start_app
        ;;
    status)
        status_app
        ;;
    logs)
        view_logs
        ;;
    *)
        echo -e "${YELLOW}用法: $0 {start|stop|restart|status|logs}${NC}"
        echo -e "  start   - 启动服务"
        echo -e "  stop    - 停止服务"
        echo -e "  restart - 重启服务"
        echo -e "  status  - 查看服务状态"
        echo -e "  logs    - 查看服务日志"
        exit 1
        ;;
esac

exit 0 