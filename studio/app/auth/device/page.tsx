"use client";

import { useSearchParams } from "next/navigation";
import { useEffect, useState, Suspense } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CheckCircle2, Loader2, AlertCircle, Monitor } from "lucide-react";

function DeviceAuthContent() {
  const searchParams = useSearchParams();
  const [userCode, setUserCode] = useState("");
  const [status, setStatus] = useState<"pending" | "verifying" | "success" | "error">("pending");
  const [errorMessage, setErrorMessage] = useState("");

  // Get code from URL if provided
  useEffect(() => {
    const code = searchParams.get("code");
    if (code) {
      setUserCode(code.toUpperCase());
    }
  }, [searchParams]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!userCode || userCode.length !== 8) {
      setErrorMessage("Please enter a valid 8-character code");
      return;
    }

    setStatus("verifying");
    setErrorMessage("");

    try {
      // TODO: Replace with actual API call
      // Simulate API delay
      await new Promise(resolve => setTimeout(resolve, 1500));

      // For now, always succeed
      setStatus("success");

      // Automatically close window after success (if opened by CLI)
      setTimeout(() => {
        if (window.opener === null && window.history.length === 1) {
          window.close();
        }
      }, 3000);
    } catch (error) {
      setStatus("error");
      setErrorMessage("Failed to verify code. Please try again.");
    }
  };

  const formatCode = (code: string) => {
    // Format as XXXX-XXXX
    const cleaned = code.replace(/[^A-Z0-9]/g, "").toUpperCase();
    if (cleaned.length > 4) {
      return `${cleaned.slice(0, 4)}-${cleaned.slice(4, 8)}`;
    }
    return cleaned;
  };

  const handleCodeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const formatted = e.target.value.replace(/[^A-Z0-9]/gi, "").toUpperCase().slice(0, 8);
    setUserCode(formatted);
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-gray-800 p-4">
      <Card className="w-full max-w-md shadow-xl">
        <CardHeader className="text-center">
          <div className="flex justify-center mb-4">
            <div className="p-3 rounded-full bg-blue-100 dark:bg-blue-900">
              <Monitor className="h-8 w-8 text-blue-600 dark:text-blue-400" />
            </div>
          </div>
          <CardTitle className="text-2xl">Device Authentication</CardTitle>
          <CardDescription>
            Enter the code displayed in your terminal to authenticate FleetCTL
          </CardDescription>
        </CardHeader>

        <CardContent>
          {status === "success" ? (
            <div className="text-center py-8">
              <CheckCircle2 className="h-16 w-16 text-green-500 mx-auto mb-4" />
              <h3 className="text-lg font-semibold mb-2">Authentication Successful!</h3>
              <p className="text-muted-foreground">
                You can now return to your terminal. This window will close automatically.
              </p>
            </div>
          ) : (
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="code">Verification Code</Label>
                <Input
                  id="code"
                  type="text"
                  placeholder="XXXX-XXXX"
                  value={formatCode(userCode)}
                  onChange={handleCodeChange}
                  className="text-center text-2xl font-mono tracking-wider"
                  maxLength={9}
                  autoFocus
                  disabled={status === "verifying"}
                />
                {errorMessage && (
                  <div className="flex items-center gap-2 text-sm text-red-500">
                    <AlertCircle className="h-4 w-4" />
                    {errorMessage}
                  </div>
                )}
              </div>

              <Button
                type="submit"
                className="w-full"
                disabled={status === "verifying" || userCode.length !== 8}
              >
                {status === "verifying" ? (
                  <>
                    <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                    Verifying...
                  </>
                ) : (
                  "Authenticate Device"
                )}
              </Button>

              <div className="text-center text-sm text-muted-foreground pt-2">
                <p>This will grant FleetCTL access to manage your fleet.</p>
                <p className="mt-1">
                  The authentication token will be stored securely on your device.
                </p>
              </div>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

export default function DeviceAuthPage() {
  return (
    <Suspense fallback={
      <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-gray-800 p-4">
        <Card className="w-full max-w-md shadow-xl">
          <CardContent className="py-8">
            <div className="flex justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-blue-600" />
            </div>
          </CardContent>
        </Card>
      </div>
    }>
      <DeviceAuthContent />
    </Suspense>
  );
}