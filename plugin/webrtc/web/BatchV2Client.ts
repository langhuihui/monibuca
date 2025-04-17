/**
 * BatchV2Client - WebRTC client for Monibuca BatchV2 protocol
 * Handles WebRTC connection, publishing, and subscribing to multiple streams
 */

// Types for stream information
interface StreamInfo {
  path: string;
  width: number;
  height: number;
  fps: number;
}

// Types for WebSocket messages
type MessageType =
  | { type: 'offer', sdp: string; }
  | { type: 'answer', sdp: string; }
  | { type: 'error', message: string; }
  | { type: 'streamList', streams: StreamInfo[]; }
  | { type: 'publish', streamPath: string, offer: string; }
  | { type: 'unpublish', streamPath: string; }
  | { type: 'subscribe', streamList: string[], offer: string; }
  | { type: 'unsubscribe', streamList: string[], offer: string; }
  | { type: 'getStreamList'; };

// Event types for the client
type EventType =
  | 'connected'
  | 'disconnected'
  | 'error'
  | 'streamList'
  | 'publishStarted'
  | 'publishStopped'
  | 'streamAdded'
  | 'streamRemoved'
  | 'iceStateChange'
  | 'connectionStateChange'
  | 'log';

type LogLevel = 'info' | 'error' | 'success' | 'warning';

// Event listener type
interface EventListener {
  (data: any): void;
}

class BatchV2Client {
  private ws: WebSocket | null = null;
  private pc: RTCPeerConnection | null = null;
  private localStream: MediaStream | null = null;
  private subscribedStreams: Set<string> = new Set();
  private videoSenders: Map<string, RTCRtpSender> = new Map();
  private streamToTransceiver: Map<string, RTCRtpTransceiver> = new Map();
  private eventListeners: Map<EventType, EventListener[]> = new Map();
  private wsUrl: string;

  /**
   * Create a new BatchV2Client
   * @param host Optional host for WebSocket connection. Defaults to current location
   */
  constructor(host?: string) {
    // Determine WebSocket URL
    const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    this.wsUrl = host ?
      `${wsProtocol}//${host}/webrtc/batchv2` :
      `${wsProtocol}//${location.host}/webrtc/batchv2`;
  }

  /**
   * Connect to the WebRTC server
   * @returns Promise that resolves when connection is established
   */
  public async connect(): Promise<void> {
    try {
      this.log(`Connecting to ${this.wsUrl}...`);

      // Create WebSocket connection
      this.ws = new WebSocket(this.wsUrl);

      return new Promise((resolve, reject) => {
        if (!this.ws) {
          reject(new Error('WebSocket not initialized'));
          return;
        }

        this.ws.onopen = async () => {
          this.log('WebSocket connection established', 'success');

          // Create and initialize PeerConnection
          const configuration: RTCConfiguration = {
            iceTransportPolicy: 'all',
            bundlePolicy: 'max-bundle',
            rtcpMuxPolicy: 'require',
            iceCandidatePoolSize: 1
          };

          this.pc = new RTCPeerConnection(configuration);

          // Use addTransceiver to create sender and receiver
          const videoTransceiver = this.pc.addTransceiver('video', {
            direction: 'sendrecv'
          });

          // Store sender reference
          this.videoSenders.set('placeholder', videoTransceiver.sender);

          this.log('Added placeholder tracks to PeerConnection', 'info');

          // Set up event handlers
          this.setupPeerConnectionEventHandlers();

          const offer = await this.pc.createOffer();
          await this.pc.setLocalDescription(offer);

          // Send offer to server
          this.sendMessage({
            type: 'offer',
            sdp: this.pc.localDescription!.sdp
          });

          this.emit('connected', null);
          resolve();
        };

        this.ws.onmessage = this.handleWebSocketMessage.bind(this);

        this.ws.onclose = () => {
          this.log('WebSocket connection closed');
          this.cleanup();
          this.emit('disconnected', null);
          reject(new Error('WebSocket connection closed'));
        };

        this.ws.onerror = (error) => {
          this.log(`WebSocket error: ${error}`, 'error');
          this.cleanup();
          this.emit('error', { message: 'WebSocket error' });
          reject(new Error('WebSocket error'));
        };
      });
    } catch (error: any) {
      this.log(`Connection error: ${error.message}`, 'error');
      this.cleanup();
      this.emit('error', { message: error.message });
      throw error;
    }
  }

