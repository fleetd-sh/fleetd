"use client";

import { useEffect, useRef } from "react";

export interface SSEOptions {
  onMessage?: (data: any) => void;
  onError?: (error: Event) => void;
  onOpen?: () => void;
  reconnectInterval?: number;
}

export function useSSE(url: string, options: SSEOptions = {}) {
  const { onMessage, onError, onOpen, reconnectInterval = 5000 } = options;
  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout>();

  useEffect(() => {
    const connect = () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }

      const eventSource = new EventSource(url);
      eventSourceRef.current = eventSource;

      eventSource.onopen = () => {
        console.log("SSE connection opened");
        onOpen?.();
      };

      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          onMessage?.(data);
        } catch (error) {
          console.error("Failed to parse SSE message:", error);
        }
      };

      eventSource.onerror = (error) => {
        console.error("SSE error:", error);
        onError?.(error);
        eventSource.close();

        // Attempt to reconnect
        reconnectTimeoutRef.current = setTimeout(() => {
          console.log("Attempting to reconnect SSE...");
          connect();
        }, reconnectInterval);
      };
    };

    connect();

    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
    };
  }, [url, onMessage, onError, onOpen, reconnectInterval]);

  return {
    close: () => eventSourceRef.current?.close(),
  };
}
