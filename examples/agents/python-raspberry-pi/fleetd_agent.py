#!/usr/bin/env python3
"""
fleetd Device Agent for Raspberry Pi
A Python implementation of the fleetd device agent for IoT devices.
"""

import os
import sys
import json
import time
import logging
import threading
import subprocess
from typing import Dict, Any, Optional, List
from dataclasses import dataclass
from datetime import datetime, timedelta
import requests
import psutil
import platform
from queue import Queue, Empty
import hashlib
import hmac

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger('fleetd-agent')


@dataclass
class AgentConfig:
    """Agent configuration"""
    server_url: str
    device_token: Optional[str] = None
    hardware_id: Optional[str] = None
    heartbeat_interval: int = 60
    telemetry_interval: int = 30
    command_poll_interval: int = 10
    log_level: str = 'INFO'
    enrollment_token: Optional[str] = None
    device_type: str = 'raspberry-pi'
    ca_cert_path: Optional[str] = None


class FleetDeviceAgent:
    """fleetd Device Agent for Raspberry Pi"""

    def __init__(self, config: AgentConfig):
        self.config = config
        self.device_id: Optional[str] = None
        self.running = False
        self.command_queue = Queue()
        self.session = requests.Session()

        # Set up SSL verification if CA cert is provided
        if config.ca_cert_path:
            self.session.verify = config.ca_cert_path

        # Get or generate hardware ID
        if not config.hardware_id:
            self.config.hardware_id = self._get_hardware_id()

        # Set up session headers
        if config.device_token:
            self.session.headers.update({
                'X-Device-Token': config.device_token
            })

    def _get_hardware_id(self) -> str:
        """Get unique hardware ID for Raspberry Pi"""
        try:
            # Try to get Raspberry Pi serial number
            with open('/proc/cpuinfo', 'r') as f:
                for line in f:
                    if line.startswith('Serial'):
                        return line.split(':')[1].strip()
        except:
            pass

        # Fall back to MAC address
        import uuid
        return ':'.join(['{:02x}'.format((uuid.getnode() >> ele) & 0xff)
                        for ele in range(0, 8*6, 8)][::-1])

    def _get_system_info(self) -> Dict[str, Any]:
        """Get system information"""
        try:
            # Get network interfaces
            interfaces = []
            for name, addrs in psutil.net_if_addrs().items():
                interface = {
                    'name': name,
                    'ip_addresses': [],
                    'state': 'up' if psutil.net_if_stats()[name].isup else 'down'
                }
                for addr in addrs:
                    if addr.family == 2:  # IPv4
                        interface['ip_addresses'].append(addr.address)
                    elif addr.family == 17:  # MAC
                        interface['mac_address'] = addr.address
                interfaces.append(interface)

            return {
                'os': platform.system(),
                'os_version': platform.release(),
                'kernel': platform.version(),
                'architecture': platform.machine(),
                'hostname': platform.node(),
                'cpu_model': platform.processor() or 'Unknown',
                'cpu_cores': psutil.cpu_count(),
                'cpu_frequency': psutil.cpu_freq().current if psutil.cpu_freq() else 0,
                'memory_total': psutil.virtual_memory().total // (1024 * 1024),
                'memory_available': psutil.virtual_memory().available // (1024 * 1024),
                'disk_total': psutil.disk_usage('/').total // (1024 * 1024 * 1024),
                'disk_available': psutil.disk_usage('/').free // (1024 * 1024 * 1024),
                'network_interfaces': interfaces
            }
        except Exception as e:
            logger.error(f"Failed to get system info: {e}")
            return {}

    def _get_telemetry_metrics(self) -> List[Dict[str, Any]]:
        """Get telemetry metrics"""
        metrics = []
        timestamp = datetime.utcnow().isoformat() + 'Z'

        try:
            # CPU usage
            metrics.append({
                'name': 'cpu_usage',
                'value': psutil.cpu_percent(interval=1),
                'unit': 'percent',
                'timestamp': timestamp
            })

            # Memory usage
            memory = psutil.virtual_memory()
            metrics.append({
                'name': 'memory_usage',
                'value': memory.percent,
                'unit': 'percent',
                'timestamp': timestamp
            })

            # Disk usage
            disk = psutil.disk_usage('/')
            metrics.append({
                'name': 'disk_usage',
                'value': disk.percent,
                'unit': 'percent',
                'timestamp': timestamp
            })

            # Network I/O
            net_io = psutil.net_io_counters()
            metrics.append({
                'name': 'network_rx',
                'value': net_io.bytes_recv,
                'unit': 'bytes',
                'timestamp': timestamp
            })
            metrics.append({
                'name': 'network_tx',
                'value': net_io.bytes_sent,
                'unit': 'bytes',
                'timestamp': timestamp
            })

            # Temperature (Raspberry Pi specific)
            try:
                temp = subprocess.check_output(
                    ['vcgencmd', 'measure_temp'],
                    universal_newlines=True
                )
                temp_value = float(temp.split('=')[1].split("'")[0])
                metrics.append({
                    'name': 'cpu_temperature',
                    'value': temp_value,
                    'unit': 'celsius',
                    'timestamp': timestamp
                })
            except:
                pass  # Not a Raspberry Pi or vcgencmd not available

        except Exception as e:
            logger.error(f"Failed to get telemetry metrics: {e}")

        return metrics

    def discover(self) -> Dict[str, Any]:
        """Discover fleet services"""
        try:
            response = self.session.get(
                f"{self.config.server_url}/api/v1/discovery",
                timeout=10
            )
            response.raise_for_status()
            return response.json()
        except Exception as e:
            logger.error(f"Discovery failed: {e}")
            raise

    def enroll(self) -> bool:
        """Enroll device with fleet"""
        try:
            logger.info("Enrolling device...")

            # Prepare enrollment data
            enrollment_data = {
                'hardware_id': self.config.hardware_id,
                'device_type': self.config.device_type,
                'capabilities': [
                    'telemetry',
                    'remote_command',
                    'ota_update',
                    'log_upload'
                ],
                'metadata': self._get_system_info()
            }

            if self.config.enrollment_token:
                enrollment_data['enrollment_token'] = self.config.enrollment_token

            # Send enrollment request
            response = self.session.post(
                f"{self.config.server_url}/api/v1/enroll",
                json=enrollment_data,
                timeout=30
            )

            if response.status_code == 409:
                logger.info("Device already enrolled")
                return True

            response.raise_for_status()
            enrollment_response = response.json()

            # Store device credentials
            self.device_id = enrollment_response['device_id']
            self.config.device_token = enrollment_response['device_token']

            # Update session headers
            self.session.headers.update({
                'X-Device-Token': self.config.device_token
            })

            # Save credentials to file
            self._save_credentials(enrollment_response)

            logger.info(f"Device enrolled successfully: {self.device_id}")
            return True

        except Exception as e:
            logger.error(f"Enrollment failed: {e}")
            return False

    def _save_credentials(self, enrollment_data: Dict[str, Any]):
        """Save device credentials to file"""
        credentials_file = os.path.expanduser('~/.fleetd/credentials.json')
        os.makedirs(os.path.dirname(credentials_file), exist_ok=True)

        with open(credentials_file, 'w') as f:
            json.dump({
                'device_id': enrollment_data['device_id'],
                'device_token': enrollment_data['device_token'],
                'fleet_id': enrollment_data.get('fleet_id'),
                'enrolled_at': datetime.utcnow().isoformat()
            }, f, indent=2)

        # Secure file permissions
        os.chmod(credentials_file, 0o600)

    def _load_credentials(self) -> bool:
        """Load saved credentials"""
        credentials_file = os.path.expanduser('~/.fleetd/credentials.json')

        if not os.path.exists(credentials_file):
            return False

        try:
            with open(credentials_file, 'r') as f:
                credentials = json.load(f)

            self.device_id = credentials['device_id']
            self.config.device_token = credentials['device_token']

            # Update session headers
            self.session.headers.update({
                'X-Device-Token': self.config.device_token
            })

            logger.info(f"Loaded credentials for device: {self.device_id}")
            return True

        except Exception as e:
            logger.error(f"Failed to load credentials: {e}")
            return False

    def send_heartbeat(self):
        """Send heartbeat to server"""
        try:
            heartbeat_data = {
                'timestamp': datetime.utcnow().isoformat() + 'Z',
                'uptime': int(time.time() - psutil.boot_time()),
                'status': 'idle',
                'version': '1.0.0'
            }

            response = self.session.post(
                f"{self.config.server_url}/api/v1/device/heartbeat",
                json=heartbeat_data,
                timeout=10
            )
            response.raise_for_status()

            result = response.json()
            if result.get('pending_commands', 0) > 0:
                self.command_queue.put('poll_commands')

            logger.debug("Heartbeat sent successfully")
            return True

        except Exception as e:
            logger.error(f"Failed to send heartbeat: {e}")
            return False

    def send_telemetry(self):
        """Send telemetry data to server"""
        try:
            telemetry_data = {
                'device_time': datetime.utcnow().isoformat() + 'Z',
                'metrics': self._get_telemetry_metrics()
            }

            response = self.session.post(
                f"{self.config.server_url}/api/v1/device/telemetry",
                json=telemetry_data,
                timeout=10
            )
            response.raise_for_status()

            logger.debug("Telemetry sent successfully")
            return True

        except Exception as e:
            logger.error(f"Failed to send telemetry: {e}")
            return False

    def poll_commands(self):
        """Poll for pending commands"""
        try:
            response = self.session.get(
                f"{self.config.server_url}/api/v1/device/commands",
                params={'timeout': 30},
                timeout=35
            )

            if response.status_code == 204:
                return  # No commands

            response.raise_for_status()
            commands = response.json().get('commands', [])

            for command in commands:
                self._execute_command(command)

        except Exception as e:
            logger.error(f"Failed to poll commands: {e}")

    def _execute_command(self, command: Dict[str, Any]):
        """Execute a command"""
        command_id = command['id']
        command_type = command['command']
        payload = command.get('payload', {})

        logger.info(f"Executing command: {command_type} (ID: {command_id})")

        try:
            # Acknowledge command receipt
            self._acknowledge_command(command_id, 'received')

            # Execute based on command type
            if command_type == 'reboot':
                self._acknowledge_command(command_id, 'executing')
                subprocess.run(['sudo', 'reboot'], check=True)

            elif command_type == 'execute':
                self._acknowledge_command(command_id, 'executing')
                script = payload.get('script', '')
                timeout = payload.get('timeout', 60)

                result = subprocess.run(
                    script,
                    shell=True,
                    capture_output=True,
                    text=True,
                    timeout=timeout
                )

                self._acknowledge_command(
                    command_id,
                    'completed' if result.returncode == 0 else 'failed',
                    result={
                        'stdout': result.stdout,
                        'stderr': result.stderr,
                        'returncode': result.returncode
                    }
                )

            elif command_type == 'update':
                self._handle_update_command(command_id, payload)

            elif command_type == 'collect_logs':
                self._collect_and_upload_logs(command_id, payload)

            else:
                self._acknowledge_command(
                    command_id,
                    'failed',
                    error=f"Unknown command type: {command_type}"
                )

        except Exception as e:
            logger.error(f"Command execution failed: {e}")
            self._acknowledge_command(
                command_id,
                'failed',
                error=str(e)
            )

    def _acknowledge_command(self, command_id: str, status: str,
                           message: str = None, result: Dict = None,
                           error: str = None):
        """Acknowledge command status"""
        try:
            ack_data = {'status': status}
            if message:
                ack_data['message'] = message
            if result:
                ack_data['result'] = result
            if error:
                ack_data['error'] = error

            response = self.session.post(
                f"{self.config.server_url}/api/v1/device/commands/{command_id}/ack",
                json=ack_data,
                timeout=10
            )
            response.raise_for_status()

        except Exception as e:
            logger.error(f"Failed to acknowledge command: {e}")

    def _handle_update_command(self, command_id: str, payload: Dict[str, Any]):
        """Handle update command"""
        update_id = payload.get('update_id')
        update_url = payload.get('download_url')
        checksum = payload.get('checksum')

        try:
            # Download update
            self._report_update_status(update_id, 'downloading', 0)
            update_file = self._download_update(update_url, update_id)

            # Verify checksum
            self._report_update_status(update_id, 'verifying', 50)
            if not self._verify_checksum(update_file, checksum):
                raise ValueError("Checksum verification failed")

            # Install update
            self._report_update_status(update_id, 'installing', 75)
            self._install_update(update_file)

            # Report success
            self._report_update_status(update_id, 'completed', 100)
            self._acknowledge_command(command_id, 'completed')

            # Schedule reboot if needed
            if payload.get('reboot_required'):
                logger.info("Scheduling reboot in 10 seconds...")
                time.sleep(10)
                subprocess.run(['sudo', 'reboot'])

        except Exception as e:
            logger.error(f"Update failed: {e}")
            self._report_update_status(update_id, 'failed', error=str(e))
            self._acknowledge_command(command_id, 'failed', error=str(e))

    def _download_update(self, url: str, update_id: str) -> str:
        """Download update file"""
        update_dir = os.path.expanduser('~/.fleetd/updates')
        os.makedirs(update_dir, exist_ok=True)
        update_file = os.path.join(update_dir, f"{update_id}.bin")

        with requests.get(url, stream=True) as r:
            r.raise_for_status()
            total_size = int(r.headers.get('content-length', 0))
            downloaded = 0

            with open(update_file, 'wb') as f:
                for chunk in r.iter_content(chunk_size=8192):
                    f.write(chunk)
                    downloaded += len(chunk)

                    # Report progress
                    if total_size > 0:
                        progress = int((downloaded / total_size) * 50)
                        self._report_update_status(update_id, 'downloading', progress)

        return update_file

    def _verify_checksum(self, file_path: str, expected_checksum: str) -> bool:
        """Verify file checksum"""
        sha256_hash = hashlib.sha256()
        with open(file_path, "rb") as f:
            for byte_block in iter(lambda: f.read(4096), b""):
                sha256_hash.update(byte_block)

        actual_checksum = sha256_hash.hexdigest()
        return actual_checksum == expected_checksum

    def _install_update(self, update_file: str):
        """Install update (implementation depends on update type)"""
        # This is a placeholder - actual implementation depends on your update mechanism
        logger.info(f"Installing update from {update_file}")
        # For example, could extract and run an installer script
        pass

    def _report_update_status(self, update_id: str, status: str,
                             progress: int = None, error: str = None):
        """Report update status"""
        try:
            status_data = {'status': status}
            if progress is not None:
                status_data['progress'] = progress
            if error:
                status_data['error'] = error

            response = self.session.post(
                f"{self.config.server_url}/api/v1/device/updates/{update_id}/status",
                json=status_data,
                timeout=10
            )
            response.raise_for_status()

        except Exception as e:
            logger.error(f"Failed to report update status: {e}")

    def _collect_and_upload_logs(self, command_id: str, payload: Dict[str, Any]):
        """Collect and upload logs"""
        try:
            log_type = payload.get('log_type', 'system')
            since = payload.get('since', '1h')

            # Collect logs based on type
            if log_type == 'system':
                logs = subprocess.check_output(
                    ['journalctl', '--since', since, '--no-pager'],
                    universal_newlines=True
                )
            else:
                logs = "Log collection not implemented for type: " + log_type

            # Upload logs
            response = self.session.post(
                f"{self.config.server_url}/api/v1/device/logs",
                json={
                    'logs': [
                        {
                            'timestamp': datetime.utcnow().isoformat() + 'Z',
                            'level': 'info',
                            'message': logs
                        }
                    ]
                },
                timeout=30
            )
            response.raise_for_status()

            self._acknowledge_command(command_id, 'completed')

        except Exception as e:
            logger.error(f"Failed to collect/upload logs: {e}")
            self._acknowledge_command(command_id, 'failed', error=str(e))

    def _heartbeat_loop(self):
        """Heartbeat loop thread"""
        while self.running:
            try:
                self.send_heartbeat()
            except:
                pass
            time.sleep(self.config.heartbeat_interval)

    def _telemetry_loop(self):
        """Telemetry loop thread"""
        while self.running:
            try:
                self.send_telemetry()
            except:
                pass
            time.sleep(self.config.telemetry_interval)

    def _command_loop(self):
        """Command polling loop thread"""
        while self.running:
            try:
                # Check for poll signal or timeout
                try:
                    self.command_queue.get(timeout=self.config.command_poll_interval)
                except Empty:
                    pass

                self.poll_commands()
            except:
                pass

    def start(self):
        """Start the device agent"""
        logger.info("Starting fleetd Device Agent...")

        # Try to load saved credentials
        if not self._load_credentials():
            # Need to enroll
            if not self.enroll():
                logger.error("Failed to enroll device")
                return False

        # Update system info
        try:
            response = self.session.put(
                f"{self.config.server_url}/api/v1/device/system",
                json=self._get_system_info(),
                timeout=10
            )
            response.raise_for_status()
        except Exception as e:
            logger.warning(f"Failed to update system info: {e}")

        self.running = True

        # Start background threads
        threads = [
            threading.Thread(target=self._heartbeat_loop, daemon=True),
            threading.Thread(target=self._telemetry_loop, daemon=True),
            threading.Thread(target=self._command_loop, daemon=True)
        ]

        for thread in threads:
            thread.start()

        logger.info(f"Device agent started: {self.device_id}")

        try:
            # Keep main thread alive
            while self.running:
                time.sleep(1)
        except KeyboardInterrupt:
            logger.info("Shutting down...")
            self.stop()

    def stop(self):
        """Stop the device agent"""
        self.running = False
        logger.info("Device agent stopped")


def main():
    """Main entry point"""
    # Load configuration from environment or file
    config = AgentConfig(
        server_url=os.getenv('FLEET_SERVER_URL', 'https://devices.fleet.yourdomain.com'),
        enrollment_token=os.getenv('FLEET_ENROLLMENT_TOKEN'),
        heartbeat_interval=int(os.getenv('FLEET_HEARTBEAT_INTERVAL', '60')),
        telemetry_interval=int(os.getenv('FLEET_TELEMETRY_INTERVAL', '30')),
        log_level=os.getenv('FLEET_LOG_LEVEL', 'INFO')
    )

    # Set log level
    logger.setLevel(getattr(logging, config.log_level.upper()))

    # Create and start agent
    agent = FleetDeviceAgent(config)
    agent.start()


if __name__ == '__main__':
    main()