"use client";
import { CheckCircledIcon, RocketIcon } from "@radix-ui/react-icons";
import { AnimatePresence, motion } from "framer-motion";
import { CopyIcon } from "lucide-react";
import { useId, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Separator } from "@/components/ui/separator";
import { Snippet, SnippetContent, SnippetCopyButton, SnippetHeader } from "@/components/ui/snippet";
import { Switch } from "@/components/ui/switch";

interface ProvisionConfig {
  deviceType: "raspberrypi" | "x86" | "jetson" | "custom";
  setupType: "k3s-server" | "k3s-worker" | "standalone" | "docker-host";
  devicePath: string;
  deviceName: string;
  wifiSSID?: string;
  wifiPassword?: string;
  enableSSH: boolean;
  k3sServerUrl?: string;
  k3sToken?: string;
  imageUrl?: string;
  autoConnect: boolean;
}
interface ProvisioningGuideProps {
  onClose?: () => void;
}

interface StepProps {
  config: ProvisionConfig;
  updateConfig: (key: keyof ProvisionConfig, value: string | boolean | undefined) => void;
  devicePathId?: string;
  deviceNameId?: string;
  wifiSsidId?: string;
  wifiPasswordId?: string;
  enableSshId?: string;
  autoConnectId?: string;
  k3sServerId?: string;
  k3sTokenId?: string;
  imageUrlId?: string;
  generateCommand?: () => string;
  copied?: boolean;
  setCopied?: (value: boolean) => void;
}

const Step1DeviceType = ({ config, updateConfig }: StepProps) => (
  <div className="space-y-4">
    <div>
      <h3 className="text-lg font-semibold mb-2">Select Device Type</h3>
      <p className="text-sm text-muted-foreground">
        Choose the type of device you want to provision
      </p>
    </div>
    <RadioGroup
      value={config.deviceType}
      onValueChange={(value) => updateConfig("deviceType", value)}
    >
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <button
          type="button"
          className="flex items-start space-x-3 p-4 border rounded-lg cursor-pointer hover:bg-accent w-full text-left"
          onClick={() => updateConfig("deviceType", "raspberrypi")}
        >
          <RadioGroupItem value="raspberrypi" className="mt-1" />
          <div className="space-y-1">
            <p className="font-medium">Raspberry Pi</p>
            <p className="text-sm text-muted-foreground">
              Models 3B, 4B, Zero W, etc. ARM-based single board computer
            </p>
          </div>
        </button>
        <button
          type="button"
          className="flex items-start space-x-3 p-4 border rounded-lg cursor-pointer hover:bg-accent w-full text-left"
          onClick={() => updateConfig("deviceType", "x86")}
        >
          <RadioGroupItem value="x86" className="mt-1" />
          <div className="space-y-1">
            <p className="font-medium">x86/x64</p>
            <p className="text-sm text-muted-foreground">
              Intel NUC, Dell Edge, standard PC hardware
            </p>
          </div>
        </button>
        <button
          type="button"
          className="flex items-start space-x-3 p-4 border rounded-lg cursor-pointer hover:bg-accent w-full text-left"
          onClick={() => updateConfig("deviceType", "jetson")}
        >
          <RadioGroupItem value="jetson" className="mt-1" />
          <div className="space-y-1">
            <p className="font-medium">NVIDIA Jetson</p>
            <p className="text-sm text-muted-foreground">
              Nano, Xavier NX, AGX Orin AI compute devices
            </p>
          </div>
        </button>
        <button
          type="button"
          className="flex items-start space-x-3 p-4 border rounded-lg cursor-pointer hover:bg-accent w-full text-left"
          onClick={() => updateConfig("deviceType", "custom")}
        >
          <RadioGroupItem value="custom" className="mt-1" />
          <div className="space-y-1">
            <p className="font-medium">Bring your own</p>
            <p className="text-sm text-muted-foreground">Provide your own OS image</p>
          </div>
        </button>
      </div>
    </RadioGroup>
  </div>
);

