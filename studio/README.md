# fleetd Web Dashboard

Modern, real-time web dashboard for fleetd management built with Next.js 15.

## Tech Stack

- **Next.js 15** - React framework with App Router and RSC
- **React 19 RC** - Latest React with concurrent features
- **TypeScript** - Type-safe development
- **Tailwind CSS** - Utility-first CSS framework
- **Radix UI** - Unstyled, accessible components
- **Framer Motion** - Smooth animations
- **React Query** - Server state management
- **Recharts** - Data visualization
- **Bun** - Fast JavaScript runtime and package manager
- **Biome** - Fast formatter and linter

## Features

- Real-time device monitoring with Server-Sent Events (SSE)
- Responsive dashboard with device stats
- Live telemetry charts
- Device discovery and management
- Dark mode support
- Optimistic updates with React Query
- Partial Pre-rendering (PPR) enabled
- **Interactive Provisioning Guide** - Step-by-step wizard for device setup
- **Quick Provisioning Commands** - Copy-paste ready commands for common setups

## Getting Started

### Prerequisites

- Bun installed (`curl -fsSL https://bun.sh/install | bash`)
- fleetd server running on port 8080

### Installation

```bash
cd studio
bun install
```

### Development

```bash
bun dev
```

Open [http://localhost:3000](http://localhost:3000) to view the dashboard.

### Production Build

```bash
bun run build
bun start
```

## Project Structure

```
web/
├── app/                 # Next.js App Router
│   ├── layout.tsx      # Root layout with providers
│   ├── page.tsx        # Dashboard page (RSC)
│   └── globals.css     # Global styles
├── components/         # React components
│   ├── ui/            # Reusable UI components
│   ├── dashboard-*    # Dashboard specific components
│   └── providers.tsx  # Context providers
├── lib/               # Utilities and API client
│   ├── api.ts        # API client
│   ├── sse.ts        # SSE utilities
│   ├── types.ts      # TypeScript types
│   └── utils.ts      # Helper functions
├── hooks/            # Custom React hooks
└── public/           # Static assets
```

## Environment Variables

Create a `.env.local` file:

```env
NEXT_PUBLIC_API_URL=http://localhost:8080
BACKEND_URL=http://localhost:8080
```

## Scripts

- `bun dev` - Start development server with Turbopack
- `bun build` - Build for production
- `bun start` - Start production server
- `bun lint` - Run Biome linter
- `bun format` - Format code with Biome
- `bun typecheck` - Type check with TypeScript

## Device Provisioning Guide

The web dashboard includes an interactive provisioning guide that helps users set up new devices:

### Interactive Wizard
Click the "Add Device" button to open the provisioning wizard:

1. **Select Device Type**: Raspberry Pi, x86/x64, NVIDIA Jetson, or custom
2. **Choose Setup Type**: Standalone, k3s server/worker, or Docker host
3. **Configure Settings**: Network, security, and fleet server connection
4. **Generate Command**: Get a customized `fleet provision` command

### Quick Provisioning
The dashboard also provides quick copy-paste commands for common scenarios:

- **Raspberry Pi k3s Cluster**: Pre-configured for k3s server and worker nodes
- **NVIDIA Jetson AI Nodes**: Docker + NVIDIA runtime setup
- **x86 Edge Servers**: Ubuntu Server or Docker host configurations

Each command is customized with:
- Your Fleet server URL
- Device-specific settings
- Appropriate plugins and configurations

### Example Commands Generated

**k3s Server on Raspberry Pi:**
```bash
fleet provision \
  --device /dev/disk2 \
  --name "k3s-server-01" \
  --wifi-ssid "YOUR_WIFI" \
  --wifi-pass "YOUR_PASSWORD" \
  --plugin k3s \
  --plugin-opt k3s.role=server \
  --fleet-server http://your-server:8080
```

## API Integration

The dashboard connects to the fleetd backend API:

- `/api/v1/devices` - Device management
- `/api/v1/telemetry` - Telemetry data
- `/api/v1/discover` - Device discovery
- `/api/v1/events` - SSE for real-time updates

## Real-time Updates

The dashboard uses Server-Sent Events (SSE) for real-time updates:

- Device connection/disconnection notifications
- Live telemetry updates
- Update availability alerts
- Automatic data refresh on events

## Deployment

The web dashboard can be deployed as a standalone Next.js app or integrated with the Go backend for a single binary deployment.
