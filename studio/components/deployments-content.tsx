"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import {
  RocketIcon,
  ReloadIcon,
  PlayIcon,
  StopIcon,
  ClockIcon,
  CheckCircledIcon,
  CrossCircledIcon,
  UpdateIcon
} from "@radix-ui/react-icons";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { api } from "@/lib/api";
import { useSonnerToast } from "@/hooks/use-sonner-toast";
import { format } from "date-fns";

interface Deployment {
  id: string;
  name: string;
  version: string;
  status: "pending" | "running" | "completed" | "failed" | "cancelled";
  progress: number;
  devices_targeted: number;
  devices_completed: number;
  devices_failed: number;
  created_at: string;
  updated_at: string;
  artifact_url?: string;
  rollback_version?: string;
}

export function DeploymentsContent() {
  const { toast } = useSonnerToast();
  const [showCreateDeployment, setShowCreateDeployment] = React.useState(false);
  const [selectedDeployment, setSelectedDeployment] = React.useState<Deployment | null>(null);

  // Mock deployments data - replace with real API
  const mockDeployments: Deployment[] = [
    {
      id: "dep-001",
      name: "Firmware Update v2.1.0",
      version: "2.1.0",
      status: "running",
      progress: 65,
      devices_targeted: 150,
      devices_completed: 98,
      devices_failed: 2,
      created_at: new Date(Date.now() - 3600000).toISOString(),
      updated_at: new Date().toISOString(),
    },
    {
      id: "dep-002",
      name: "Security Patch KB2024-001",
      version: "1.0.0",
      status: "completed",
      progress: 100,
      devices_targeted: 150,
      devices_completed: 150,
      devices_failed: 0,
      created_at: new Date(Date.now() - 86400000).toISOString(),
      updated_at: new Date(Date.now() - 3600000).toISOString(),
    },
    {
      id: "dep-003",
      name: "Application Update",
      version: "3.5.2",
      status: "pending",
      progress: 0,
      devices_targeted: 75,
      devices_completed: 0,
      devices_failed: 0,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    },
  ];

  const { data: deployments = mockDeployments, refetch } = useQuery({
    queryKey: ["deployments"],
    queryFn: async () => mockDeployments, // Replace with: api.getDeployments
    refetchInterval: 10000,
  });

  const handleCreateDeployment = async (data: any) => {
    toast.promise(
      new Promise((resolve) => setTimeout(resolve, 2000)),
      {
        loading: "Creating deployment...",
        success: "Deployment created successfully",
        error: "Failed to create deployment"
      }
    );
    setShowCreateDeployment(false);
  };

  const handleCancelDeployment = async (id: string) => {
    toast.promise(
      new Promise((resolve) => setTimeout(resolve, 1000)),
      {
        loading: "Cancelling deployment...",
        success: "Deployment cancelled",
        error: "Failed to cancel deployment"
      }
    );
  };

  const handleRollback = async (id: string) => {
    toast.promise(
      new Promise((resolve) => setTimeout(resolve, 2000)),
      {
        loading: "Initiating rollback...",
        success: "Rollback initiated",
        error: "Failed to rollback"
      }
    );
  };

  const activeDeployments = deployments.filter(d => d.status === "running");
  const completedDeployments = deployments.filter(d => d.status === "completed");
  const failedDeployments = deployments.filter(d => d.status === "failed");

  return (
    <div className="space-y-6">
      {/* Stats Overview */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Active Deployments</CardTitle>
            <RocketIcon className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{activeDeployments.length}</div>
            <p className="text-xs text-muted-foreground">Currently running</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Success Rate</CardTitle>
            <CheckCircledIcon className="h-4 w-4 text-green-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">98.5%</div>
            <p className="text-xs text-muted-foreground">Last 30 days</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Deployed</CardTitle>
            <UpdateIcon className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">342</div>
            <p className="text-xs text-muted-foreground">This month</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Avg. Duration</CardTitle>
            <ClockIcon className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">12m</div>
            <p className="text-xs text-muted-foreground">Per deployment</p>
          </CardContent>
        </Card>
      </div>

      {/* Action Bar */}
      <div className="flex justify-between items-center">
        <h2 className="text-xl font-semibold">Deployment History</h2>
        <div className="flex gap-2">
          <Button onClick={() => refetch()} variant="outline" size="sm">
            <ReloadIcon className="mr-2 h-4 w-4" />
            Refresh
          </Button>
          <Button onClick={() => setShowCreateDeployment(true)} size="sm">
            <RocketIcon className="mr-2 h-4 w-4" />
            New Deployment
          </Button>
        </div>
      </div>

      {/* Deployments Tabs */}
      <Tabs defaultValue="all" className="space-y-4">
        <TabsList>
          <TabsTrigger value="all">All</TabsTrigger>
          <TabsTrigger value="active">Active</TabsTrigger>
          <TabsTrigger value="completed">Completed</TabsTrigger>
          <TabsTrigger value="failed">Failed</TabsTrigger>
        </TabsList>

        <TabsContent value="all" className="space-y-4">
          {deployments.map((deployment) => (
            <DeploymentCard
              key={deployment.id}
              deployment={deployment}
              onCancel={() => handleCancelDeployment(deployment.id)}
              onRollback={() => handleRollback(deployment.id)}
              onSelect={() => setSelectedDeployment(deployment)}
            />
          ))}
        </TabsContent>

        <TabsContent value="active" className="space-y-4">
          {activeDeployments.map((deployment) => (
            <DeploymentCard
              key={deployment.id}
              deployment={deployment}
              onCancel={() => handleCancelDeployment(deployment.id)}
              onRollback={() => handleRollback(deployment.id)}
              onSelect={() => setSelectedDeployment(deployment)}
            />
          ))}
        </TabsContent>

        <TabsContent value="completed" className="space-y-4">
          {completedDeployments.map((deployment) => (
            <DeploymentCard
              key={deployment.id}
              deployment={deployment}
              onCancel={() => handleCancelDeployment(deployment.id)}
              onRollback={() => handleRollback(deployment.id)}
              onSelect={() => setSelectedDeployment(deployment)}
            />
          ))}
        </TabsContent>

        <TabsContent value="failed" className="space-y-4">
          {failedDeployments.length === 0 ? (
            <Card>
              <CardContent className="py-8 text-center text-muted-foreground">
                No failed deployments
              </CardContent>
            </Card>
          ) : (
            failedDeployments.map((deployment) => (
              <DeploymentCard
                key={deployment.id}
                deployment={deployment}
                onCancel={() => handleCancelDeployment(deployment.id)}
                onRollback={() => handleRollback(deployment.id)}
                onSelect={() => setSelectedDeployment(deployment)}
              />
            ))
          )}
        </TabsContent>
      </Tabs>

      {/* Create Deployment Dialog */}
      <Dialog open={showCreateDeployment} onOpenChange={setShowCreateDeployment}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle>Create New Deployment</DialogTitle>
            <DialogDescription>
              Deploy an update to your fleet devices
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="name">Deployment Name</Label>
              <Input id="name" placeholder="e.g., Firmware Update v2.2.0" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="artifact">Artifact</Label>
              <Input id="artifact" type="file" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="target">Target Devices</Label>
              <Select>
                <SelectTrigger>
                  <SelectValue placeholder="Select target devices" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Devices</SelectItem>
                  <SelectItem value="group-a">Group A</SelectItem>
                  <SelectItem value="group-b">Group B</SelectItem>
                  <SelectItem value="custom">Custom Selection</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="strategy">Deployment Strategy</Label>
              <Select>
                <SelectTrigger>
                  <SelectValue placeholder="Select strategy" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="rolling">Rolling Update</SelectItem>
                  <SelectItem value="canary">Canary (10%)</SelectItem>
                  <SelectItem value="blue-green">Blue-Green</SelectItem>
                  <SelectItem value="all-at-once">All at Once</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setShowCreateDeployment(false)}>
              Cancel
            </Button>
            <Button onClick={handleCreateDeployment}>
              Deploy
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function DeploymentCard({
  deployment,
  onCancel,
  onRollback,
  onSelect
}: {
  deployment: Deployment;
  onCancel: () => void;
  onRollback: () => void;
  onSelect: () => void;
}) {
  const getStatusIcon = () => {
    switch (deployment.status) {
      case "running":
        return <PlayIcon className="h-4 w-4" />;
      case "completed":
        return <CheckCircledIcon className="h-4 w-4 text-green-600" />;
      case "failed":
        return <CrossCircledIcon className="h-4 w-4 text-red-600" />;
      case "cancelled":
        return <StopIcon className="h-4 w-4 text-gray-600" />;
      default:
        return <ClockIcon className="h-4 w-4" />;
    }
  };

  const getStatusColor = () => {
    switch (deployment.status) {
      case "running":
        return "default";
      case "completed":
        return "success";
      case "failed":
        return "destructive";
      case "cancelled":
        return "secondary";
      default:
        return "outline";
    }
  };

  return (
    <Card className="cursor-pointer hover:shadow-md transition-shadow" onClick={onSelect}>
      <CardHeader>
        <div className="flex items-start justify-between">
          <div className="space-y-1">
            <CardTitle className="text-lg">{deployment.name}</CardTitle>
            <CardDescription>
              Version {deployment.version} • Started {format(new Date(deployment.created_at), "PPp")}
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            {getStatusIcon()}
            <Badge variant={getStatusColor() as any}>
              {deployment.status}
            </Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {deployment.status === "running" && (
          <div className="space-y-2">
            <div className="flex justify-between text-sm">
              <span>Progress</span>
              <span>{deployment.progress}%</span>
            </div>
            <Progress value={deployment.progress} />
          </div>
        )}

        <div className="grid grid-cols-3 gap-4 text-sm">
          <div>
            <p className="text-muted-foreground">Targeted</p>
            <p className="font-medium">{deployment.devices_targeted} devices</p>
          </div>
          <div>
            <p className="text-muted-foreground">Completed</p>
            <p className="font-medium text-green-600">{deployment.devices_completed}</p>
          </div>
          <div>
            <p className="text-muted-foreground">Failed</p>
            <p className="font-medium text-red-600">{deployment.devices_failed}</p>
          </div>
        </div>

        {deployment.status === "running" && (
          <div className="flex gap-2" onClick={(e) => e.stopPropagation()}>
            <Button size="sm" variant="outline" onClick={onCancel}>
              <StopIcon className="mr-2 h-4 w-4" />
              Cancel
            </Button>
            <Button size="sm" variant="outline" onClick={onRollback}>
              <UpdateIcon className="mr-2 h-4 w-4" />
              Rollback
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}