"use client";
import { ChevronRightIcon, CircleIcon } from "@radix-ui/react-icons";
import { format } from "date-fns";
import { AnimatePresence, motion } from "framer-motion";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import type { Device } from "@/lib/types";
import { cn } from "@/lib/utils";

interface DeviceListProps {
  devices: Device[];
  selectedDevice: string | null;
  onSelectDevice: (deviceId: string | null) => void;
}
export function DeviceList({ devices, selectedDevice, onSelectDevice }: DeviceListProps) {
  return (
    <ScrollArea className="h-[400px] pr-4">
      <AnimatePresence mode="popLayout">
        {devices.length === 0 ? (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="flex items-center justify-center h-full text-muted-foreground"
          >
            No devices found
          </motion.div>
        ) : (
          <div className="space-y-2">
            {devices.map((device, index) => (
              <motion.div
                key={device.id}
                layout
                initial={{ opacity: 0, x: -20 }}
                animate={{ opacity: 1, x: 0 }}
                exit={{ opacity: 0, x: 20 }}
                transition={{ delay: index * 0.05 }}
                className={cn(
                  "p-4 rounded-lg border transition-all cursor-pointer hover:shadow-md",
                  selectedDevice === device.id
                    ? "border-primary bg-primary/5"
                    : "hover:border-primary/50",
                )}
                onClick={() => onSelectDevice(device.id === selectedDevice ? null : device.id)}
              >
                <div className="flex items-center justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <CircleIcon
                        className={cn(
                          "h-2 w-2 fill-current",
                          device.status === "online" ? "text-green-500" : "text-gray-400",
                        )}
                      />
                      <h3 className="font-semibold">{device.name}</h3>
                      <Badge variant="outline" className="text-xs">
                        {device.type}
                      </Badge>
                    </div>
                    <div className="flex items-center gap-4 mt-2 text-sm text-muted-foreground">
                      <span>v{device.version}</span>
                      <span>ID: {device.id.slice(0, 8)}...</span>
                      <span>Last seen: {format(new Date(device.last_seen), "HH:mm:ss")}</span>
                    </div>
                  </div>
                  <ChevronRightIcon
                    className={cn(
                      "h-5 w-5 transition-transform",
                      selectedDevice === device.id && "rotate-90",
                    )}
                  />
                </div>
              </motion.div>
            ))}
          </div>
        )}
      </AnimatePresence>
    </ScrollArea>
  );
}