  /**
   * Disconnect from the WebRTC server
   */
  public disconnect(): void {
    this.cleanup();
  }

  /**
   * Start publishing a stream
   * @param streamPath Path for the stream
   * @returns Promise that resolves when publishing starts
   */
  public async startPublishing(streamPath: string): Promise<void> {
    try {
      if (!streamPath) {
        throw new Error('Please enter a valid stream path');
      }

      if (!this.pc || !this.ws) {
        throw new Error('Not connected to server');
      }

      // Get user media - only video needed
      this.localStream = await navigator.mediaDevices.getUserMedia({
        video: true,
        audio: false
      });

      // Get actual video track
      const videoTrack = this.localStream.getVideoTracks()[0];

      // Use existing sender to replace track
      const videoSender = this.videoSenders.get('placeholder');

      if (videoSender) {
        await videoSender.replaceTrack(videoTrack);
        this.log('Replaced placeholder video track with real track', 'success');
      }

      // Update sender mapping
      this.videoSenders.delete('placeholder');
      this.videoSenders.set(streamPath, videoSender!);

      // Create new offer
      const offer = await this.pc.createOffer();
      await this.pc.setLocalDescription(offer);

      // Wait for ICE gathering to complete with a timeout
      await this.waitForIceGathering();

      // Send publish signal
      this.sendMessage({
        type: 'publish',
        streamPath: streamPath,
        offer: this.pc.localDescription!.sdp
      });

      this.log(`Started publishing to ${streamPath}`, 'success');
      this.emit('publishStarted', { streamPath });

      return Promise.resolve();
    } catch (error: any) {
      this.log(`Publishing error: ${error.message}`, 'error');
      this.emit('error', { message: error.message });
      throw error;
    }
  }

  /**
   * Stop publishing a stream
   * @param streamPath Path of the stream to stop
   * @returns Promise that resolves when publishing stops
   */
  public async stopPublishing(streamPath: string): Promise<void> {
    try {
      if (!this.pc || !this.ws) {
        throw new Error('Not connected to server');
      }

      // Get current sender
      const videoSender = this.videoSenders.get(streamPath);

      // Set track to null
      if (videoSender) {
        await videoSender.replaceTrack(null);
        this.log('Removed video track', 'info');
        // Update sender mapping
        this.videoSenders.delete(streamPath);
        this.videoSenders.set('placeholder', videoSender);
      }

      // Stop local stream
      if (this.localStream) {
        this.localStream.getTracks().forEach(track => track.stop());
        this.localStream = null;
      }

      // Create new offer
      const offer = await this.pc.createOffer();
      await this.pc.setLocalDescription(offer);

      // Wait for ICE gathering to complete with a timeout
      await this.waitForIceGathering();

      // Send unpublish signal
      this.sendMessage({
        type: 'unpublish',
        streamPath: streamPath
      });

      this.log(`Stopped publishing to ${streamPath}`, 'success');
      this.emit('publishStopped', { streamPath });

      return Promise.resolve();
    } catch (error: any) {
      this.log(`Error stopping publish: ${error.message}`, 'error');
      this.emit('error', { message: error.message });
      throw error;
    }
  }

  /**
   * Get list of available streams
   */
  public getStreamList(): void {
    if (!this.ws) {
      this.log('Not connected to server', 'error');
      return;
    }

    // Send getStreamList signal
    this.sendMessage({
      type: 'getStreamList'
    });

    this.log('Requested stream list', 'info');
  }

