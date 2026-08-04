"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { apiGetOptional, type CubeInvite } from "@/lib/api";

// A dismissable "you have invites" banner. It only points at /invites (where you
// accept/decline) — it never acts on an invite itself, so dismissing it is purely
// cosmetic and the invites are still there next time.
export function InviteBanner() {
  const [count, setCount] = useState(0);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    apiGetOptional<CubeInvite[]>("/me/invites", 0).then((i) => setCount(i?.length ?? 0));
  }, []);

  if (count === 0 || dismissed) return null;

  return (
    <div className="invite-banner">
      <Link href="/invites" className="invite-banner-text">
        You have {count} new cube {count === 1 ? "invite" : "invites"} — click here to review and
        join.
      </Link>
      <button
        type="button"
        className="invite-banner-close"
        aria-label="Dismiss"
        onClick={() => setDismissed(true)}
      >
        ✕
      </button>
    </div>
  );
}
