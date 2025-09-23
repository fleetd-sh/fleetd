"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import {
  CalendarIcon,
  CubeIcon,
  GearIcon,
  HomeIcon,
  MagnifyingGlassIcon,
  PersonIcon,
  ReloadIcon,
  RocketIcon,
} from "@radix-ui/react-icons";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from "@/components/ui/command";
import { useToast } from "@/hooks/use-toast";

export function CommandPalette() {
  const [open, setOpen] = React.useState(false);
  const router = useRouter();
  const { toast } = useToast();

  React.useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((open) => !open);
      }
    };

    document.addEventListener("keydown", down);
    return () => document.removeEventListener("keydown", down);
  }, []);

  const runCommand = React.useCallback((command: () => void) => {
    setOpen(false);
    command();
  }, []);

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        className="hidden md:inline-flex items-center gap-2 rounded-lg border bg-muted px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
      >
        <MagnifyingGlassIcon className="h-3 w-3" />
        <span>Search...</span>
        <CommandShortcut>⌘K</CommandShortcut>
      </button>
      <CommandDialog open={open} onOpenChange={setOpen}>
        <CommandInput placeholder="Type a command or search..." />
        <CommandList>
          <CommandEmpty>No results found.</CommandEmpty>
          <CommandGroup heading="Navigation">
            <CommandItem onSelect={() => runCommand(() => router.push("/"))}>
              <HomeIcon className="mr-2 h-4 w-4" />
              <span>Dashboard</span>
            </CommandItem>
            <CommandItem onSelect={() => runCommand(() => router.push("/devices"))}>
              <CubeIcon className="mr-2 h-4 w-4" />
              <span>Devices</span>
            </CommandItem>
            <CommandItem onSelect={() => runCommand(() => router.push("/deployments"))}>
              <RocketIcon className="mr-2 h-4 w-4" />
              <span>Deployments</span>
            </CommandItem>
            <CommandItem onSelect={() => runCommand(() => router.push("/telemetry"))}>
              <CalendarIcon className="mr-2 h-4 w-4" />
              <span>Telemetry</span>
            </CommandItem>
            <CommandItem onSelect={() => runCommand(() => router.push("/settings"))}>
              <GearIcon className="mr-2 h-4 w-4" />
              <span>Settings</span>
            </CommandItem>
          </CommandGroup>
          <CommandSeparator />
          <CommandGroup heading="Actions">
            <CommandItem
              onSelect={() =>
                runCommand(() => {
                  window.location.reload();
                })
              }
            >
              <ReloadIcon className="mr-2 h-4 w-4" />
              <span>Refresh Page</span>
            </CommandItem>
            <CommandItem
              onSelect={() =>
                runCommand(() => {
                  toast({
                    title: "Device Discovery",
                    description: "Scanning network for devices...",
                  });
                })
              }
            >
              <MagnifyingGlassIcon className="mr-2 h-4 w-4" />
              <span>Discover Devices</span>
            </CommandItem>
            <CommandItem
              onSelect={() =>
                runCommand(() => {
                  toast({
                    title: "Quick Provision",
                    description: "Starting device provisioning...",
                  });
                })
              }
            >
              <RocketIcon className="mr-2 h-4 w-4" />
              <span>Quick Provision Device</span>
            </CommandItem>
          </CommandGroup>
          <CommandSeparator />
          <CommandGroup heading="Help">
            <CommandItem
              onSelect={() =>
                runCommand(() => {
                  window.open("https://github.com/fleetd/docs", "_blank");
                })
              }
            >
              <PersonIcon className="mr-2 h-4 w-4" />
              <span>Documentation</span>
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </>
  );
}