  /**
   * Subscribe to streams
   * @param streamPaths Array of stream paths to subscribe to
   * @returns Promise that resolves when subscription is complete
   */
  public async subscribeToStreams(streamPaths: string[]): Promise<void> {
    try {
      if (!this.pc || !this.ws) {
        throw new Error('Not connected to server');
      }

      if (streamPaths.length === 0) {
        throw new Error('Please select at least one stream');
      }

      // Get the current subscribed streams before clearing
      const previousStreams = new Set(this.subscribedStreams);

      // Clear current subscriptions
      this.subscribedStreams.clear();

      // Add all selected streams to the subscription list
      streamPaths.forEach(path => {
        this.subscribedStreams.add(path);
      });

      // Find streams that were previously subscribed but are no longer in the list
      const removedStreams: string[] = [];
      previousStreams.forEach(stream => {
        if (!this.subscribedStreams.has(stream)) {
          // Get the transceiver associated with this stream
          const transceiver = this.streamToTransceiver.get(stream);

          // Set the transceiver to inactive if it exists
          if (transceiver) {
            transceiver.direction = 'inactive';
            this.log(`Set transceiver for removed stream ${stream} to inactive`, 'info');
            this.streamToTransceiver.delete(stream);
          }

          // Add to removed streams list
          removedStreams.push(stream);

          // Emit stream removed event
          this.emit('streamRemoved', { streamPath: stream });
        }
      });

      // Send unsubscribe signal for removed streams
      if (removedStreams.length > 0) {
        await this.sendUnsubscribeSignal(removedStreams);
      }

      // Find streams that need to be newly added
      const newStreamPaths = Array.from(this.subscribedStreams).filter(
        path => !previousStreams.has(path)
      );

      this.log(`New stream paths: ${newStreamPaths.join(', ')}`, 'info');

      // If there are new streams to subscribe to
      if (newStreamPaths.length > 0) {
        // Get all video transceivers that are inactive
        const availableTransceivers = this.pc.getTransceivers().filter(
          transceiver => transceiver.direction === 'inactive'
        );

        this.log(`Available transceivers: ${availableTransceivers.length}`, 'info');

        // Use available transceivers for new streams
        let remainingNewStreams = [...newStreamPaths];
        while (remainingNewStreams.length > 0 && availableTransceivers.length > 0) {
          remainingNewStreams.pop();
          availableTransceivers.pop()!.direction = 'recvonly';
        }

        const transceiverToAdd = remainingNewStreams.length;

        // If available transceivers are not enough, create new ones
        if (transceiverToAdd > 0) {
          this.log(`Adding ${transceiverToAdd} new video transceivers`, 'info');

          for (let i = 0; i < transceiverToAdd; i++) {
            this.pc.addTransceiver('video', { direction: 'recvonly' });
          }
        }

        // Create offer
        const offer = await this.pc.createOffer();
        await this.pc.setLocalDescription(offer);

        // Send subscribe signal only for the new streams
        this.sendMessage({
          type: 'subscribe',
          streamList: newStreamPaths,
          offer: this.pc.localDescription!.sdp
        });

        this.log(`Subscribing to ${newStreamPaths.length} new streams`, 'success');
      }

      this.log(`Total playing streams: ${this.subscribedStreams.size}`, 'success');
      return Promise.resolve();
    } catch (error: any) {
      this.log(`Error playing streams: ${error.message}`, 'error');
      this.emit('error', { message: error.message });
      throw error;
    }
  }

  /**
   * Send unsubscribe signal to the server
   * @param streamPaths Array of stream paths to unsubscribe from
   * @returns Promise that resolves when the unsubscribe signal is sent
   */
  private async sendUnsubscribeSignal(streamPaths: string[]): Promise<void> {
    if (!this.ws || !this.pc) {
      this.log('Not connected to server', 'error');
      return;
    }

    if (streamPaths.length === 0) {
      return;
    }

    try {
      // Create offer for SDP exchange
      const offer = await this.pc.createOffer();
      await this.pc.setLocalDescription(offer);

      // Wait for ICE gathering to complete
      await this.waitForIceGathering();

      // Send unsubscribe signal with SDP
      this.sendMessage({
        type: 'unsubscribe',
        streamList: streamPaths,
        offer: this.pc.localDescription!.sdp
      });

      this.log(`Sent unsubscribe signal for ${streamPaths.length} streams`, 'info');
    } catch (error: any) {
      this.log(`Error sending unsubscribe signal: ${error.message}`, 'error');
      throw error;
    }
  }

