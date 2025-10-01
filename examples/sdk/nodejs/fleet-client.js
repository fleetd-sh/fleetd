#!/usr/bin/env node

/**
 * fleetd Platform SDK Example for Node.js
 *
 * Usage:
 *   npm install axios ws
 *   FLEET_API_URL=https://api.fleet.yourdomain.com FLEET_API_KEY=your-key node fleet-client.js
 */

const axios = require('axios');
const WebSocket = require('ws');

class FleetClient {
  constructor(baseURL, apiKey) {
    this.baseURL = baseURL;
    this.apiKey = apiKey;

    // Configure axios instance
    this.client = axios.create({
      baseURL: this.baseURL,
      headers: {
        'Content-Type': 'application/json',
        'X-API-Key': this.apiKey
      },
      timeout: 30000
    });
  }

  // Fleet Operations

  async createFleet(name, description, tags = {}) {
    try {
      const response = await this.client.post('/api/v1/fleets', {
        name,
        description,
        tags
      });
      return response.data;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  async listFleets(page = 1, limit = 20) {
    try {
      const response = await this.client.get('/api/v1/fleets', {
        params: { page, limit }
      });
      return response.data;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  async getFleet(fleetId) {
    try {
      const response = await this.client.get(`/api/v1/fleets/${fleetId}`);
      return response.data;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  async updateFleet(fleetId, updates) {
    try {
      const response = await this.client.put(`/api/v1/fleets/${fleetId}`, updates);
      return response.data;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  async deleteFleet(fleetId, cascade = false) {
    try {
      await this.client.delete(`/api/v1/fleets/${fleetId}`, {
        params: { cascade }
      });
      return true;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  // Device Operations

  async listDevices(fleetId, status = null) {
    try {
      const params = {};
      if (status) params.status = status;

      const response = await this.client.get(`/api/v1/fleets/${fleetId}/devices`, {
        params
      });
      return response.data;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  async getDevice(deviceId) {
    try {
      const response = await this.client.get(`/api/v1/devices/${deviceId}`);
      return response.data;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  async sendCommand(deviceId, command, payload = {}, timeout = 60) {
    try {
      const response = await this.client.post(`/api/v1/devices/${deviceId}/command`, {
        command,
        payload,
        timeout
      });
      return response.data;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  // Deployment Operations

  async createDeployment(name, fleetId, manifest, strategy = null) {
    try {
      const deploymentData = {
        name,
        fleet_id: fleetId,
        manifest
      };

      if (strategy) {
        deploymentData.strategy = strategy;
      } else {
        deploymentData.strategy = {
          type: 'rolling',
          rolling_update: {
            max_unavailable: '25%',
            max_surge: '25%'
          }
        };
      }

      const response = await this.client.post('/api/v1/deployments', deploymentData);
      return response.data;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  async listDeployments(fleetId = null, status = null) {
    try {
      const params = {};
      if (fleetId) params.fleet_id = fleetId;
      if (status) params.status = status;

      const response = await this.client.get('/api/v1/deployments', {
        params
      });
      return response.data;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  async getDeployment(deploymentId) {
    try {
      const response = await this.client.get(`/api/v1/deployments/${deploymentId}`);
      return response.data;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  async updateDeploymentStatus(deploymentId, action) {
    try {
      const response = await this.client.patch(`/api/v1/deployments/${deploymentId}`, {
        action
      });
      return response.data;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  // Telemetry Operations

  async queryMetrics(metric, startTime, endTime, options = {}) {
    try {
      const params = {
        metric,
        start_time: startTime,
        end_time: endTime,
        ...options
      };

      const response = await this.client.get('/api/v1/telemetry/metrics', {
        params
      });
      return response.data;
    } catch (error) {
      throw this.handleError(error);
    }
  }

  streamTelemetry(deviceId = null, fleetId = null, metrics = [], onMessage, onError) {
    const params = new URLSearchParams();
    if (deviceId) params.append('device_id', deviceId);
    if (fleetId) params.append('fleet_id', fleetId);
    if (metrics.length > 0) params.append('metrics', metrics.join(','));

    const wsUrl = this.baseURL.replace('http', 'ws') +
                  `/api/v1/telemetry/stream?${params.toString()}`;

    const ws = new WebSocket(wsUrl, {
      headers: {
        'X-API-Key': this.apiKey
      }
    });

    ws.on('open', () => {
      console.log('Telemetry stream connected');
    });

    ws.on('message', (data) => {
      try {
        const message = JSON.parse(data);
        onMessage(message);
      } catch (error) {
        console.error('Failed to parse telemetry message:', error);
      }
    });

    ws.on('error', (error) => {
      if (onError) onError(error);
      else console.error('Telemetry stream error:', error);
    });

    ws.on('close', () => {
      console.log('Telemetry stream closed');
    });

    return ws;
  }

  // Utility methods

  handleError(error) {
    if (error.response) {
      const { status, data } = error.response;
      const message = data.message || data.error || 'Unknown error';
      const err = new Error(`API Error (${status}): ${message}`);
      err.status = status;
      err.details = data;
      return err;
    }
    return error;
  }

  async waitForDeployment(deploymentId, pollInterval = 10000, timeout = 600000) {
    const startTime = Date.now();

    while (Date.now() - startTime < timeout) {
      const deployment = await this.getDeployment(deploymentId);

      if (deployment.status === 'succeeded') {
        return { success: true, deployment };
      }

      if (deployment.status === 'failed' || deployment.status === 'cancelled') {
        return { success: false, deployment };
      }

      console.log(`Deployment progress: ${deployment.progress.percentage.toFixed(1)}%`);
      await new Promise(resolve => setTimeout(resolve, pollInterval));
    }

    throw new Error('Deployment timeout');
  }
}

// Example usage
async function main() {
  const client = new FleetClient(
    process.env.FLEET_API_URL || 'https://api.fleet.yourdomain.com',
    process.env.FLEET_API_KEY || 'your-api-key'
  );

  try {
    // Create a fleet
    console.log('Creating fleet...');
    const fleet = await client.createFleet(
      'Test Fleet',
      'A test fleet for development',
      {
        environment: 'development',
        team: 'engineering'
      }
    );
    console.log('Fleet created:', fleet.id);

    // List all fleets
    console.log('\nListing fleets...');
    const { fleets } = await client.listFleets();
    fleets.forEach(f => {
      console.log(`  ${f.name}: ${f.device_count} devices (${f.online_count} online)`);
    });

    // List devices in fleet
    console.log('\nListing devices...');
    const { devices } = await client.listDevices(fleet.id);
    console.log(`Found ${devices.length} devices`);
    devices.forEach(d => {
      console.log(`  ${d.name}: ${d.status} (last seen: ${d.last_seen})`);
    });

    // Create a deployment
    console.log('\nCreating deployment...');
    const deployment = await client.createDeployment(
      'Firmware Update v2.0',
      fleet.id,
      {
        version: '2.0.0',
        artifact_url: 'https://updates.example.com/firmware-v2.0.bin',
        checksum: 'sha256:abcdef1234567890'
      }
    );
    console.log('Deployment created:', deployment.id);

    // Monitor deployment
    console.log('\nMonitoring deployment...');
    const result = await client.waitForDeployment(deployment.id);
    if (result.success) {
      console.log('Deployment succeeded!');
    } else {
      console.log('Deployment failed:', result.deployment.status);
    }

    // Query metrics
    console.log('\nQuerying metrics...');
    const endTime = new Date().toISOString();
    const startTime = new Date(Date.now() - 3600000).toISOString(); // 1 hour ago

    const metrics = await client.queryMetrics(
      'cpu_usage',
      startTime,
      endTime,
      {
        fleet_id: fleet.id,
        aggregation: 'avg',
        interval: '5m'
      }
    );
    console.log(`Average CPU usage: ${metrics.data_points.length} data points`);

    // Stream telemetry (example - runs for 30 seconds)
    console.log('\nStreaming telemetry...');
    const ws = client.streamTelemetry(
      null,
      fleet.id,
      ['cpu_usage', 'memory_usage'],
      (message) => {
        console.log('Telemetry:', message);
      },
      (error) => {
        console.error('Stream error:', error);
      }
    );

    // Close stream after 30 seconds
    setTimeout(() => {
      ws.close();
      console.log('Stream closed');
    }, 30000);

  } catch (error) {
    console.error('Error:', error.message);
    if (error.details) {
      console.error('Details:', error.details);
    }
  }
}

// Run if executed directly
if (require.main === module) {
  main().catch(console.error);
}

// Export for use as module
module.exports = FleetClient;