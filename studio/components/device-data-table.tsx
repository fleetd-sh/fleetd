"use client";
import { ChevronDownIcon, DotsHorizontalIcon } from "@radix-ui/react-icons";
import type { ColumnDef } from "@tanstack/react-table";
import { format } from "date-fns";
import * as React from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DataTable } from "@/components/ui/data-table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { Device } from "@/lib/types";
import { cn } from "@/lib/utils";

interface DeviceDataTableProps {
  devices: Device[];
  onSelectDevice?: (device: Device) => void;
}
export function DeviceDataTable({ devices, onSelectDevice }: DeviceDataTableProps) {
  const columns: ColumnDef<Device>[] = React.useMemo(
    () => [
      {
        id: "select",
        header: ({ table }) => (
          <Checkbox
            checked={
              table.getIsAllPageRowsSelected() ||
              (table.getIsSomePageRowsSelected() && "indeterminate")
            }
            onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
            aria-label="Select all"
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(value) => row.toggleSelected(!!value)}
            aria-label="Select row"
          />
        ),
        enableSorting: false,
        enableHiding: false,
      },
      {
        accessorKey: "status",
        header: "Status",
        cell: ({ row }) => {
          const status = row.getValue<string>("status");
          return (
            <div className="flex items-center gap-2">
              <div
                className={cn(
                  "h-2 w-2 rounded-full",
                  status === "online" ? "bg-green-500" : "bg-gray-400",
                )}
              />
              <span className="capitalize">{status}</span>
            </div>
          );
        },
      },
      {
        accessorKey: "name",
        header: ({ column }) => {
          return (
            <Button
              variant="ghost"
              onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
            >
              Name
              <ChevronDownIcon className="ml-2 h-4 w-4" />
            </Button>
          );
        },
        cell: ({ row }) => {
          const device = row.original;
          return (
            <Button
              onClick={() => onSelectDevice?.(device)}
              className="font-medium hover:underline text-left"
            >
              {device.name}
            </Button>
          );
        },
      },
      {
        accessorKey: "type",
        header: "Type",
        cell: ({ row }) => {
          return <Badge variant="outline">{row.getValue("type")}</Badge>;
        },
      },
      {
        accessorKey: "version",
        header: "Version",
        cell: ({ row }) => {
          return <code className="text-sm">v{row.getValue("version")}</code>;
        },
      },
      {
        accessorKey: "id",
        header: "Device ID",
        cell: ({ row }) => {
          const id = row.getValue<string>("id");
          return <code className="text-xs text-muted-foreground">{id.slice(0, 8)}...</code>;
        },
      },
      {
        accessorKey: "last_seen",
        header: ({ column }) => {
          return (
            <Button
              variant="ghost"
              onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
            >
              Last Seen
              <ChevronDownIcon className="ml-2 h-4 w-4" />
            </Button>
          );
        },
        cell: ({ row }) => {
          const date = new Date(row.getValue("last_seen"));
          return (
            <time dateTime={date.toISOString()} className="text-sm text-muted-foreground">
              {format(date, "MMM d, HH:mm:ss")}
            </time>
          );
        },
      },
      {
        id: "actions",
        enableHiding: false,
        cell: ({ row }) => {
          const device = row.original;
          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" className="h-8 w-8 p-0">
                  <span className="sr-only">Open menu</span>
                  <DotsHorizontalIcon className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuLabel>Actions</DropdownMenuLabel>
                <DropdownMenuItem onClick={() => onSelectDevice?.(device)}>
                  View details
                </DropdownMenuItem>
                <DropdownMenuItem>Deploy application</DropdownMenuItem>
                <DropdownMenuItem>View telemetry</DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem className="text-destructive">Remove device</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          );
        },
      },
    ],
    [onSelectDevice],
  );
  return <DataTable columns={columns} data={devices} searchKey="name" />;
}
