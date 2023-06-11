#源镜像
FROM alpine:latest

WORKDIR /opt

ADD monibuca /opt
ADD config.yaml /opt
ADD local.monibuca.com.key /opt
ADD local.monibuca.com_bundle.pem /opt
#暴露端口
EXPOSE 8080 8081 1935 554 58200 5060 8000-9000
EXPOSE 5060/udp 58200/udp 8000-9000/udp
#最终运行docker的命令
ENTRYPOINT ["./monibuca"]