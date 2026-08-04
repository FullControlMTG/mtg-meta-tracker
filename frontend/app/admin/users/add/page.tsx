"use client";

import { useState } from "react";
import { apiPost } from "@/lib/api";
import { PasswordInput } from "@/components/PasswordInput";

const ROLES = ["user", "admin"] as const;

// Create an account. There is no public signup, so this is how everyone but the first
// admin gets in. Access is gated by app/admin/layout.tsx.
export default function AddUserPage() {
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<string>("user");
  const [err, setErr] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    setMsg(null);
    try {
      await apiPost("/admin/users", { username, display_name: displayName, email, password, role });
      setMsg(`Created ${username}. Give them the password — they can change it under Settings.`);
      setUsername("");
      setDisplayName("");
      setEmail("");
      setPassword("");
      setRole("user");
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="container" style={{ maxWidth: 560 }}>
      <h1>Add user</h1>
      <form onSubmit={create} className="card">
        <label htmlFor="username">Username</label>
        <input id="username" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" required />

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
        <PasswordInput id="password" value={password} onChange={setPassword} minLength={8} required />
        <p className="muted" style={{ margin: "0.25rem 0 0", fontSize: "0.8rem" }}>
          At least 8 characters. Reveal it to hand it over; they can change it later.
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
    </main>
  );
}
