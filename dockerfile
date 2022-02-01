FROM golang:1.17.0-alpine3.14 as builder

LABEL maintainer="yangshuhai@pdnews.cn"

WORKDIR /app

# 先装好基础依赖，减少在代码变化时的重复构建时间
RUN echo "https://mirror.tuna.tsinghua.edu.cn/alpine/v3.14/main" > /etc/apk/repositories
RUN echo "https://mirror.tuna.tsinghua.edu.cn/alpine/v3.14/community" >> /etc/apk/repositories
RUN apk add --no-cache --update autoconf automake make gcc g++
RUN go env -w GO111MODULE=on
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go get -d github.com/Monibuca/engine/v3@v3.4.7
RUN go get -d github.com/Monibuca/plugin-gateway/v3@v3.0.10
RUN go get -d github.com/Monibuca/plugin-gb28181/v3@v3.0.2
RUN go get -d github.com/Monibuca/plugin-hdl/v3@v3.0.5
RUN go get -d github.com/Monibuca/plugin-hls/v3@v3.0.6
RUN go get -d github.com/Monibuca/plugin-jessica/v3@v3.0.0
RUN go get -d github.com/Monibuca/plugin-logrotate/v3@v3.0.0
RUN go get -d github.com/Monibuca/plugin-record/v3@v3.0.0
RUN go get -d github.com/Monibuca/plugin-rtmp/v3@v3.0.1
RUN go get -d github.com/Monibuca/plugin-rtsp/v3@v3.0.7
RUN go get -d github.com/Monibuca/plugin-summary@v1.0.0
RUN go get -d github.com/Monibuca/plugin-ts/v3@v3.0.1
RUN go get -d github.com/Monibuca/plugin-webrtc/v3@v3.0.3

# 再复制代码进行编译，可节省大量构建时间
COPY . .
RUN go mod tidy
RUN GOOS=linux go build -o m7s

# 构建完成则将成品复制到新的镜像中，减小镜像大小，可以考虑添加 upx 进一步减少空间
FROM alpine:3.14

WORKDIR /app
COPY --from=builder /app/m7s /app/m7s
COPY config.toml /app/config.toml
RUN /app/m7s --help

EXPOSE 554
EXPOSE 1935
EXPOSE 5060
EXPOSE 8081
EXPOSE 8082

CMD ["/app/m7s", "-c", "/app/config.toml"]
