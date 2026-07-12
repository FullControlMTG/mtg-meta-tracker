"use client";

import { apiPost } from "@/lib/api";
import { useSession } from "@/components/SessionProvider";

// Sign out lives in two places: the nav on a wide screen, and the settings page — which
// is where the mobile nav sends you, having no room for the link itself.
export function useSignOut(): () => Promise<void> {
  const { refresh } = useSession();
  return async () => {
    try {
      await apiPost("/auth/logout");
    } catch {
      // ignore — refresh clears the client view regardless
    }
    // Re-reads the (now cleared) session, revalidates server pages, and
    // broadcasts the logout to the user's other tabs — no hard reload.
    await refresh();
  };
}

export function SignOutButton({
  className,
  style,
}: {
  className?: string;
  style?: React.CSSProperties;
}) {
  const signOut = useSignOut();
  return (
    <button onClick={signOut} className={className} style={style}>
      Sign out
    </button>
  );
}
