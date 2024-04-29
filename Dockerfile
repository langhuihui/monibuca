# Compile Stage 
FROM golang:1.22.2-alpine3.19 AS builder
MAINTAINER monibuca <awesome@monibuca.com>

LABEL stage=gobuilder

# Env 
ENV CGO_ENABLE 0
ENV GOOS linux 
ENV GOARCH amd64
ENV GOPROXY https://goproxy.cn,direct 
ENV HOME /monibuca 

COPY . /monibuca
WORKDIR /monibuca

# compile 
RUN go mod download 
RUN go build -ldflags="-s -w" -o /monibuca/build/monibuca ./main.go

RUN cp -r /monibuca/config.yaml /monibuca/build
RUN cp -r /monibuca/favicon.ico /monibuca/build

# Running Stage 
FROM alpine:latest

WORKDIR /monibuca 
COPY --from=builder /monibuca/build /monibuca/

# Export necessary ports 
EXPOSE 8080 8443 1935 554 58200-59200 5060 8000-9000
EXPOSE 5060/udp 58200-59200/udp 8000-9000/udp

CMD [ "./monibuca" ]
