#源镜像
FROM alpine:latest

WORKDIR /opt

ADD monibuca_linux /opt
ADD favicon.ico /opt
ADD config.yaml /opt
# RUN apk --no-cache add ffmpeg 
#暴露端口
EXPOSE 8080 8443 1935 554 58200 5060 8000-9000
EXPOSE 5060/udp 58200/udp 8000-9000/udp

#最终运行docker的命令
ENTRYPOINT ["./monibuca_linux"]