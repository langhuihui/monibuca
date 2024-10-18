
1. Alias handling logic when Publisher Start:

```mermaid
graph TD
    A[Publisher Start] --> B{Check if Publisher with same StreamPath exists}
    B -->|Yes| C[Call takeOver to handle old Publisher]
    B -->|No| D[Add new Publisher to Streams]
    D --> E[Wake up subscribers waiting for this StreamPath]
    E --> F[Iterate through AliasStreams]
    F --> G{Does Alias StreamPath match?}
    G -->|Yes| H{Is Alias Publisher empty?}
    H -->|Yes| I[Set Alias Publisher to new Publisher]
    H -->|No| J[Transfer Alias subscribers to new Publisher]
    G -->|No| K[Continue iteration]
    I --> L[Wake up subscribers waiting for this Alias]
    J --> L
    L --> M[End]
```

2. Alias handling when Publisher Dispose:

```mermaid
graph TD
    A[Publisher Dispose] --> B{Is it stopping due to being kicked out?}
    B -->|No| C[Remove Publisher from Streams]
    C --> D[Iterate through AliasStreams]
    D --> E{Does Alias point to this Publisher?}
    E -->|Yes| F{Auto-remove?}
    F -->|Yes| G[Remove Alias from AliasStreams]
    F -->|No| H[Retain Alias]
    E -->|No| I[Continue iteration]
    G --> J[Handle subscribers]
    H --> J
    J --> K[End]
```

3. Alias handling when Subscriber Start:

```mermaid
graph TD
    A[Subscriber Start] --> B{Check if matching Alias exists in AliasStreams}
    B -->|Yes| C{Does Alias Publisher exist?}
    C -->|Yes| D[Add subscriber to Alias Publisher]
    C -->|No| E[Trigger OnSubscribe event]
    B -->|No| F[Check if matching regex exists in StreamAlias]
    F -->|Yes| G[Create new AliasStream]
    G --> H{Does corresponding Publisher exist?}
    H -->|Yes| I[Add subscriber to Publisher]
    H -->|No| J[Trigger OnSubscribe event]
    F -->|No| K{Does corresponding Publisher exist in Streams?}
    K -->|Yes| L[Add subscriber to Publisher]
    K -->|No| M[Add subscriber to waiting list]
    M --> N[Trigger OnSubscribe event]
```

4. Logic for adding alias in API SetAliasStream call:

```mermaid
graph TD
    A[SetAliasStream - Add alias] --> B{Does alias already exist in AliasStreams?}
    B -->|Yes| C[Update existing AliasStream]
    B -->|No| D[Create new AliasStream]
    C --> E{Has StreamPath changed?}
    E -->|Yes| F{Does Publisher for new StreamPath exist?}
    F -->|Yes| G[Transfer subscribers to new Publisher]
    F -->|No| H[Wake up subscribers waiting for new StreamPath]
    E -->|No| I[End]
    D --> J{Does Publisher for StreamPath exist?}
    J -->|Yes| K[Replace existing stream or wake up waiting subscribers]
    J -->|No| L[End]
```

5. Logic for removing alias in API SetAliasStream call:

```mermaid
graph TD
    A[SetAliasStream - Remove alias] --> B{Does alias exist in AliasStreams?}
    B -->|Yes| C[Remove alias from AliasStreams]
    C --> D{Does Alias Publisher exist?}
    D -->|Yes| E{Does Publisher with same name exist in Streams?}
    E -->|Yes| F[Transfer Alias subscribers to same-name Publisher]
    E -->|No| G[End]
    D -->|No| H[End]
    B -->|No| I[End]
```


Based on your requirements, I'll create a flowchart representing the name mapping relationships for the `SetAliasStream` method. I'll use subgraphs to show the structural relationships for different conditions and include the effects on subscribers. Here's the flowchart:

````mermaid
graph TD
    subgraph SetAliasStream
        A[Start] --> B{Is StreamPath provided?}
        B -->|Yes| C[Parse StreamPath]
        B -->|No| D[Remove Alias]
        
        C --> E{Does Publisher exist for StreamPath?}
        E -->|Yes| F[Can Replace = true]
        E -->|No| G[Can Replace = false]
        G --> H[Trigger OnSubscribe]
        
        F --> I{Does Alias exist?}
        G --> I
        I -->|Yes| J[Modify Alias]
        I -->|No| K[Create Alias]
        
        J --> L{Has StreamPath changed?}
        L -->|Yes| M{Can Replace?}
        M -->|Yes| N[Transfer Subscribers or WakeUp]
        M -->|No| O[Update StreamPath]
        L -->|No| P[Update AutoRemove]
        
        K --> Q{Can Replace?}
        Q -->|Yes| R[Transfer Subscribers or WakeUp]
        Q -->|No| S[Add to AliasStreams]
        
        D --> T{Does Alias exist?}
        T -->|Yes| U[Remove from AliasStreams]
        T -->|No| V[End]
        
        U --> W{Does Publisher exist for Alias?}
        W -->|Yes| X[Transfer Subscribers]
        W -->|No| Y[End]
    end
    
    subgraph Effects on Subscribers
        N1[Transfer Subscribers]
        N2[WakeUp waiting Subscribers]
        X1[Transfer Subscribers to matching Publisher]
    end
    
    subgraph Streams and AliasStreams
        S1[Streams: StreamPath -> Publisher]
        S2[AliasStreams: Alias -> StreamPath]
    end
````

This flowchart represents the `SetAliasStream` method's logic, including:

1. Checking if a StreamPath is provided
2. Parsing the StreamPath and checking if a Publisher exists
3. Handling existing or new aliases
4. Updating StreamPath and AutoRemove settings
5. Transferring subscribers or waking up waiting subscribers when applicable
6. Removing aliases and handling potential subscriber transfers

The "Effects on Subscribers" subgraph shows the possible actions that affect subscribers:
- Transferring subscribers to a new Publisher
- Waking up waiting subscribers when a stream becomes available
- Transferring subscribers to a matching Publisher when removing an alias

The "Streams and AliasStreams" subgraph represents the relationship between StreamPaths, Publishers, and Aliases.

This flowchart provides a comprehensive view of the `SetAliasStream` method's logic and its effects on the stream naming and subscriber management.