const Step2SetupType = ({ config, updateConfig }: StepProps) => (
  <div className="space-y-4">
    <div>
      <h3 className="text-lg font-semibold mb-2">Setup Type</h3>
      <p className="text-sm text-muted-foreground">How should this device be configured?</p>
    </div>
    <RadioGroup
      value={config.setupType}
      onValueChange={(value) => updateConfig("setupType", value)}
    >
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <button
          type="button"
          className="flex items-start space-x-3 p-4 border rounded-lg cursor-pointer hover:bg-accent w-full text-left"
          onClick={() => updateConfig("setupType", "standalone")}
        >
          <RadioGroupItem value="standalone" className="mt-1" />
          <div className="space-y-1">
            <p className="font-medium">Standalone Device</p>
            <p className="text-sm text-muted-foreground">
              Simple telemetry agent for monitoring and control
            </p>
          </div>
        </button>
        <button
          type="button"
          className="flex items-start space-x-3 p-4 border rounded-lg cursor-pointer hover:bg-accent w-full text-left"
          onClick={() => updateConfig("setupType", "docker-host")}
        >
          <RadioGroupItem value="docker-host" className="mt-1" />
          <div className="space-y-1">
            <p className="font-medium">Docker Host</p>
            <p className="text-sm text-muted-foreground">
              Container runtime for deploying applications
            </p>
          </div>
        </button>
        <button
          type="button"
          className="flex items-start space-x-3 p-4 border rounded-lg cursor-pointer hover:bg-accent w-full text-left"
          onClick={() => updateConfig("setupType", "k3s-server")}
        >
          <RadioGroupItem value="k3s-server" className="mt-1" />
          <div className="space-y-1">
            <p className="font-medium">k3s Server</p>
            <p className="text-sm text-muted-foreground">
              Kubernetes control plane for edge orchestration
            </p>
          </div>
        </button>
        <button
          type="button"
          className="flex items-start space-x-3 p-4 border rounded-lg cursor-pointer hover:bg-accent w-full text-left"
          onClick={() => updateConfig("setupType", "k3s-worker")}
        >
          <RadioGroupItem value="k3s-worker" className="mt-1" />
          <div className="space-y-1">
            <p className="font-medium">k3s Worker</p>
            <p className="text-sm text-muted-foreground">
              Join existing k3s cluster as a worker node
            </p>
          </div>
        </button>
      </div>
    </RadioGroup>
  </div>
);

