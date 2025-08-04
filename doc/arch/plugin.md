# Plugin System

Monibuca adopts a plugin-based architecture design, extending functionality through its plugin mechanism. The plugin system is one of Monibuca's core features, allowing developers to add new functionality in a modular way without modifying the core code.

## Plugin Lifecycle

The plugin system has complete lifecycle management, including the following phases:

### 1. Registration Phase

Plugins are registered using the `InstallPlugin` generic function, during which:

- Plugin metadata (PluginMeta) is created, including:
  - Plugin name: automatically extracted from the plugin struct name (removing "Plugin" suffix)
  - Plugin version: extracted from the caller's file path or package path, defaults to "dev" if not extractable
  - Plugin type: obtained through reflection of the plugin struct type
  
- Optional features are registered:
  - Exit handler (OnExitHandler)
  - Default configuration (DefaultYaml)
  - Puller
  - Pusher
  - Recorder
  - Transformer
  - Publish authentication (AuthPublisher)
  - Subscribe authentication (AuthSubscriber)
  - gRPC service (ServiceDesc)
  - gRPC gateway handler (RegisterGRPCHandler)

- Plugin metadata is added to the global plugin list

The registration phase is the first stage in a plugin's lifecycle, providing the plugin system with basic information and functional definitions, preparing for subsequent initialization and startup.

### 2. Initialization Phase (Init)

Plugins are initialized through the `Plugin.Init` method, including these steps:

1. Instance Verification
   - Check if the plugin implements the IPlugin interface
   - Get plugin instance through reflection

2. Basic Setup
   - Set plugin metadata and server reference
   - Configure plugin logger
   - Set plugin name and version information

3. Environment Check
   - Check if plugin is disabled by environment variables ({PLUGIN_NAME}_ENABLE=false)
   - Check global disable status (DisableAll)
   - Check enable status in user configuration (enable)

4. Configuration Loading
   - Parse common configuration
   - Load default YAML configuration
   - Merge user configuration
   - Apply final configuration and log

5. Database Initialization (if needed)
   - Check database connection configuration (DSN)
   - Establish database connection
   - Auto-migrate database tables (for recording functionality)

6. Status Recording
   - Record plugin version
   - Record user configuration
   - Set log level
   - Record initialization status

If errors occur during initialization:
- Plugin is marked as disabled
- Disable reason is recorded
- Plugin is added to the disabled plugins list

The initialization phase prepares necessary environment and resources for plugin operation, crucial for ensuring normal plugin operation.

### 3. Startup Phase (Start)

Plugins start through the `Plugin.Start` method, executing these operations in sequence:

1. gRPC Service Registration (if configured)
   - Register gRPC service
   - Register gRPC gateway handler
   - Handle gRPC-related errors

2. Plugin Management
   - Add plugin to server's plugin list
   - Set plugin status to running

3. Network Listener Initialization
   - Start HTTP/HTTPS services
   - Start TCP/TLS services (if implementing ITCPPlugin interface)
   - Start UDP services (if implementing IUDPPlugin interface)
   - Start QUIC services (if implementing IQUICPlugin interface)

4. Plugin Initialization Callback
   - Call plugin's Start method
   - Handle initialization errors

5. Timer Task Setup
   - Configure server keepalive task (if enabled)
   - Set up other timer tasks

If errors occur during startup:
- Error reason is recorded
- Plugin is marked as disabled
- Subsequent startup steps are stopped

The startup phase is crucial for plugins to begin providing services, with all preparations completed and ready for business logic processing.

### 4. Stop Phase (Stop)

The plugin stop phase is implemented through the `Plugin.OnDispose` method and related stop handling logic, including:

1. Service Shutdown
   - Stop all network services (HTTP/HTTPS/TCP/UDP/QUIC)
   - Close all network connections
   - Stop processing new requests

2. Resource Cleanup
   - Stop all timer tasks
   - Close database connections (if any)
   - Clean up temporary files and cache

3. Status Handling
   - Update plugin status to stopped
   - Remove from server's active plugin list
   - Trigger stop event notifications

4. Callback Processing
   - Call plugin's custom OnDispose method
   - Execute registered stop callback functions
   - Handle errors during stop process

5. Connection Handling
   - Wait for current request processing to complete
   - Gracefully close existing connections
   - Reject new connection requests

The stop phase aims to ensure plugins can safely and cleanly stop running without affecting other parts of the system.

### 5. Destroy Phase (Destroy)

The plugin destroy phase is implemented through the `Plugin.Dispose` method, the final phase in a plugin's lifecycle, including:

1. Resource Release
   - Call plugin's OnDispose method for stop processing
   - Remove from server's plugin list
   - Release all allocated system resources

2. Status Cleanup
   - Clear all plugin status information
   - Reset plugin internal variables
   - Clear plugin configuration information

3. Connection Disconnection
   - Disconnect all connections with other plugins
   - Clean up plugin dependencies
   - Remove event listeners

4. Data Cleanup
   - Clean up temporary data generated by plugin
   - Close and clean up database connections
   - Delete unnecessary files

5. Final Processing
   - Execute registered destroy callback functions
   - Log destruction
   - Ensure all resources are properly released

The destroy phase aims to ensure plugins completely clean up all resources, leaving no residual state, preventing memory and resource leaks. 