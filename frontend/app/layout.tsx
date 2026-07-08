import "./globals.css";
import type { Metadata } from "next";
import type { ReactNode } from "react";
import { Nav } from "@/components/Nav";
import { SessionProvider } from "@/components/SessionProvider";

export const metadata: Metadata = {
  title: "MTG Meta Tracker",
  description: "Meta analysis for your local cube playgroup.",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>
        <SessionProvider>
          <Nav />
          {children}
        </SessionProvider>
      </body>
    </html>
  );
}
