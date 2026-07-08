"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import type { ReactNode } from "react";
import { useRouter } from "next/navigation";
import { apiGetOptional, type PublicUser } from "@/lib/api";

// undefined = still loading, null = logged out, object = logged in
type Me = PublicUser | null | undefined;

interface SessionValue {
  me: Me;
  // refresh re-reads the session and re-renders server components in this tab,
  // then notifies the user's other tabs. Call it after any notable event
  // (login, logout, deck/cube create/delete).
  refresh: () => Promise<void>;
  // setMe allows an optimistic update (e.g. from a login response) so the nav
  // flips immediately without waiting on the /auth/me round-trip.
  setMe: (me: Me) => void;
}

const SessionContext = createContext<SessionValue | null>(null);

const CHANNEL = "session";

export function SessionProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [me, setMe] = useState<Me>(undefined);
  const channelRef = useRef<BroadcastChannel | null>(null);

  // applyRefresh re-fetches the session and revalidates server components in
  // this tab. It does NOT broadcast — used both on mount and when reacting to
  // another tab's broadcast, so it must not echo back and cause a loop.
  const applyRefresh = useCallback(async () => {
    const next = await apiGetOptional<PublicUser>("/auth/me");
    setMe(next);
    router.refresh();
  }, [router]);

  // Load the session once on mount, and subscribe to cross-tab refreshes.
  useEffect(() => {
    void applyRefresh();

    if (typeof BroadcastChannel === "undefined") return;
    const channel = new BroadcastChannel(CHANNEL);
    channelRef.current = channel;
    channel.onmessage = () => {
      void applyRefresh();
    };
    return () => {
      channel.close();
      channelRef.current = null;
    };
  }, [applyRefresh]);

  const refresh = useCallback(async () => {
    await applyRefresh();
    channelRef.current?.postMessage("refresh");
  }, [applyRefresh]);

  return (
    <SessionContext.Provider value={{ me, refresh, setMe }}>
      {children}
    </SessionContext.Provider>
  );
}

export function useSession(): SessionValue {
  const ctx = useContext(SessionContext);
  if (!ctx) {
    throw new Error("useSession must be used within a SessionProvider");
  }
  return ctx;
}
