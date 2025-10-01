"use client";

import { CheckIcon, CopyIcon } from "lucide-react";
import { useTheme } from "next-themes";
import {
  type ComponentProps,
  createContext,
  forwardRef,
  type HTMLAttributes,
  useContext,
  useState,
} from "react";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { oneDark, oneLight } from "react-syntax-highlighter/dist/cjs/styles/prism";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";

// Context for managing snippet state
type SnippetContextValue = {
  value: string;
  onValueChange: (value: string) => void;
};

const SnippetContext = createContext<SnippetContextValue | undefined>(undefined);

const _useSnippet = () => {
  const context = useContext(SnippetContext);
  if (!context) {
    throw new Error("useSnippet must be used within a Snippet");
  }
  return context;
};

// Main Snippet wrapper
export type SnippetProps = ComponentProps<typeof Tabs> & {
  language?: string;
};

export const Snippet = forwardRef<HTMLDivElement, SnippetProps>(
  ({ className, children, language, ...props }, ref) => {
    const [value, setValue] = useState(props.value || props.defaultValue || "");

    return (
      <SnippetContext.Provider
        value={{
          value: props.value || value,
          onValueChange: props.onValueChange || setValue,
        }}
      >
        <Tabs
          ref={ref}
          className={cn("group w-full overflow-hidden rounded-lg border", className)}
          data-language={language}
          {...props}
        >
          {children}
        </Tabs>
      </SnippetContext.Provider>
    );
  },
);
Snippet.displayName = "Snippet";

// Header with language indicator
export type SnippetHeaderProps = HTMLAttributes<HTMLDivElement> & {
  language?: string;
};

export const SnippetHeader = forwardRef<HTMLDivElement, SnippetHeaderProps>(
  ({ className, children, language, ...props }, ref) => (
    <div
      ref={ref}
      className={cn(
        "flex h-10 flex-row items-center justify-between border-b bg-secondary/50 px-3",
        className,
      )}
      {...props}
    >
      {language && <span className="text-xs font-medium text-muted-foreground">{language}</span>}
      <div className="ml-auto">{children}</div>
    </div>
  ),
);
SnippetHeader.displayName = "SnippetHeader";

// Tabs components for multiple code snippets
export const SnippetTabsList = forwardRef<HTMLDivElement, ComponentProps<typeof TabsList>>(
  ({ className, ...props }, ref) => (
    <TabsList ref={ref} className={cn("h-auto gap-1 bg-transparent p-0", className)} {...props} />
  ),
);
SnippetTabsList.displayName = "SnippetTabsList";

export const SnippetTabsTrigger = forwardRef<HTMLButtonElement, ComponentProps<typeof TabsTrigger>>(
  ({ className, children, ...props }, ref) => (
    <TabsTrigger
      ref={ref}
      className={cn(
        "h-auto gap-1 rounded px-2 py-1 text-xs font-medium data-[state=active]:bg-background data-[state=active]:text-foreground data-[state=active]:shadow-sm",
        className,
      )}
      {...props}
    >
      {children}
    </TabsTrigger>
  ),
);
SnippetTabsTrigger.displayName = "SnippetTabsTrigger";

export const SnippetTabsContent = forwardRef<HTMLDivElement, ComponentProps<typeof TabsContent>>(
  ({ className, children, ...props }, ref) => (
    <TabsContent ref={ref} className={cn("mt-0", className)} {...props}>
      {children}
    </TabsContent>
  ),
);
SnippetTabsContent.displayName = "SnippetTabsContent";

// Copy button
export type SnippetCopyButtonProps = ComponentProps<typeof Button> & {
  value: string;
  onCopy?: () => void;
  onError?: (error: Error) => void;
  timeout?: number;
};

export const SnippetCopyButton = ({
  value,
  onCopy,
  onError,
  timeout = 2000,
  children,
  className,
  ...props
}: SnippetCopyButtonProps) => {
  const [isCopied, setIsCopied] = useState(false);

  const copyToClipboard = () => {
    if (typeof window === "undefined" || !navigator.clipboard.writeText || !value) {
      return;
    }

    navigator.clipboard.writeText(value).then(() => {
      setIsCopied(true);
      onCopy?.();
      setTimeout(() => setIsCopied(false), timeout);
    }, onError);
  };

  const icon = isCopied ? (
    <CheckIcon size={14} className="text-muted-foreground" />
  ) : (
    <CopyIcon size={14} className="text-muted-foreground" />
  );

  return (
    <Button
      className={cn(
        "h-7 px-2 text-muted-foreground transition-all hover:bg-secondary hover:text-foreground",
        className,
      )}
      onClick={copyToClipboard}
      size="sm"
      variant="ghost"
      {...props}
    >
      {children ?? icon}
    </Button>
  );
};

// Content with syntax highlighting
export type SnippetContentProps = HTMLAttributes<HTMLDivElement> & {
  language?: string;
  showLineNumbers?: boolean;
};

export const SnippetContent = forwardRef<HTMLDivElement, SnippetContentProps>(
  ({ className, children, language = "bash", showLineNumbers = false, ...props }, ref) => {
    const { theme } = useTheme();
    const syntaxTheme = theme === "dark" ? oneDark : oneLight;

    // If language is specified and not 'none', use syntax highlighting
    if (language && language !== "none") {
      return (
        <div ref={ref} className={cn("bg-muted/30 overflow-x-auto rounded", className)} {...props}>
          <SyntaxHighlighter
            language={language}
            style={syntaxTheme}
            showLineNumbers={showLineNumbers}
            customStyle={{
              margin: 0,
              padding: "0.75rem 1rem",
              background: "transparent",
              fontSize: "0.8125rem",
              lineHeight: "1.5",
            }}
          >
            {String(children).trim()}
          </SyntaxHighlighter>
        </div>
      );
    }

    // Fallback to plain pre element
    return (
      <div ref={ref} className={cn("bg-muted/30 overflow-x-auto rounded", className)} {...props}>
        <pre className="p-3 text-[0.8125rem] leading-[1.5]">
          <code>{children}</code>
        </pre>
      </div>
    );
  },
);
SnippetContent.displayName = "SnippetContent";
