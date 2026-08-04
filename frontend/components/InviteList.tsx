"use client";

import { useEffect, useState } from "react";
import { apiGetOptional, apiPost, type CubeInvite } from "@/lib/api";

// The caller's pending cube invites, with accept/decline. Shared by the dashboard and
// the dedicated /invites page. onChange fires after a response so a host can refresh a
// count or a cube list.
export function InviteList({ onChange }: { onChange?: () => void }) {
  const [invites, setInvites] = useState<CubeInvite[] | null>(null);
  const [err, setErr] = useState<string | null>(null);

  function refresh() {
    apiGetOptional<CubeInvite[]>("/me/invites", 0).then((i) => setInvites(i ?? []));
  }
  useEffect(refresh, []);

  async function respond(invite: CubeInvite, accept: boolean) {
    setErr(null);
    try {
      await apiPost(`/invites/${invite.id}/${accept ? "accept" : "decline"}`);
      refresh();
      onChange?.();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    }
  }

  if (invites === null) return <p className="muted">Loading…</p>;
  if (invites.length === 0) return <p className="muted">No pending invites.</p>;

  return (
    <div>
      {err && <p style={{ color: "var(--bad)" }}>{err}</p>}
      {invites.map((i) => (
        <div
          key={i.id}
          style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "0.75rem", padding: "0.35rem 0", flexWrap: "wrap" }}
        >
          <span>
            <strong>{i.cube_name}</strong>
            {i.invited_by && <span className="muted"> · invited by {i.invited_by}</span>}
          </span>
          <span style={{ display: "flex", gap: "0.5rem" }}>
            <button type="button" className="button" onClick={() => respond(i, true)}>
              Accept
            </button>
            <button type="button" className="ghost-button" onClick={() => respond(i, false)}>
              Decline
            </button>
          </span>
        </div>
      ))}
    </div>
  );
}