  /**
   * Unsubscribe from a stream
   * @param streamPath Path of the stream to unsubscribe from
   * @returns Promise that resolves when unsubscription is complete
   */
  public async unsubscribeFromStream(streamPath: string): Promise<void> {
    try {
      if (!this.pc || !this.ws) {
        throw new Error('Not connected to server');
      }

      // Get the transceiver associated with this stream
      const transceiver = this.streamToTransceiver.get(streamPath);

      // Set the transceiver to inactive if it exists
      if (transceiver) {
        transceiver.direction = 'inactive';
        this.log(`Set transceiver for ${streamPath} to inactive`, 'info');
        this.streamToTransceiver.delete(streamPath);

        // Send unsubscribe signal with SDP exchange
        await this.sendUnsubscribeSignal([streamPath]);
      }

      this.subscribedStreams.delete(streamPath);
      this.emit('streamRemoved', { streamPath });

      this.log(`Removed ${streamPath} from subscription list`, 'info');
      return Promise.resolve();
    } catch (error: any) {
      this.log(`Error unsubscribing from stream: ${error.message}`, 'error');
      this.emit('error', { message: error.message });
      throw error;
    }
  }

  /**
   * Get the local media stream
   * @returns The local media stream or null if not publishing
   */
  public getLocalStream(): MediaStream | null {
    return this.localStream;
  }

  /**
   * Get the list of currently subscribed streams
   * @returns Array of stream paths
   */
  public getSubscribedStreams(): string[] {
    return Array.from(this.subscribedStreams);
  }

  /**
   * Add event listener
   * @param event Event type
   * @param listener Event listener function
   */
  public on(event: EventType, listener: EventListener): void {
    if (!this.eventListeners.has(event)) {
      this.eventListeners.set(event, []);
    }

    this.eventListeners.get(event)!.push(listener);
  }

  /**
   * Remove event listener
   * @param event Event type
   * @param listener Event listener function to remove
   */
  public off(event: EventType, listener: EventListener): void {
    if (!this.eventListeners.has(event)) {
      return;
    }

    const listeners = this.eventListeners.get(event)!;
    const index = listeners.indexOf(listener);

    if (index !== -1) {
      listeners.splice(index, 1);
    }
  }

  /**
   * Emit an event
   * @param event Event type
   * @param data Event data
   */
  private emit(event: EventType, data: any): void {
    if (!this.eventListeners.has(event)) {
      return;
    }

    const listeners = this.eventListeners.get(event)!;

    for (const listener of listeners) {
      listener(data);
    }
  }

  /**
   * Log a message and emit a log event
   * @param message Message to log
   * @param level Log level
   */
  private log(message: string, level: LogLevel = 'info'): void {
    this.emit('log', { message, level, time: new Date() });
  }

  /**
   * Set up event handlers for the peer connection
   */
  private setupPeerConnectionEventHandlers(): void {
    if (!this.pc) {
      return;
    }

    this.pc.onicecandidate = event => {
      if (event.candidate) {
        this.log('ICE candidate: ' + event.candidate.candidate);
      } else {
        this.log('ICE gathering complete');
      }
    };

    this.pc.onicegatheringstatechange = () => {
      this.log(`ICE gathering state: ${this.pc!.iceGatheringState}`);
      this.emit('iceStateChange', { state: this.pc!.iceGatheringState });
    };

    this.pc.oniceconnectionstatechange = () => {
      this.log(`ICE connection state: ${this.pc!.iceConnectionState}`);
      this.emit('iceStateChange', { state: this.pc!.iceConnectionState });

      if (this.pc!.iceConnectionState === 'failed') {
        this.log('ICE connection failed', 'error');
      }
    };

    this.pc.onconnectionstatechange = () => {
      this.log(`Connection state changed: ${this.pc!.connectionState}`);
      this.emit('connectionStateChange', { state: this.pc!.connectionState });

      if (this.pc!.connectionState === 'connected') {
        this.log('PeerConnection established successfully', 'success');
      }
    };

    this.pc.ontrack = this.handleTrackEvent.bind(this);
  }

