"use client";

import { InviteList } from "@/components/InviteList";

// Reached from the account menu. The full accept/decline view for cube invites.
export default function InvitesPage() {
  return (
    <main className="container" style={{ maxWidth: 640 }}>
      <h1>Invites</h1>
      <p className="muted" style={{ marginTop: "-0.5rem" }}>
        Cubes you&apos;ve been invited to. Accepting adds you as a member.
      </p>
      <div className="card" style={{ marginTop: "1rem" }}>
        <InviteList />
      </div>
    </main>
  );
}
