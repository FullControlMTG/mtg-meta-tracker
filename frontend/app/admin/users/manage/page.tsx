"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { apiGetNoStore, apiPost, apiPatch, apiDelete, type PublicUser } from "@/lib/api";
import { useSession } from "@/components/SessionProvider";
import { PasswordInput } from "@/components/PasswordInput";

const ROLES = ["user", "admin"] as const;

// Existing accounts: change roles, delete, reset passwords. Access is gated by
// app/admin/layout.tsx; useSession here only supplies me.id for the self-checks.
export default function ManageUsersPage() {
  const { me } = useSession();
  const [users, setUsers] = useState<PublicUser[]>([]);
  const [err, setErr] = useState<string | null>(null);

  function refresh() {
    apiGetNoStore<PublicUser[]>("/users")
      .then((us) => setUsers(us ?? []))
      .catch(() => setUsers([]));
  }
  useEffect(refresh, []);

  async function changeRole(u: PublicUser, next: string) {
    setErr(null);
    try {
      await apiPatch(`/users/${u.id}`, { role: next });
      refresh();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    }
  }

  async function remove(u: PublicUser) {
    if (!window.confirm(`Delete ${u.username}? Their decks are deleted too. This cannot be undone.`)) return;
    setErr(null);
    try {
      await apiDelete(`/users/${u.id}`);
      refresh();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    }
  }

  return (
    <main className="container" style={{ maxWidth: 820 }}>
      <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: "1rem", flexWrap: "wrap" }}>
        <h1>Manage users</h1>
        <Link href="/admin/users/add" className="button">
          Add user
        </Link>
      </div>

      {err && <p style={{ color: "var(--bad)" }}>{err}</p>}

      <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem", marginTop: "1rem" }}>
        {users.map((u) => (
          <div key={u.id} className="card">
            <div style={{ display: "flex", alignItems: "baseline", gap: "0.75rem", flexWrap: "wrap" }}>
              <strong style={{ fontSize: "1.05rem" }}>
                <Link href={`/users/${u.username}`}>{u.display_name}</Link>
              </strong>
              <span className="muted">@{u.username}</span>
              {u.id === me?.id && <span className="pill">you</span>}
            </div>

            <div style={{ display: "flex", gap: "0.75rem", marginTop: "0.5rem", flexWrap: "wrap", alignItems: "center" }}>
              <label htmlFor={`role-${u.id}`} className="muted" style={{ fontSize: "0.85rem" }}>
                Role
              </label>
              <select
                id={`role-${u.id}`}
                value={u.role}
                onChange={(e) => changeRole(u, e.target.value)}
                // Demoting yourself would lock you out of this page.
                disabled={u.id === me?.id}
                style={{ width: "auto" }}
              >
                {ROLES.map((r) => (
                  <option key={r} value={r}>
                    {r}
                  </option>
                ))}
              </select>

              {u.id !== me?.id && (
                <button type="button" className="button" onClick={() => remove(u)} style={{ background: "var(--bad, #b00)", color: "#fff" }}>
                  Delete
                </button>
              )}
            </div>

            {u.id !== me?.id && <ResetPassword user={u} onError={setErr} />}
          </div>
        ))}
      </div>
    </main>
  );
}

// Sets a new password for another user without knowing their current one — the way back
// in for someone who forgot theirs. Their sessions are dropped server-side.
function ResetPassword({ user, onError }: { user: PublicUser; onError: (msg: string | null) => void }) {
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState("");
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    onError(null);
    setMsg(null);
    try {
      await apiPost(`/users/${user.id}/password`, { new_password: password });
      setMsg(`New password set. Give it to ${user.username} — they are signed out until they use it.`);
      setPassword("");
      setOpen(false);
    } catch (e) {
      onError(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(false);
    }
  }

  if (!open) {
    return (
      <div style={{ marginTop: "0.5rem" }}>
        <button
          type="button"
          className="button"
          onClick={() => setOpen(true)}
          style={{ background: "var(--surface)", color: "var(--text)", border: "1px solid var(--border)" }}
        >
          Set new password
        </button>
        {msg && <p className="muted" style={{ marginTop: "0.5rem", fontSize: "0.85rem" }}>{msg}</p>}
      </div>
    );
  }

  return (
    <form onSubmit={submit} style={{ marginTop: "0.5rem", display: "flex", gap: "0.5rem", flexWrap: "wrap" }}>
      <PasswordInput value={password} onChange={setPassword} placeholder="New password" minLength={8} required style={{ width: 220 }} />
      <button className="button" disabled={busy}>
        {busy ? "Saving…" : "Save"}
      </button>
      <button
        type="button"
        className="button"
        onClick={() => {
          setOpen(false);
          setPassword("");
        }}
        style={{ background: "var(--surface)", color: "var(--text)", border: "1px solid var(--border)" }}
      >
        Cancel
      </button>
    </form>
  );
}
