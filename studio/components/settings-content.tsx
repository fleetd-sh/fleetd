"use client";
import { CheckIcon, CopyIcon, LockClosedIcon as KeyIcon } from "@radix-ui/react-icons";
import { useQuery } from "@tanstack/react-query";
import * as React from "react";
import { useId } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useSonnerToast } from "@/hooks/use-sonner-toast";
import { useSettingsClient } from "@/lib/api/connect-hooks";
export function SettingsContent() {
  const { toast } = useSonnerToast();
  const settingsClient = useSettingsClient();
  const [apiKey, setApiKey] = React.useState("fleetd_sk_test_4242424242424242");
  const [copiedKey, setCopiedKey] = React.useState(false);

  // Generate unique IDs for form elements
  const orgNameId = useId();
  const orgEmailId = useId();
  const timezoneId = useId();
  const languageId = useId();
  const webhookUrlId = useId();
  const { data: orgSettings } = useQuery({
    queryKey: ["org-settings"],
    queryFn: async () => {
      const response = await settingsClient.getOrganizationSettings({});
      return response.settings;
    },
  });
  useQuery({
    queryKey: ["security-settings"],
    queryFn: async () => {
      const response = await settingsClient.getSecuritySettings({});
      return response.settings;
    },
  });
  const { refetch: refetchApiSettings } = useQuery({
    queryKey: ["api-settings"],
    queryFn: async () => {
      const response = await settingsClient.getAPISettings({});
      if (response.settings?.apiKey) {
        setApiKey(response.settings.apiKey);
      }
      return response.settings;
    },
  });
  const handleSaveSettings = () => {
    toast.promise(new Promise((resolve) => setTimeout(resolve, 1000)), {
      loading: "Saving settings...",
      success: "Settings saved successfully",
      error: "Failed to save settings",
    });
  };
  const handleCopyApiKey = () => {
    navigator.clipboard.writeText(apiKey);
    setCopiedKey(true);
    toast.success("API key copied to clipboard");
    setTimeout(() => setCopiedKey(false), 2000);
  };
  const handleRegenerateApiKey = () => {
    toast.promise(
      (async () => {
        const response = await settingsClient.regenerateAPIKey({});
        if (response.newApiKey) {
          setApiKey(response.newApiKey);
          refetchApiSettings();
        }
        return response;
      })(),
      {
        loading: "Regenerating API key...",
        success: "New API key generated",
        error: "Failed to regenerate API key",
      },
    );
  };
  return (
    <div className="space-y-6">
      <Tabs defaultValue="general" className="space-y-4">
        <TabsList className="grid w-full grid-cols-5">
          <TabsTrigger value="general">General</TabsTrigger>
          <TabsTrigger value="security">Security</TabsTrigger>
          <TabsTrigger value="notifications">Notifications</TabsTrigger>
          <TabsTrigger value="api">API</TabsTrigger>
          <TabsTrigger value="advanced">Advanced</TabsTrigger>
        </TabsList>
        <TabsContent value="general" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Organization Settings</CardTitle>
              <CardDescription>Manage your organization details and preferences</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-2">
                <Label htmlFor={orgNameId}>Organization Name</Label>
                <Input id={orgNameId} defaultValue={orgSettings?.name || "Acme Corporation"} />
              </div>
              <div className="space-y-2">
                <Label htmlFor={orgEmailId}>Contact Email</Label>
                <Input
                  id={orgEmailId}
                  type="email"
                  defaultValue={orgSettings?.contactEmail || "admin@acme.com"}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor={timezoneId}>Timezone</Label>
                <Select defaultValue={orgSettings?.timezone?.toLowerCase() || "utc"}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="utc">UTC</SelectItem>
                    <SelectItem value="est">Eastern Time</SelectItem>
                    <SelectItem value="pst">Pacific Time</SelectItem>
                    <SelectItem value="cst">Central Time</SelectItem>
                    <SelectItem value="mst">Mountain Time</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor={languageId}>Language</Label>
                <Select defaultValue="en">
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="en">English</SelectItem>
                    <SelectItem value="es">Spanish</SelectItem>
                    <SelectItem value="fr">French</SelectItem>
                    <SelectItem value="de">German</SelectItem>
                    <SelectItem value="jp">Japanese</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <Separator />
              <div className="flex justify-end">
                <Button onClick={handleSaveSettings}>Save Changes</Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="security" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Security Settings</CardTitle>
              <CardDescription>Configure security and authentication settings</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="flex items-center justify-between">
                <div className="space-y-1">
                  <Label>Two-Factor Authentication</Label>
                  <p className="text-sm text-muted-foreground">
                    Require 2FA for all admin accounts
                  </p>
                </div>
                <Switch defaultChecked />
              </div>
              <div className="flex items-center justify-between">
                <div className="space-y-1">
                  <Label>Session Timeout</Label>
                  <p className="text-sm text-muted-foreground">
                    Automatically log out after inactivity
                  </p>
                </div>
                <Select defaultValue="30">
                  <SelectTrigger className="w-32">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="15">15 min</SelectItem>
                    <SelectItem value="30">30 min</SelectItem>
                    <SelectItem value="60">1 hour</SelectItem>
                    <SelectItem value="120">2 hours</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-center justify-between">
                <div className="space-y-1">
                  <Label>IP Whitelist</Label>
                  <p className="text-sm text-muted-foreground">
                    Restrict access to specific IP addresses
                  </p>
                </div>
                <Switch />
              </div>
              <div className="flex items-center justify-between">
                <div className="space-y-1">
                  <Label>Audit Logging</Label>
                  <p className="text-sm text-muted-foreground">Log all administrative actions</p>
                </div>
                <Switch defaultChecked />
              </div>
              <Separator />
              <div className="space-y-2">
                <Label>Password Policy</Label>
                <div className="space-y-3">
                  <div className="flex items-center gap-2">
                    <Switch defaultChecked />
                    <Label className="font-normal">Minimum 8 characters</Label>
                  </div>
                  <div className="flex items-center gap-2">
                    <Switch defaultChecked />
                    <Label className="font-normal">Require uppercase and lowercase</Label>
                  </div>
                  <div className="flex items-center gap-2">
                    <Switch defaultChecked />
                    <Label className="font-normal">Require numbers</Label>
                  </div>
                  <div className="flex items-center gap-2">
                    <Switch />
                    <Label className="font-normal">Require special characters</Label>
                  </div>
                </div>
              </div>
              <Separator />
              <div className="flex justify-end">
                <Button onClick={handleSaveSettings}>Save Security Settings</Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="notifications" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Notification Preferences</CardTitle>
              <CardDescription>Configure how you receive alerts and updates</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-4">
                <h4 className="text-sm font-medium">Email Notifications</h4>
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <Label className="font-normal">Device Offline Alerts</Label>
                    <Switch defaultChecked />
                  </div>
                  <div className="flex items-center justify-between">
                    <Label className="font-normal">Deployment Status Updates</Label>
                    <Switch defaultChecked />
                  </div>
                  <div className="flex items-center justify-between">
                    <Label className="font-normal">Security Alerts</Label>
                    <Switch defaultChecked />
                  </div>
                  <div className="flex items-center justify-between">
                    <Label className="font-normal">Weekly Summary Reports</Label>
                    <Switch />
                  </div>
                </div>
              </div>
              <Separator />
              <div className="space-y-4">
                <h4 className="text-sm font-medium">Webhook Notifications</h4>
                <div className="space-y-2">
                  <Label htmlFor={webhookUrlId}>Webhook URL</Label>
                  <Input
                    id={webhookUrlId}
                    placeholder="https://your-domain.com/webhook"
                    defaultValue=""
                  />
                </div>
                <div className="flex items-center gap-2">
                  <Switch />
                  <Label className="font-normal">Enable webhook notifications</Label>
                </div>
              </div>
              <Separator />
              <div className="space-y-4">
                <h4 className="text-sm font-medium">Alert Thresholds</h4>
                <div className="space-y-3">
                  <div className="space-y-2">
                    <Label>CPU Usage Alert (%)</Label>
                    <Input type="number" defaultValue="80" />
                  </div>
                  <div className="space-y-2">
                    <Label>Memory Usage Alert (%)</Label>
                    <Input type="number" defaultValue="85" />
                  </div>
                  <div className="space-y-2">
                    <Label>Disk Usage Alert (%)</Label>
                    <Input type="number" defaultValue="90" />
                  </div>
                </div>
              </div>
              <Separator />
              <div className="flex justify-end">
                <Button onClick={handleSaveSettings}>Save Notification Settings</Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="api" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>API Configuration</CardTitle>
              <CardDescription>Manage API keys and access tokens</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-4">
                <div className="space-y-2">
                  <Label>API Key</Label>
                  <div className="flex gap-2">
                    <Input value={apiKey} readOnly className="font-mono text-sm" />
                    <Button variant="outline" size="icon" onClick={handleCopyApiKey}>
                      {copiedKey ? (
                        <CheckIcon className="h-4 w-4" />
                      ) : (
                        <CopyIcon className="h-4 w-4" />
                      )}
                    </Button>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Use this key to authenticate API requests
                  </p>
                </div>
                <Button variant="outline" onClick={handleRegenerateApiKey} className="w-full">
                  <KeyIcon className="mr-2 h-4 w-4" />
                  Regenerate API Key
                </Button>
              </div>
              <Separator />
              <div className="space-y-4">
                <h4 className="text-sm font-medium">API Rate Limits</h4>
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label>Requests per minute</Label>
                    <Input type="number" defaultValue="60" />
                  </div>
                  <div className="space-y-2">
                    <Label>Requests per hour</Label>
                    <Input type="number" defaultValue="1000" />
                  </div>
                </div>
              </div>
              <Separator />
              <div className="space-y-4">
                <h4 className="text-sm font-medium">CORS Settings</h4>
                <div className="space-y-2">
                  <Label>Allowed Origins</Label>
                  <Input
                    placeholder="https://example.com, https://app.example.com"
                    defaultValue="*"
                  />
                </div>
                <div className="flex items-center gap-2">
                  <Switch defaultChecked />
                  <Label className="font-normal">Allow credentials</Label>
                </div>
              </div>
              <Separator />
              <div className="flex justify-end">
                <Button onClick={handleSaveSettings}>Save API Settings</Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="advanced" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Advanced Settings</CardTitle>
              <CardDescription>Configure advanced system settings</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-4">
                <h4 className="text-sm font-medium">Data Retention</h4>
                <div className="space-y-3">
                  <div className="space-y-2">
                    <Label>Telemetry Data</Label>
                    <Select defaultValue="30">
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="7">7 days</SelectItem>
                        <SelectItem value="30">30 days</SelectItem>
                        <SelectItem value="90">90 days</SelectItem>
                        <SelectItem value="365">1 year</SelectItem>
                        <SelectItem value="0">Forever</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>System Logs</Label>
                    <Select defaultValue="90">
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="7">7 days</SelectItem>
                        <SelectItem value="30">30 days</SelectItem>
                        <SelectItem value="90">90 days</SelectItem>
                        <SelectItem value="365">1 year</SelectItem>
                        <SelectItem value="0">Forever</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              </div>
              <Separator />
              <div className="space-y-4">
                <h4 className="text-sm font-medium">Experimental Features</h4>
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="space-y-1">
                      <Label>Beta Features</Label>
                      <p className="text-xs text-muted-foreground">
                        Enable access to beta features
                      </p>
                    </div>
                    <Switch />
                  </div>
                  <div className="flex items-center justify-between">
                    <div className="space-y-1">
                      <Label>Debug Mode</Label>
                      <p className="text-xs text-muted-foreground">
                        Enable verbose logging for debugging
                      </p>
                    </div>
                    <Switch />
                  </div>
                </div>
              </div>
              <Separator />
              <div className="space-y-4">
                <h4 className="text-sm font-medium">Danger Zone</h4>
                <div className="rounded-lg border border-destructive p-4 space-y-4">
                  <div className="space-y-2">
                    <h5 className="font-medium text-destructive">Export All Data</h5>
                    <p className="text-sm text-muted-foreground">
                      Download all your data in JSON format
                    </p>
                    <Button variant="outline" className="w-full">
                      Export Data
                    </Button>
                  </div>
                  <div className="space-y-2">
                    <h5 className="font-medium text-destructive">Delete All Data</h5>
                    <p className="text-sm text-muted-foreground">
                      Permanently delete all data. This action cannot be undone.
                    </p>
                    <Button variant="destructive" className="w-full">
                      Delete Everything
                    </Button>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
