import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";
import { typography, type TypographySize } from "@/lib/typography";

// Text component with Atlas typography system
const textVariants = cva("", {
  variants: {
    size: {
      xs: "text-xs-atlas",
      sm: "text-sm-atlas",
      base: "text-base-atlas",
      md: "text-md-atlas",
      lg: "text-lg-atlas",
      xl: "text-xl-atlas",
      display: "text-display-atlas",
    },
    variant: {
      default: "text-foreground",
      muted: "text-muted-foreground",
      subtle: "text-muted-foreground/80",
      accent: "text-primary",
      destructive: "text-destructive",
      success: "text-success-foreground",
      warning: "text-warning-foreground",
    },
    weight: {
      normal: "font-normal",
      medium: "font-medium",
      semibold: "font-semibold",
      bold: "font-bold",
    },
  },
  defaultVariants: {
    size: "base",
    variant: "default",
    weight: "normal",
  },
});

export interface TextProps
  extends React.HTMLAttributes<HTMLParagraphElement>,
    VariantProps<typeof textVariants> {
  as?: "p" | "span" | "div" | "label";
}

export const Text = React.forwardRef<HTMLElement, TextProps>(
  ({ className, size, variant, weight, as: Component = "p", ...props }, ref) => {
    return React.createElement(
      Component,
      {
        className: cn(textVariants({ size, variant, weight, className })),
        ref,
        ...props
      }
    );
  },
);
Text.displayName = "Text";

// Heading component with Atlas typography system
const headingVariants = cva("font-semibold tracking-tight", {
  variants: {
    size: {
      sm: "text-md-atlas",
      base: "text-lg-atlas",
      lg: "text-xl-atlas",
      xl: "text-display-atlas",
    },
    variant: {
      default: "text-foreground",
      muted: "text-muted-foreground",
      accent: "text-primary",
    },
  },
  defaultVariants: {
    size: "base",
    variant: "default",
  },
});

export interface HeadingProps
  extends React.HTMLAttributes<HTMLHeadingElement>,
    VariantProps<typeof headingVariants> {
  level?: 1 | 2 | 3 | 4 | 5 | 6;
}

export const Heading = React.forwardRef<HTMLHeadingElement, HeadingProps>(
  ({ className, size, variant, level = 2, ...props }, ref) => {
    const Component = `h${level}` as any;

    return (
      <Component
        className={cn(headingVariants({ size, variant, className }))}
        ref={ref}
        {...props}
      />
    );
  },
);
Heading.displayName = "Heading";

// Metric component - optimized for displaying numbers/stats
export interface MetricProps extends React.HTMLAttributes<HTMLDivElement> {
  value: string | number;
  label?: string;
  sublabel?: string;
  variant?: "default" | "success" | "warning" | "destructive";
}

export const Metric = React.forwardRef<HTMLDivElement, MetricProps>(
  ({ className, value, label, sublabel, variant = "default", ...props }, ref) => {
    const getValueColor = () => {
      switch (variant) {
        case "success": return "text-success-foreground";
        case "warning": return "text-warning-foreground";
        case "destructive": return "text-destructive";
        default: return "text-foreground";
      }
    };

    return (
      <div
        ref={ref}
        className={cn("space-y-baseline-1", className)}
        {...props}
      >
        {label && (
          <Text size="sm" variant="muted" weight="medium">
            {label}
          </Text>
        )}
        <div className={cn("text-xl-atlas font-bold", getValueColor())}>
          {value}
        </div>
        {sublabel && (
          <Text size="xs" variant="subtle">
            {sublabel}
          </Text>
        )}
      </div>
    );
  },
);
Metric.displayName = "Metric";

// Code/Mono component for technical text
export interface CodeProps
  extends React.HTMLAttributes<HTMLElement> {
  inline?: boolean;
  size?: "xs" | "sm" | "base";
}

export const Code = React.forwardRef<HTMLElement, CodeProps>(
  ({ className, inline = true, size = "sm", children, ...props }, ref) => {
    const Component = inline ? "code" : "pre";
    const sizeClass = size === "xs" ? "text-xs" : size === "sm" ? "text-sm" : "text-base";

    return (
      <Component
        ref={ref as any}
        className={cn(
          "font-mono",
          sizeClass,
          inline
            ? "relative rounded px-[0.3rem] py-[0.2rem] bg-muted text-muted-foreground"
            : "rounded-md bg-muted p-baseline-3 text-muted-foreground overflow-x-auto",
          className
        )}
        {...props}
      >
        {children}
      </Component>
    );
  },
);
Code.displayName = "Code";

export { textVariants, headingVariants };