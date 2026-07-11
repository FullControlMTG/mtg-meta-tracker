"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { apiGetNoStore, apiPost, apiPatch, apiDelete, type PublicUser } from "@/lib/api";
import { useSession } from "@/components/SessionProvider";

const ROLES = ["user", "admin"] as const;

export default function AdminUsersPage() {
  const { me } = useSession();
  const [users, setUsers] = useState<PublicUser[]>([]);

  // New-user form.
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<string>("user");
  const [err, setErr] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function refresh() {
    apiGetNoStore<PublicUser[]>("/users")
      .then((us) => setUsers(us ?? []))
      .catch(() => setUsers([]));
  }

  useEffect(refresh, []);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    setMsg(null);
    try {
      await apiPost<PublicUser>("/admin/users", {
        username,
        display_name: displayName,
        email,
        password,
        role,
      });
      setMsg(`Created ${username}. Give them the password — they can change it under Settings.`);
      setUsername("");
      setDisplayName("");
      setEmail("");
      setPassword("");
      setRole("user");
      refresh();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(false);
    }
  }

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
    if (!window.confirm(`Delete ${u.username}? Their decks are deleted too. This cannot be undone.`)) {
      return;
    }
    setErr(null);
    try {
      await apiDelete(`/users/${u.id}`);
      refresh();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    }
  }

  if (me === undefined) return <main className="container">Loading…</main>;
  if (me === null || me.role !== "admin") {
    return (
      <main className="container">
        <h1>User management</h1>
        <p>
          You are not authorized to view this page. <Link href="/">Go home</Link>.
        </p>
      </main>
    );
  }

  return (
    <main className="container" style={{ maxWidth: 820 }}>
      <h1>User management</h1>

      <form onSubmit={create} className="card">
        <h2 style={{ marginTop: 0 }}>New user</h2>

        <label htmlFor="username">Username</label>
        <input
          id="username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoComplete="off"
          required
        />

        <label htmlFor="display">Display name (optional)</label>
        <input
          id="display"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
          placeholder="Defaults to the username"
        />

        <label htmlFor="email">Email (optional)</label>
        <input id="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />

        <label htmlFor="password">Password</label>
        <input
          id="password"
          type="text"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="off"
          minLength={8}
          required
        />
        <p className="muted" style={{ margin: "0.25rem 0 0", fontSize: "0.8rem" }}>
          At least 8 characters. Shown in the clear so you can hand it over; they can change it later.
        </p>

        <label htmlFor="role">Role</label>
        <select id="role" value={role} onChange={(e) => setRole(e.target.value)}>
          {ROLES.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </select>

        {err && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{err}</p>}
        {msg && <p className="muted" style={{ marginTop: "0.75rem" }}>{msg}</p>}

        <button className="button" style={{ marginTop: "1rem" }} disabled={busy}>
          {busy ? "Creating…" : "Create user"}
        </button>
      </form>

      <h2>Users</h2>
      <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
        {users.map((u) => (
          <div key={u.id} className="card">
            <div style={{ display: "flex", alignItems: "baseline", gap: "0.75rem", flexWrap: "wrap" }}>
              <strong style={{ fontSize: "1.05rem" }}>
                <Link href={`/users/${u.username}`}>{u.display_name}</Link>
              </strong>
              <span className="muted">@{u.username}</span>
              {u.id === me.id && <span className="pill">you</span>}
            </div>

            <div
              style={{
                display: "flex",
                gap: "0.75rem",
                marginTop: "0.5rem",
                flexWrap: "wrap",
                alignItems: "center",
              }}
            >
              <label htmlFor={`role-${u.id}`} className="muted" style={{ fontSize: "0.85rem" }}>
                Role
              </label>
              <select
                id={`role-${u.id}`}
                value={u.role}
                onChange={(e) => changeRole(u, e.target.value)}
                // Demoting yourself would lock you out of this page.
                disabled={u.id === me.id}
                style={{ width: "auto" }}
              >
                {ROLES.map((r) => (
                  <option key={r} value={r}>
                    {r}
                  </option>
                ))}
              </select>

              {u.id !== me.id && (
                <button
                  type="button"
                  className="button"
                  onClick={() => remove(u)}
                  style={{ background: "var(--bad, #b00)", color: "#fff" }}
                >
                  Delete
                </button>
              )}
            </div>

            {u.id !== me.id && <ResetPassword user={u} onError={setErr} />}
          </div>
        ))}
      </div>
    </main>
  );
}

// Sets a new password for another user without knowing their current one — the
// way back in for someone who has forgotten theirs. Their existing sessions are
// dropped server-side, so they must sign in again with what you give them.
function ResetPassword({
  user,
  onError,
}: {
  user: PublicUser;
  onError: (msg: string | null) => void;
}) {
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
      <input
        type="text"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        placeholder="New password"
        autoComplete="off"
        minLength={8}
        required
        style={{ width: 220 }}
      />
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
