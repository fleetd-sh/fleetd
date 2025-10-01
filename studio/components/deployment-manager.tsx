"use client";
import {
  AlertCircleIcon,
  CheckCircleIcon,
  DownloadIcon,
  HistoryIcon,
  LayersIcon,
  PackageIcon,
  PauseIcon,
  RefreshCwIcon,
  RocketIcon,
  TrendingUpIcon,
  UploadIcon,
  XCircleIcon,
} from "lucide-react";
import { useId, useState } from "react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

interface Device {
  id: string;
  name: string;
  [key: string]: unknown;
}
interface DeploymentManagerProps {
  applicationId?: string;
  devices?: Device[];
}
export function DeploymentManager({ devices = [] }: DeploymentManagerProps) {
  // Generate unique IDs for radio buttons
  const canaryId = useId();
  const rollingId = useId();
  const blueGreenId = useId();
  const allAtOnceId = useId();
  const targetAllId = useId();
  const targetGroupId = useId();
  const targetTagsId = useId();
  const targetCustomId = useId();

  const [selectedStrategy, setSelectedStrategy] = useState("canary");
  const [targetGroup, setTargetGroup] = useState("all");
  const [canaryConfig, setCanaryConfig] = useState({
    initialPercentage: 10,
    incrementPercentage: 20,
    incrementInterval: 300, // seconds
    successThreshold: 0.95,
  });
  const [rollbackEnabled, setRollbackEnabled] = useState(true);
  interface Deployment {
    id: string;
    application: string;
    version: string;
    previousVersion: string;
    status: string;
    strategy: string;
    progress: number;
    targetDevices: number;
    currentDevices: number;
    successRate: number;
    startedAt: string;
    estimatedCompletion: string;
    phases: Array<{
      name: string;
      status: string;
      percentage: number;
      devices: number;
      successRate: number;
    }>;
  }
  const [currentDeployment, setCurrentDeployment] = useState<Deployment | null>(null);
  // Mock deployment status
  const mockDeployment: Deployment = {
    id: "dep-123",
    application: "edge-inference",
    version: "2.1.0",
    previousVersion: "2.0.0",
    status: "in_progress",
    strategy: "canary",
    progress: 35,
    targetDevices: 150,
    currentDevices: 53,
    successRate: 0.96,
    startedAt: new Date(Date.now() - 1200000).toISOString(), // 20 minutes ago
    estimatedCompletion: new Date(Date.now() + 2400000).toISOString(), // 40 minutes from now
    phases: [
      {
        name: "Phase 1 (10%)",
        status: "completed",
        percentage: 10,
        devices: 15,
        successRate: 1.0,
      },
      {
        name: "Phase 2 (30%)",
        status: "completed",
        percentage: 30,
        devices: 30,
        successRate: 0.97,
      },
      {
        name: "Phase 3 (50%)",
        status: "in_progress",
        percentage: 50,
        devices: 30,
        successRate: 0.8,
      },
      {
        name: "Phase 4 (100%)",
        status: "pending",
        percentage: 100,
        devices: 75,
        successRate: 0,
      },
    ],
  };
  const startDeployment = () => {
    setCurrentDeployment(mockDeployment);
  };
  const rollbackDeployment = () => {
    // Implement rollback logic
    console.log("Rolling back deployment");
  };
  const pauseDeployment = () => {
    // Implement pause logic
    console.log("Pausing deployment");
  };
  return (
    <div className="space-y-6">
      {/* Current Deployment Status */}
      {currentDeployment && (
        <Alert
          className={
            Math.floor(currentDeployment.currentDevices * (1 - currentDeployment.successRate)) > 5
              ? "border-red-500"
              : currentDeployment.status === "completed"
                ? "border-green-500"
                : "border-blue-500"
          }
        >
          <AlertCircleIcon className="h-4 w-4" />
          <AlertDescription className="flex items-center justify-between">
            <div>
              <strong>Active Deployment:</strong> {currentDeployment.application} v
              {currentDeployment.version}
              <span className="ml-4 text-sm text-muted-foreground">
                Started {new Date(currentDeployment.startedAt).toLocaleTimeString()}
              </span>
            </div>
            <div className="flex gap-2">
              <Button size="sm" variant="outline" onClick={pauseDeployment}>
                <PauseIcon className="h-4 w-4 mr-1" />
                Pause
              </Button>
              <Button size="sm" variant="destructive" onClick={rollbackDeployment}>
                <HistoryIcon className="h-4 w-4 mr-1" />
                Rollback
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      )}
      <Tabs defaultValue="deploy" className="space-y-4">
        <TabsList>
          <TabsTrigger value="deploy">New Deployment</TabsTrigger>
          <TabsTrigger value="active">Active Deployments</TabsTrigger>
          <TabsTrigger value="history">History</TabsTrigger>
          <TabsTrigger value="artifacts">Artifacts</TabsTrigger>
        </TabsList>
        {/* New Deployment Tab */}
        <TabsContent value="deploy" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Deploy Application</CardTitle>
              <CardDescription>Configure and start a new deployment</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Application & Version Selection */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <Label>Application</Label>
                  <Select defaultValue="edge-inference">
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="edge-inference">Edge Inference Engine</SelectItem>
                      <SelectItem value="data-collector">Data Collector</SelectItem>
                      <SelectItem value="sync-agent">Sync Agent</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <Label>Version</Label>
                  <Select defaultValue="2.1.0">
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="2.1.0">2.1.0 (latest)</SelectItem>
                      <SelectItem value="2.0.0">2.0.0</SelectItem>
                      <SelectItem value="1.9.5">1.9.5</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              {/* Deployment Strategy */}
              <div>
                <Label>Deployment Strategy</Label>
                <RadioGroup value={selectedStrategy} onValueChange={setSelectedStrategy}>
                  <div className="grid grid-cols-2 gap-4 mt-2">
                    <Card className={selectedStrategy === "canary" ? "border-primary" : ""}>
                      <CardHeader className="p-4">
                        <div className="flex items-center space-x-2">
                          <RadioGroupItem value="canary" id={canaryId} />
                          <Label
                            htmlFor={canaryId}
                            className="flex items-center gap-2 cursor-pointer"
                          >
                            <TrendingUpIcon className="h-4 w-4" />
                            Canary Deployment
                          </Label>
                        </div>
                        <CardDescription className="text-xs mt-2">
                          Gradually roll out to a percentage of devices
                        </CardDescription>
                      </CardHeader>
                    </Card>
                    <Card className={selectedStrategy === "rolling" ? "border-primary" : ""}>
                      <CardHeader className="p-4">
                        <div className="flex items-center space-x-2">
                          <RadioGroupItem value="rolling" id={rollingId} />
                          <Label
                            htmlFor={rollingId}
                            className="flex items-center gap-2 cursor-pointer"
                          >
                            <RefreshCwIcon className="h-4 w-4" />
                            Rolling Update
                          </Label>
                        </div>
                        <CardDescription className="text-xs mt-2">
                          Update devices in batches with zero downtime
                        </CardDescription>
                      </CardHeader>
                    </Card>
                    <Card className={selectedStrategy === "blue-green" ? "border-primary" : ""}>
                      <CardHeader className="p-4">
                        <div className="flex items-center space-x-2">
                          <RadioGroupItem value="blue-green" id={blueGreenId} />
                          <Label
                            htmlFor={blueGreenId}
                            className="flex items-center gap-2 cursor-pointer"
                          >
                            <LayersIcon className="h-4 w-4" />
                            Blue-Green
                          </Label>
                        </div>
                        <CardDescription className="text-xs mt-2">
                          Switch between two identical environments
                        </CardDescription>
                      </CardHeader>
                    </Card>
                    <Card className={selectedStrategy === "all-at-once" ? "border-primary" : ""}>
                      <CardHeader className="p-4">
                        <div className="flex items-center space-x-2">
                          <RadioGroupItem value="all-at-once" id={allAtOnceId} />
                          <Label
                            htmlFor={allAtOnceId}
                            className="flex items-center gap-2 cursor-pointer"
                          >
                            <RocketIcon className="h-4 w-4" />
                            All at Once
                          </Label>
                        </div>
                        <CardDescription className="text-xs mt-2">
                          Deploy to all devices simultaneously
                        </CardDescription>
                      </CardHeader>
                    </Card>
                  </div>
                </RadioGroup>
              </div>
              {/* Strategy Configuration */}
              {selectedStrategy === "canary" && (
                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm">Canary Configuration</CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div>
                      <Label className="text-xs">
                        Initial Percentage: {canaryConfig.initialPercentage}%
                      </Label>
                      <Slider
                        value={[canaryConfig.initialPercentage]}
                        onValueChange={([v]) =>
                          setCanaryConfig((prev) => ({
                            ...prev,
                            initialPercentage: v,
                          }))
                        }
                        min={5}
                        max={50}
                        step={5}
                        className="mt-2"
                      />
                    </div>
                    <div>
                      <Label className="text-xs">
                        Increment: {canaryConfig.incrementPercentage}%
                      </Label>
                      <Slider
                        value={[canaryConfig.incrementPercentage]}
                        onValueChange={([v]) =>
                          setCanaryConfig((prev) => ({
                            ...prev,
                            incrementPercentage: v,
                          }))
                        }
                        min={10}
                        max={50}
                        step={10}
                        className="mt-2"
                      />
                    </div>
                    <div>
                      <Label className="text-xs">Interval: {canaryConfig.incrementInterval}s</Label>
                      <Slider
                        value={[canaryConfig.incrementInterval]}
                        onValueChange={([v]) =>
                          setCanaryConfig((prev) => ({
                            ...prev,
                            incrementInterval: v,
                          }))
                        }
                        min={60}
                        max={1800}
                        step={60}
                        className="mt-2"
                      />
                    </div>
                    <div>
                      <Label className="text-xs">
                        Success Threshold: {(canaryConfig.successThreshold * 100).toFixed(0)}%
                      </Label>
                      <Slider
                        value={[canaryConfig.successThreshold * 100]}
                        onValueChange={([v]) =>
                          setCanaryConfig((prev) => ({
                            ...prev,
                            successThreshold: v / 100,
                          }))
                        }
                        min={90}
                        max={100}
                        step={1}
                        className="mt-2"
                      />
                    </div>
                  </CardContent>
                </Card>
              )}
              {/* Target Selection */}
              <div>
                <Label>Target Devices</Label>
                <RadioGroup value={targetGroup} onValueChange={setTargetGroup}>
                  <div className="space-y-2 mt-2">
                    <div className="flex items-center space-x-2">
                      <RadioGroupItem value="all" id={targetAllId} />
                      <Label htmlFor={targetAllId}>All Devices ({devices.length})</Label>
                    </div>
                    <div className="flex items-center space-x-2">
                      <RadioGroupItem value="group" id={targetGroupId} />
                      <Label htmlFor={targetGroupId}>Device Group</Label>
                    </div>
                    <div className="flex items-center space-x-2">
                      <RadioGroupItem value="tags" id={targetTagsId} />
                      <Label htmlFor={targetTagsId}>By Tags</Label>
                    </div>
                    <div className="flex items-center space-x-2">
                      <RadioGroupItem value="custom" id={targetCustomId} />
                      <Label htmlFor={targetCustomId}>Custom Selection</Label>
                    </div>
                  </div>
                </RadioGroup>
              </div>
              {/* Rollback Configuration */}
              <div className="flex items-center justify-between p-4 border rounded-lg">
                <div>
                  <Label>Automatic Rollback</Label>
                  <p className="text-sm text-muted-foreground">Rollback on failure threshold</p>
                </div>
                <Switch checked={rollbackEnabled} onCheckedChange={setRollbackEnabled} />
              </div>
              {/* Action Buttons */}
              <div className="flex justify-between">
                <Button variant="outline">
                  <HistoryIcon className="h-4 w-4 mr-2" />
                  Dry Run
                </Button>
                <Button onClick={startDeployment}>
                  <RocketIcon className="h-4 w-4 mr-2" />
                  Start Deployment
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
        {/* Active Deployments Tab */}
        <TabsContent value="active" className="space-y-4">
          {currentDeployment && (
            <Card>
              <CardHeader>
                <div className="flex items-center justify-between">
                  <div>
                    <CardTitle>
                      {currentDeployment.application} v{currentDeployment.version}
                    </CardTitle>
                    <CardDescription>
                      Deploying to {currentDeployment.targetDevices} devices using{" "}
                      {currentDeployment.strategy} strategy
                    </CardDescription>
                  </div>
                  <Badge
                    variant={
                      currentDeployment.status === "completed"
                        ? "default"
                        : currentDeployment.status === "failed"
                          ? "destructive"
                          : "secondary"
                    }
                  >
                    {currentDeployment.status.toUpperCase()}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent className="space-y-6">
                {/* Overall Progress */}
                <div>
                  <div className="flex justify-between text-sm mb-2">
                    <span>Overall Progress</span>
                    <span>{currentDeployment.progress}%</span>
                  </div>
                  <Progress value={currentDeployment.progress} className="h-2" />
                  <div className="flex justify-between text-xs text-muted-foreground mt-2">
                    <span>
                      <CheckCircleIcon className="h-3 w-3 inline text-green-500 mr-1" />
                      {Math.floor(currentDeployment.currentDevices * currentDeployment.successRate)}{" "}
                      Success
                    </span>
                    <span>
                      <XCircleIcon className="h-3 w-3 inline text-red-500 mr-1" />
                      {Math.floor(
                        currentDeployment.currentDevices * (1 - currentDeployment.successRate),
                      )}{" "}
                      Failed
                    </span>
                    <span>
                      <RefreshCwIcon className="h-3 w-3 inline text-blue-500 mr-1" />
                      {currentDeployment.targetDevices - currentDeployment.currentDevices} Remaining
                    </span>
                  </div>
                </div>
                {/* Deployment Phases */}
                <div>
                  <Label className="text-sm">Deployment Phases</Label>
                  <div className="space-y-2 mt-2">
                    {currentDeployment.phases.map((phase, idx: number) => (
                      <div
                        key={`phase-${phase.name}-${idx}`}
                        className="flex items-center gap-3 p-2 border rounded-lg"
                      >
                        <Badge
                          variant={
                            phase.status === "completed"
                              ? "default"
                              : phase.status === "in_progress"
                                ? "secondary"
                                : "outline"
                          }
                          className="w-24"
                        >
                          {phase.name}
                        </Badge>
                        <div className="flex-1">
                          <Progress
                            value={
                              phase.status === "completed"
                                ? 100
                                : phase.status === "in_progress"
                                  ? 50
                                  : 0
                            }
                            className="h-2"
                          />
                        </div>
                        <span className="text-xs text-muted-foreground w-20">
                          {phase.percentage}% ({phase.devices} devices)
                        </span>
                        <div className="text-xs">
                          <span className="text-green-500 mr-2">
                            ✓ {Math.floor(phase.devices * phase.successRate)}
                          </span>
                          {Math.floor(phase.devices * (1 - phase.successRate)) > 0 && (
                            <span className="text-red-500">
                              ✗ {Math.floor(phase.devices * (1 - phase.successRate))}
                            </span>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              </CardContent>
            </Card>
          )}
        </TabsContent>
        {/* Artifacts Tab */}
        <TabsContent value="artifacts" className="space-y-4">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle>Application Artifacts</CardTitle>
                <Button size="sm">
                  <UploadIcon className="h-4 w-4 mr-2" />
                  Upload Artifact
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              <div className="space-y-2">
                {["2.1.0", "2.0.0", "1.9.5"].map((version) => (
                  <div
                    key={version}
                    className="flex items-center justify-between p-3 border rounded-lg"
                  >
                    <div className="flex items-center gap-3">
                      <PackageIcon className="h-5 w-5 text-muted-foreground" />
                      <div>
                        <div className="font-medium">edge-inference</div>
                        <div className="text-sm text-muted-foreground">Version {version}</div>
                      </div>
                    </div>
                    <div className="flex items-center gap-4 text-sm text-muted-foreground">
                      <span>45.2 MB</span>
                      <span>linux/arm64</span>
                      <Badge variant={version === "2.1.0" ? "default" : "outline"}>
                        {version === "2.1.0" ? "Latest" : "Stable"}
                      </Badge>
                      <Button size="sm" variant="ghost">
                        <DownloadIcon className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
