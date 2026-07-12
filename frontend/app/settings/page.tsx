"use client";

import { useState } from "react";
import Link from "next/link";
import { apiPost } from "@/lib/api";
import { useSession } from "@/components/SessionProvider";
import { SignOutButton } from "@/components/SignOutButton";

export default function SettingsPage() {
  const { me } = useSession();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setMsg(null);
    if (next !== confirm) {
      setErr("The new passwords do not match.");
      return;
    }
    setBusy(true);
    try {
      // The backend replaces our session as part of this call, so we stay signed in.
      await apiPost(`/users/${me!.id}/password`, {
        current_password: current,
        new_password: next,
      });
      setMsg("Password changed.");
      setCurrent("");
      setNext("");
      setConfirm("");
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(false);
    }
  }

  if (me === undefined) return <main className="container">Loading…</main>;
  if (me === null) {
    return (
      <main className="container">
        <h1>Settings</h1>
        <p>
          You need to <Link href="/login">sign in</Link> to change your settings.
        </p>
      </main>
    );
  }

  return (
    <main className="container" style={{ maxWidth: 420 }}>
      <h1>Settings</h1>
      <p className="muted">
        Signed in as <Link href={`/users/${me.username}`}>@{me.username}</Link>.
      </p>

      <form onSubmit={submit} className="card">
        <h2 style={{ marginTop: 0 }}>Change password</h2>

        <label htmlFor="current">Current password</label>
        <input
          id="current"
          type="password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          autoComplete="current-password"
          required
        />

        <label htmlFor="next">New password</label>
        <input
          id="next"
          type="password"
          value={next}
          onChange={(e) => setNext(e.target.value)}
          autoComplete="new-password"
          minLength={8}
          required
        />

        <label htmlFor="confirm">Confirm new password</label>
        <input
          id="confirm"
          type="password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          autoComplete="new-password"
          minLength={8}
          required
        />
        <p className="muted" style={{ margin: "0.25rem 0 0", fontSize: "0.8rem" }}>
          At least 8 characters. Changing it signs out your other devices.
        </p>

        {err && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{err}</p>}
        {msg && <p className="muted" style={{ marginTop: "0.75rem" }}>{msg}</p>}

        <button className="button" style={{ marginTop: "1rem" }} disabled={busy}>
          {busy ? "Saving…" : "Change password"}
        </button>
      </form>

      {/* The nav has no room for a Sign out on a phone, so this is the only one there is. */}
      <div className="card" style={{ marginTop: "1rem" }}>
        <SignOutButton className="button" />
      </div>
    </main>
  );
}