const Step3Configuration = ({
  config,
  updateConfig,
  devicePathId,
  deviceNameId,
  wifiSsidId,
  wifiPasswordId,
  enableSshId,
  autoConnectId,
  k3sServerId,
  k3sTokenId,
  imageUrlId,
}: StepProps) => (
  <div className="space-y-4">
    <div>
      <h3 className="text-lg font-semibold mb-2">Device Configuration</h3>
      <p className="text-sm text-muted-foreground">Configure device-specific settings</p>
    </div>
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div className="space-y-2">
        <Label htmlFor={devicePathId}>Device Path</Label>
        <Input
          id={devicePathId}
          value={config.devicePath}
          onChange={(e) => updateConfig("devicePath", e.target.value)}
          placeholder="/dev/disk2 or /dev/sdb"
        />
        <p className="text-xs text-muted-foreground">
          Examples: /dev/disk2 (macOS), /dev/sdb (Linux), /dev/mmcblk0 (Raspberry Pi)
        </p>
      </div>
      <div className="space-y-2">
        <Label htmlFor={deviceNameId}>Device Name</Label>
        <Input
          id={deviceNameId}
          value={config.deviceName}
          onChange={(e) => updateConfig("deviceName", e.target.value)}
          placeholder="fleet-device-01"
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={wifiSsidId}>WiFi SSID (Optional)</Label>
        <Input
          id={wifiSsidId}
          value={config.wifiSSID || ""}
          onChange={(e) => updateConfig("wifiSSID", e.target.value)}
          placeholder="Your-WiFi-Network"
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={wifiPasswordId}>WiFi Password (Optional)</Label>
        <Input
          id={wifiPasswordId}
          type="password"
          value={config.wifiPassword || ""}
          onChange={(e) => updateConfig("wifiPassword", e.target.value)}
          placeholder="••••••••"
        />
      </div>
      <div className="flex items-center justify-between p-4 border rounded-lg">
        <Label htmlFor={enableSshId}>Enable SSH Access</Label>
        <Switch
          id={enableSshId}
          checked={config.enableSSH}
          onCheckedChange={(checked) => updateConfig("enableSSH", checked)}
        />
      </div>
      <div className="flex items-center justify-between p-4 border rounded-lg">
        <Label htmlFor={autoConnectId}>Auto-Connect to Fleet</Label>
        <Switch
          id={autoConnectId}
          checked={config.autoConnect}
          onCheckedChange={(checked) => updateConfig("autoConnect", checked)}
        />
      </div>
      {config.setupType === "k3s-worker" && (
        <>
          <div className="col-span-2 space-y-2">
            <Label htmlFor={k3sServerId}>k3s Server URL</Label>
            <Input
              id={k3sServerId}
              value={config.k3sServerUrl}
              onChange={(e) => updateConfig("k3sServerUrl", e.target.value)}
              placeholder="https://192.168.1.100:6443"
            />
            <p className="text-xs text-muted-foreground">The URL of your existing k3s server</p>
          </div>
          <div className="col-span-2 space-y-2">
            <Label htmlFor={k3sTokenId}>k3s Join Token</Label>
            <Input
              id={k3sTokenId}
              type="password"
              value={config.k3sToken}
              onChange={(e) => updateConfig("k3sToken", e.target.value)}
              placeholder="K10abc..."
            />
            <p className="text-xs text-muted-foreground">
              Get from server: `sudo cat /var/lib/rancher/k3s/server/node-token`
            </p>
          </div>
        </>
      )}
      {config.deviceType === "custom" && (
        <div className="col-span-2 space-y-2">
          <Label htmlFor={imageUrlId}>Custom Image URL</Label>
          <Input
            id={imageUrlId}
            value={config.imageUrl}
            onChange={(e) => updateConfig("imageUrl", e.target.value)}
            placeholder="https://example.com/custom-os.img.xz"
          />
        </div>
      )}
    </div>
  </div>
);

