/**
 * Typography system inspired by Atlas - crafted specifically for fleetd's device management interface
 *
 * Key principles:
 * - Baseline grid alignment (6px unit)
 * - Optical adjustments for readability at each size
 * - T-shirt sizing for contextual flexibility
 * - Optimized for scanning technical data
 */

export const typography = {
  // Extra small - for secondary metadata, timestamps
  xs: {
    fontSize: "0.75rem", // 12px
    lineHeight: "1.125rem", // 18px (3 × 6px grid)
    letterSpacing: "0.025em", // Slight spacing for legibility at small size
  },

  // Small - for labels, captions, table headers
  sm: {
    fontSize: "0.875rem", // 14px
    lineHeight: "1.375rem", // 22px (approx 3.67 × 6px, rounded for optical balance)
    letterSpacing: "0.01em",
  },

  // Base - primary body text, optimized for readability
  base: {
    fontSize: "1rem", // 16px (bumped up 1px from typical 15px for better readability)
    lineHeight: "1.625rem", // 26px (approx 4.33 × 6px for comfortable reading)
    letterSpacing: "0",
  },

  // Medium - for emphasized content, card titles
  md: {
    fontSize: "1.125rem", // 18px
    lineHeight: "1.875rem", // 30px (5 × 6px grid)
    letterSpacing: "-0.01em", // Tighter for larger text
  },

  // Large - for section headings, prominent stats
  lg: {
    fontSize: "1.5rem", // 24px
    lineHeight: "2.25rem", // 36px (6 × 6px grid)
    letterSpacing: "-0.02em",
  },

  // Extra large - for hero numbers, dashboard metrics
  xl: {
    fontSize: "2rem", // 32px
    lineHeight: "2.75rem", // 44px (approx 7.33 × 6px, optimized for visual balance)
    letterSpacing: "-0.03em",
  },

  // Display - for major headings
  display: {
    fontSize: "2.5rem", // 40px
    lineHeight: "3rem", // 48px (8 × 6px grid)
    letterSpacing: "-0.04em",
  },
} as const;

export type TypographySize = keyof typeof typography;

/**
 * Utility function to get typography styles as CSS properties
 */
export function getTypographyStyles(size: TypographySize) {
  return typography[size];
}

/**
 * Spacing system based on 6px baseline grid
 */
export const spacing = {
  baseline: {
    1: "0.375rem", // 6px
    2: "0.75rem", // 12px
    3: "1.125rem", // 18px
    4: "1.5rem", // 24px
    5: "1.875rem", // 30px
    6: "2.25rem", // 36px
    7: "2.625rem", // 42px
    8: "3rem", // 48px
  },
} as const;

export type BaselineSpacing = keyof typeof spacing.baseline;
