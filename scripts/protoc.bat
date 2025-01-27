@echo off
setlocal enabledelayedexpansion

echo Starting protoc compilation for: %1

if "%1"=="" (
    if not exist "pb" (
        echo Error: pb directory not found
        exit /b 1
    )
    cd pb
    for %%f in (*.proto) do (
        protoc -I. ^
            --go_out=. ^
            --go_opt=paths=source_relative ^
            --go-grpc_out=. ^
            --go-grpc_opt=paths=source_relative ^
            --grpc-gateway_out=. ^
            --grpc-gateway_opt=paths=source_relative ^
            "%%f"
    )
    
    if !errorlevel! equ 0 (
        echo Proto files for global built successfully
    ) else (
        echo Error building proto files for global
        exit /b 1
    )
) else (
    set "name=%~1"
    echo Checking directory: plugin\!name!\pb
    if not exist "plugin\!name!\pb" (
        echo Error: plugin\!name!\pb directory not found
        echo Current directory: %CD%
        exit /b 1
    )
    cd "plugin\!name!\pb" || (
        echo Error: Failed to change directory to plugin\!name!\pb
        exit /b 1
    )
    echo Current directory after cd: %CD%
    for %%f in (*.proto) do (
        echo Processing proto file: %%f
        protoc -I. ^
            -I"..\..\..\pb" ^
            --go_out=. ^
            --go_opt=paths=source_relative ^
            --go-grpc_out=. ^
            --go-grpc_opt=paths=source_relative ^
            --grpc-gateway_out=. ^
            --grpc-gateway_opt=paths=source_relative ^
            "%%f"
    )
    
    if !errorlevel! equ 0 (
        echo Proto files for !name! built successfully
    ) else (
        echo Error building proto files for !name!
        exit /b 1
    )
) 