const Step4Review = ({ config, generateCommand, copied, setCopied }: StepProps) => {
  const command = generateCommand ? generateCommand() : "";
  return (
    <div className="space-y-4 overflow-hidden">
      <div>
        <h3 className="text-lg font-semibold mb-2">Review & Execute</h3>
        <p className="text-sm text-muted-foreground mb-4">
          Copy and run this command in your terminal
        </p>
      </div>

      <div className="space-y-4">
        <div>
          <h4 className="font-medium mb-2">Generated Command</h4>
          <p className="text-sm text-muted-foreground mb-3">
            Run this command on your local machine with the fleetd CLI installed
          </p>
          <div className="w-full overflow-x-auto">
            <Snippet defaultValue="command" className="max-w-full">
              <SnippetHeader language="bash">
                <SnippetCopyButton value={command} />
              </SnippetHeader>
              <SnippetContent language="bash" className="whitespace-pre-wrap break-all">
                {command}
              </SnippetContent>
            </Snippet>
          </div>
        </div>

        <Separator />

        <div>
          <h4 className="font-medium mb-4">Next Steps</h4>
          <div className="space-y-3">
            <div className="flex items-start space-x-2">
              <CheckCircledIcon className="h-5 w-5 text-green-500 mt-0.5 flex-shrink-0" />
              <div className="min-w-0">
                <p className="font-medium">1. Install fleetd CLI</p>
                <div className="text-sm text-muted-foreground break-words space-y-2 mt-1">
                  <button
                    type="button"
                    className="group flex items-center gap-2 bg-muted/50 px-3 py-2 rounded cursor-pointer hover:bg-muted/70 transition-colors"
                    onClick={() => {
                      navigator.clipboard.writeText("curl -sSL https://get.fleetd.sh | sh");
                      setCopied?.(true);
                      setTimeout(() => setCopied?.(false), 2000);
                    }}
                    title="Click to copy"
                  >
                    <code className="flex-1 text-xs font-mono">
                      curl -sSL https://get.fleetd.sh | sh
                    </code>
                    {copied ? (
                      <CheckCircledIcon className="h-3 w-3 text-green-500 flex-shrink-0" />
                    ) : (
                      <CopyIcon className="h-3 w-3 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0" />
                    )}
                  </button>
                  <p className="text-xs">
                    Or download directly from{" "}
                    <a
                      href="https://fleetd.sh/downloads"
                      target="_blank"
                      rel="noopener noreferrer"
                      className="underline hover:text-foreground transition-colors"
                    >
                      https://fleetd.sh/downloads
                    </a>
                  </p>
                </div>
              </div>
            </div>
            <div className="flex items-start space-x-2">
              <CheckCircledIcon className="h-5 w-5 text-green-500 mt-0.5 flex-shrink-0" />
              <div className="min-w-0">
                <p className="font-medium">2. Insert SD Card / USB Drive</p>
                <p className="text-sm text-muted-foreground break-words">
                  Make sure the device path matches: {config.devicePath}
                </p>
              </div>
            </div>
            <div className="flex items-start space-x-2">
              <CheckCircledIcon className="h-5 w-5 text-green-500 mt-0.5 flex-shrink-0" />
              <div className="min-w-0">
                <p className="font-medium">3. Run the Command</p>
                <p className="text-sm text-muted-foreground break-words">
                  This will download the OS image and write it to your device
                </p>
              </div>
            </div>
            <div className="flex items-start space-x-2">
              <CheckCircledIcon className="h-5 w-5 text-green-500 mt-0.5 flex-shrink-0" />
              <div className="min-w-0">
                <p className="font-medium">4. Boot the Device</p>
                <p className="text-sm text-muted-foreground break-words">
                  Remove the media, insert into target device, and power on
                </p>
              </div>
            </div>
            <div className="flex items-start space-x-2">
              <CheckCircledIcon className="h-5 w-5 text-green-500 mt-0.5 flex-shrink-0" />
              <div className="min-w-0">
                <p className="font-medium">5. Check Dashboard</p>
                <p className="text-sm text-muted-foreground break-words">
                  Return here to see your device appear with real-time telemetry
                </p>
              </div>
            </div>
          </div>
        </div>

        {config.setupType === "k3s-server" && (
          <>
            <Separator />
            <div className="bg-blue-50 dark:bg-blue-950/20 rounded-lg p-4">
              <h4 className="font-medium flex items-center mb-2">
                <RocketIcon className="mr-2 h-4 w-4" />
                k3s Server Token
              </h4>
              <p className="text-sm mb-2">
                After the server boots, get the join token for worker nodes:
              </p>
              <pre className="bg-background p-2 rounded text-xs overflow-x-auto whitespace-pre-wrap break-all">
                ssh {config.deviceName}.local sudo cat /var/lib/rancher/k3s/server/node-token
              </pre>
            </div>
          </>
        )}
      </div>
    </div>
  );
};