  /**
   * Handle track events from the peer connection
   * @param event Track event
   */
  private handleTrackEvent(event: RTCTrackEvent): void {
    this.log(`Track received: ${event.track.kind}/${event.track.id}`, 'success');

    // Get transceiver directly from event
    const transceiver = event.transceiver;

    if (!transceiver) {
      this.log(`Could not find transceiver for track: ${event.track.id}`, 'warning');
    }

    // Add track statistics
    const stats: Record<string, number> = {};

    event.track.onunmute = () => {
      this.log(`Track unmuted: ${event.track.kind}/${event.track.id}`, 'success');
    };

    // Periodically get statistics
    const statsInterval = setInterval(async () => {
      if (!this.pc || this.pc.connectionState !== 'connected') {
        this.log('Connection state changed, stopping stats collection', 'info');
        clearInterval(statsInterval);
        return;
      }

      try {
        const rtcStats = await this.pc.getStats(event.track);
        rtcStats.forEach(stat => {
          if (stat.type === 'inbound-rtp' && stat.kind === event.track.kind) {
            const packetsReceived = stat.packetsReceived || 0;
            const prevPackets = stats[event.track.id] || 0;

            if (prevPackets !== packetsReceived) {
              stats[event.track.id] = packetsReceived;
            }
          }
        });
      } catch (e: any) {
        this.log(`Error getting stats: ${e.message}`, 'error');
      }
    }, 5000); // Update every 5 seconds

    if (event.track.kind === 'video' && event.streams[0]) {
      const streamId = event.streams[0].id;
      this.streamToTransceiver.set(streamId, transceiver);

      // Emit stream added event with stream information
      this.emit('streamAdded', {
        streamId,
        stream: event.streams[0],
        track: event.track
      });
    }
  }

  /**
   * Handle WebSocket messages
   * @param event WebSocket message event
   */
  private async handleWebSocketMessage(event: MessageEvent): Promise<void> {
    const message = JSON.parse(event.data) as MessageType;
    this.log(`Received message: ${(message as any).type}`);

    if ('type' in message) {
      if (message.type === 'answer') {
        const answer = new RTCSessionDescription({
          type: 'answer',
          sdp: message.sdp
        });

        await this.pc!.setRemoteDescription(answer);
        this.log('Remote description set', 'success');
      } else if (message.type === 'error') {
        this.log(`Error: ${message.message}`, 'error');
        this.emit('error', { message: message.message });
      } else if (message.type === 'streamList') {
        this.log(`Received stream list with ${message.streams.length} streams`, 'info');
        this.emit('streamList', { streams: message.streams });
      }
    }
  }

  /**
   * Send a message to the WebSocket server
   * @param message Message to send
   */
  private sendMessage(message: any): void {
    if (!this.ws) {
      this.log('Not connected to server', 'error');
      return;
    }

    this.ws.send(JSON.stringify(message));
  }

  /**
   * Wait for ICE gathering to complete with a timeout
   * @param timeout Timeout in milliseconds
   * @returns Promise that resolves when ICE gathering is complete or timeout is reached
   */
  private async waitForIceGathering(timeout: number = 2000): Promise<void> {
    if (!this.pc) {
      return Promise.reject(new Error('PeerConnection not initialized'));
    }

    return Promise.race([
      new Promise<void>(resolve => {
        if (this.pc!.iceGatheringState === 'complete') {
          resolve();
        } else {
          const checkState = () => {
            if (this.pc!.iceGatheringState === 'complete') {
              this.pc!.removeEventListener('icegatheringstatechange', checkState);
              resolve();
            }
          };
          this.pc!.addEventListener('icegatheringstatechange', checkState);
        }
      }),
      new Promise<void>(resolve => setTimeout(resolve, timeout))
    ]);
  }

  /**
   * Clean up all resources
   */
  private cleanup(): void {
    // Close WebSocket
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }

    // Close PeerConnection
    if (this.pc) {
      this.pc.close();
      this.pc = null;
    }

    // Stop local stream
    if (this.localStream) {
      this.localStream.getTracks().forEach(track => track.stop());
      this.localStream = null;
    }

    // Clear subscribed streams
    this.subscribedStreams.clear();

    // Clear senders and transceiver mappings
    this.videoSenders.clear();
    this.streamToTransceiver.clear();

    this.log('Connection cleaned up', 'info');
  }
}

export default BatchV2Client;
