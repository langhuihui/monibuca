#!/bin/bash

protoc -I. --go_out=. --go_opt=paths=source_relative --go-grpc_out=. \
--go-grpc_opt=paths=source_relative --grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative ${1}.proto