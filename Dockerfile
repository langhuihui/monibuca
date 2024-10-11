# Compile Stage 
FROM golang:1.23.1-bullseye AS builder


LABEL stage=gobuilder

# Env 
ENV CGO_ENABLED 0
ENV GOOS linux 
ENV GOARCH amd64
#ENV GOPROXY https://goproxy.cn,direct
ENV HOME /monibuca 

WORKDIR /

RUN git clone -b v5 --depth 1 https://github.com/langhuihui/monibuca

# compile 
WORKDIR /monibuca
RUN go build -tags sqlite -o ./build/monibuca ./example/default/main.go

RUN cp -r /monibuca/example/default/config.yaml /monibuca/build

# Running Stage 
FROM alpine:latest

WORKDIR /monibuca 
COPY --from=builder /monibuca/build /monibuca/
RUN cp -r ./config.yaml /etc/monibuca
# Export necessary ports 
EXPOSE 8080 8443 1935 554 5060 9000-20000
EXPOSE 5060/udp

CMD [ "./monibuca", "-c", "/etc/monibuca/config.yaml" ]
