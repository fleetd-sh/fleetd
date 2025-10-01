import { toast as sonnerToast } from "sonner";

export function useSonnerToast() {
  return {
    toast: sonnerToast,
    success: (message: string, description?: string) =>
      sonnerToast.success(message, { description }),
    error: (message: string, description?: string) => sonnerToast.error(message, { description }),
    info: (message: string, description?: string) => sonnerToast.info(message, { description }),
    warning: (message: string, description?: string) =>
      sonnerToast.warning(message, { description }),
    loading: (message: string, description?: string) =>
      sonnerToast.loading(message, { description }),
    promise: <T>(
      promise: Promise<T>,
      options: {
        loading: string;
        success: string | ((data: T) => string);
        error: string | ((error: unknown) => string);
      },
    ) => sonnerToast.promise(promise, options),
  };
}
