# Context/architecture

## Core Design Principles

- OS-agnostic, bare metal first

- Local-first operation

- Minimal dependencies

- Progressive enhancement based on available resources

- Secure by default

- Resource-efficient core

## System Components

### Device Agent (SOFTWARE)

#### Core Features (P0)

- Binary deployment & lifecycle management

- Basic telemetry collection

- Offline operation capability

- Minimal state management

- Self-update mechanism

- mDNS discovery support

#### Progressive Features (P1)

- Local storage/caching when available

- Extended metrics collection

- Container support where available

### Server (SERVERAPP)

#### Core Features (P0)

- Device registration & authentication

- Binary/package distribution

- Basic telemetry ingestion

- Fleet-wide update management

- Storage service for binaries

#### Progressive Features (P1)

- Advanced analytics

- Custom metrics storage backends

- Webhook integrations

## Initial Implementation Plan

### Phase 1 (Core Focus)

1. Basic device agent

   - Runtime management

   - Update mechanism

   - Binary deployment

   - mDNS support

2. Minimal server

   - Device registry

   - Binary storage

   - Basic API

### Phase 2 (Enhancement)

1. Telemetry pipeline

2. Offline capabilities

3. Security hardening

4. SDK for vendor integration

## Technology Decisions

### Device Agent

- Golang for core runtime

- Minimal external dependencies

- Optional SQLite for local storage

### Server

- Connect RPCs with HTTP/REST proxies for communication

- SQLite for core data

- Single binary deployment

## Use cases

I'll line out some use cases to help guide the design of the system.

### Use case 1: Basic hobbyist deployment

I myself am a hobbyist that like to fiddle with both hardware and software. I have a few Raspberry Pis that I use to run a few services at home. I'd like to be able to remotely manage the devices that I own; update the software I run on them, check on their status, etc. without having to manually SSH into each device.

### Use case 2: Consumer hardware

Let's use a norwegian power company called Tibber as an example. Tibber is a smart energy company that offers a smart energy monitor called Pulse. The device itself is a tiny box that you can plug into the HAN-port of your electricity meter, which in Norway, all are equipped with. It has WiFi and Bluetooth LE, and it monitors the electricity usage in your home. They offer integrations with e.g. EV chargers so that it automatically starts load balancing when you're charging your electric car.

When you buy a Pulse device, you need to set it up first. You do this by downloading the Tibber app on your phone, scan either a QR code or the nearby area using BLE. The app then guides you through the setup process, during which it also registers the device with Tibber's backend server and connects it to your account.

Once the device is set up, you can manage the device through the Tibber app, e.g. view the current electricity usage, set load balancing rules for your EV charger, etc.

When any updates to the Pulse device are available, you'll receive a notification in the Tibber app, and you can install the update directly through the app.

### Use case 3: Industrial IoT

In an industrial environment, you often have thousands of devices that needs to be managed. These devices often have harsh operating conditions, high physical security requirements, and stringent compliance needs.

Most of these devices run without any OS, or a minimal RTOS. They often have tight restrictions on what software can be installed, and where. Downtimes are extremely costly, and the devices are often in hard to reach locations. Therefore, it is crucial that any updates can be installed without any user intervention and that measures like automatic rollback, self-healing and remote control are built-in to the software.
