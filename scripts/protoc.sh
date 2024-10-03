#!/bin/bash

name=$(basename $(pwd))
cd pb
# Run the protoc command
protoc -I. \
    -I"../../../pb" \
    --go_out=. \
    --go_opt=paths=source_relative \
    --go-grpc_out=. \
    --go-grpc_opt=paths=source_relative \
    --grpc-gateway_out=. \
    --grpc-gateway_opt=paths=source_relative \
    "${name}.proto"

# Check if the command was successful
if [ $? -eq 0 ]; then
    echo "Proto files for ${name} built successfully"
else
    echo "Error building proto files for ${name}"
    exit 1
fi