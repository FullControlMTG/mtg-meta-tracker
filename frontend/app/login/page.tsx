"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { apiPost, type PublicUser } from "@/lib/api";

export default function LoginPage() {
  const router = useRouter();
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      await apiPost<PublicUser>("/auth/login", { login, password });
      router.push("/");
      router.refresh();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="container" style={{ maxWidth: 420 }}>
      <h1>Sign in</h1>
      <form onSubmit={submit} className="card">
        <label htmlFor="login">Username or email</label>
        <input
          id="login"
          value={login}
          onChange={(e) => setLogin(e.target.value)}
          autoComplete="username"
          required
        />
        <label htmlFor="password">Password</label>
        <input
          id="password"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="current-password"
          required
        />
        {err && (
          <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{err}</p>
        )}
        <button className="button" style={{ marginTop: "1rem" }} disabled={busy}>
          {busy ? "Signing in…" : "Sign in"}
        </button>
      </form>
      <p className="muted" style={{ marginTop: "1rem", fontSize: "0.85rem" }}>
        Accounts are invite-only. Ask an admin for an invite link.
      </p>
    </main>
  );
}