export function ProvisioningGuide({ onClose }: ProvisioningGuideProps = {}) {
  const [currentStep, setCurrentStep] = useState(1);
  const [copied, setCopied] = useState(false);

  // Generate unique IDs for form elements
  const devicePathId = useId();
  const deviceNameId = useId();
  const wifiSsidId = useId();
  const wifiPasswordId = useId();
  const enableSshId = useId();
  const autoConnectId = useId();
  const k3sServerId = useId();
  const k3sTokenId = useId();
  const imageUrlId = useId();

  const [config, setConfig] = useState<ProvisionConfig>({
    deviceType: "raspberrypi",
    setupType: "standalone",
    devicePath: "/dev/disk2",
    deviceName: "fleet-device",
    enableSSH: true,
    autoConnect: true,
  });
  const updateConfig = (key: keyof ProvisionConfig, value: string | boolean | undefined) => {
    setConfig((prev) => ({ ...prev, [key]: value }));
  };
  const generateCommand = () => {
    const args = [`fleetctl provision --device ${config.devicePath}`];
    args.push(`--type ${config.deviceType}`);
    args.push(`--setup ${config.setupType}`);
    args.push(`--name "${config.deviceName}"`);
    if (config.enableSSH) args.push("--enable-ssh");
    if (config.wifiSSID) {
      args.push(`--wifi-ssid "${config.wifiSSID}"`);
      if (config.wifiPassword) {
        args.push(`--wifi-password "${config.wifiPassword}"`);
      }
    }
    if (config.setupType === "k3s-worker" && config.k3sServerUrl) {
      args.push(`--k3s-server "${config.k3sServerUrl}"`);
      if (config.k3sToken) {
        args.push(`--k3s-token "${config.k3sToken}"`);
      }
    }
    if (config.deviceType === "custom" && config.imageUrl) {
      args.push(`--image-url "${config.imageUrl}"`);
    }
    if (config.autoConnect) args.push("--auto-connect");
    return args.join(" \\\n  ");
  };

  const stepProps: StepProps = {
    config,
    updateConfig,
    devicePathId,
    deviceNameId,
    wifiSsidId,
    wifiPasswordId,
    enableSshId,
    autoConnectId,
    k3sServerId,
    k3sTokenId,
    imageUrlId,
    generateCommand,
    copied,
    setCopied,
  };

  const renderStep = () => {
    switch (currentStep) {
      case 1:
        return <Step1DeviceType {...stepProps} />;
      case 2:
        return <Step2SetupType {...stepProps} />;
      case 3:
        return <Step3Configuration {...stepProps} />;
      case 4:
        return <Step4Review {...stepProps} />;
      default:
        return null;
    }
  };

  const steps = [
    { number: 1, title: "Device Type" },
    { number: 2, title: "Setup Type" },
    { number: 3, title: "Configuration" },
    { number: 4, title: "Review" },
  ];

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 max-w-4xl mx-auto w-full space-y-6 px-4 sm:px-6 lg:px-8 pb-6">
        <div>
          <h2 className="text-2xl font-bold">Device Provisioning Guide</h2>
          <p className="text-muted-foreground mt-2">
            Follow this guide to provision new devices with fleetd
          </p>
        </div>
        {/* Step indicator */}
        <div className="flex items-center justify-between mb-8">
          {steps.map((step, index) => (
            <div key={step.number} className="flex items-center">
              <Button
                onClick={() => setCurrentStep(step.number)}
                className={`
                flex items-center justify-center w-10 h-10 rounded-full font-medium
                ${
                  currentStep >= step.number
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted text-muted-foreground"
                }
                ${currentStep === step.number ? "ring-2 ring-primary ring-offset-2" : ""}
                transition-all cursor-pointer hover:scale-105
              `}
              >
                {step.number}
              </Button>
              {index < steps.length - 1 && (
                <div
                  className={`
                w-full h-1 mx-2
                ${currentStep > step.number ? "bg-primary" : "bg-muted"}
                transition-colors
              `}
                />
              )}
            </div>
          ))}
        </div>
        {/* Step content */}
        <div className="p-6 border rounded-lg">
          <AnimatePresence mode="wait">
            <motion.div
              key={currentStep}
              initial={{ opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -20 }}
              transition={{ duration: 0.2 }}
            >
              {renderStep()}
            </motion.div>
          </AnimatePresence>
        </div>
      </div>
      {/* Footer */}
      <div className="border-t px-4 sm:px-6 lg:px-8 py-4">
        <div className="flex justify-between max-w-4xl mx-auto">
          <Button
            variant="outline"
            onClick={() => setCurrentStep(Math.max(1, currentStep - 1))}
            disabled={currentStep === 1}
          >
            Previous
          </Button>
          {currentStep === steps.length ? (
            <Button onClick={onClose}>Done</Button>
          ) : (
            <Button onClick={() => setCurrentStep(Math.min(steps.length, currentStep + 1))}>
              Next
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
