import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "./globals.css";
import { Providers } from "@/components/providers";
import { AppHeader } from "@/components/app-header";
import { CommandPalette } from "@/components/command-palette";

const inter = Inter({ subsets: ["latin"] });

export const metadata: Metadata = {
  title: "fleetd",
  description: "Fleet device management and monitoring dashboard",
  keywords: ["fleet", "device", "management", "monitoring", "iot"],
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className={inter.className}>
        <Providers>
          <div className="min-h-screen bg-background">
            <AppHeader />
            <CommandPalette />
            {children}
          </div>
        </Providers>
      </body>
    </html>
  );
}
