"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
  apiGet,
  apiPost,
  apiPatch,
  apiDelete,
  type CubeView,
} from "@/lib/api";
import { useSession } from "@/components/SessionProvider";

function fmtDate(s?: string): string {
  if (!s) return "never";
  const d = new Date(s);
  return isNaN(d.getTime()) ? "—" : d.toLocaleString();
}

export default function AdminCubesPage() {
  const { me, refresh: refreshSession } = useSession();
  const [cubes, setCubes] = useState<CubeView[]>([]);

  // Form state (create when editingId is null, otherwise update).
  const [editingId, setEditingId] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [moxfieldUrl, setMoxfieldUrl] = useState("");
  const [description, setDescription] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function refresh() {
    apiGet<CubeView[]>("/cubes")
      .then(setCubes)
      .catch(() => setCubes([]));
  }

  useEffect(() => {
    refresh();
  }, []);

  function resetForm() {
    setEditingId(null);
    setName("");
    setMoxfieldUrl("");
    setDescription("");
    setErr(null);
  }

  function startEdit(cv: CubeView) {
    setEditingId(cv.cube.id);
    setName(cv.cube.name);
    setMoxfieldUrl(cv.cube.moxfield_public_id ?? "");
    setDescription(cv.cube.description ?? "");
    setErr(null);
    window.scrollTo({ top: 0, behavior: "smooth" });
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      const body = { name, moxfield_url: moxfieldUrl, description };
      if (editingId) {
        await apiPatch<CubeView>(`/admin/cubes/${editingId}`, body);
      } else {
        await apiPost<CubeView>("/admin/cubes", body);
      }
      resetForm();
      refresh();
      // Also update the nav, the public server-rendered cube pages, and the
      // user's other tabs — not just this page's local list.
      void refreshSession();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(false);
    }
  }

  async function sync(id: string) {
    try {
      await apiPost(`/admin/cubes/${id}/sync`);
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    }
  }

  async function remove(cv: CubeView) {
    if (!window.confirm(`Delete cube "${cv.cube.name}"? This cannot be undone.`)) return;
    try {
      await apiDelete(`/admin/cubes/${cv.cube.id}`);
      if (editingId === cv.cube.id) resetForm();
      refresh();
      void refreshSession();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    }
  }

  if (me === undefined) return <main className="container">Loading…</main>;
  if (me === null || me.role !== "admin") {
    return (
      <main className="container">
        <h1>Cube management</h1>
        <p>
          You are not authorized to view this page. <Link href="/">Go home</Link>.
        </p>
      </main>
    );
  }

  return (
    <main className="container" style={{ maxWidth: 820 }}>
      <h1>Cube management</h1>

      <form onSubmit={submit} className="card">
        <h2 style={{ marginTop: 0 }}>{editingId ? "Edit cube" : "New cube"}</h2>

        <label htmlFor="name">Name</label>
        <input id="name" value={name} onChange={(e) => setName(e.target.value)} required />

        <label htmlFor="mox">Moxfield URL</label>
        <input
          id="mox"
          value={moxfieldUrl}
          onChange={(e) => setMoxfieldUrl(e.target.value)}
          placeholder="https://www.moxfield.com/decks/…"
        />

        <label htmlFor="desc">Description (optional)</label>
        <textarea
          id="desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          rows={3}
          style={{ resize: "vertical" }}
        />

        {err && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{err}</p>}

        <div style={{ display: "flex", gap: "0.75rem", marginTop: "1rem" }}>
          <button className="button" disabled={busy}>
            {busy ? "Saving…" : editingId ? "Save changes" : "Create cube"}
          </button>
          {editingId && (
            <button
              type="button"
              className="button"
              onClick={resetForm}
              style={{ background: "var(--surface)", color: "var(--text)", border: "1px solid var(--border)" }}
            >
              Cancel
            </button>
          )}
        </div>
      </form>

      <h2>Cubes</h2>
      {cubes.length === 0 && <p className="muted">No cubes yet.</p>}
      <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
        {cubes.map((cv) => (
          <div key={cv.cube.id} className="card">
            <div style={{ display: "flex", alignItems: "baseline", gap: "0.75rem", flexWrap: "wrap" }}>
              <strong style={{ fontSize: "1.05rem" }}>{cv.cube.name}</strong>
              <span className="muted">{cv.card_count} cards</span>
              <span className="muted">· synced {fmtDate(cv.cube.last_synced_at)}</span>
            </div>
            {cv.cube.moxfield_public_id && (
              <p className="muted" style={{ margin: "0.25rem 0", fontSize: "0.85rem" }}>
                Moxfield: {cv.cube.moxfield_public_id}
              </p>
            )}
            {cv.cube.description && <p style={{ margin: "0.25rem 0" }}>{cv.cube.description}</p>}
            <div style={{ display: "flex", gap: "0.75rem", marginTop: "0.5rem", flexWrap: "wrap" }}>
              <button type="button" className="button" onClick={() => startEdit(cv)}>
                Edit
              </button>
              {cv.cube.moxfield_public_id && (
                <button
                  type="button"
                  className="button"
                  onClick={() => sync(cv.cube.id)}
                  style={{ background: "var(--surface)", color: "var(--text)", border: "1px solid var(--border)" }}
                >
                  Sync now
                </button>
              )}
              <button
                type="button"
                className="button"
                onClick={() => remove(cv)}
                style={{ background: "var(--bad, #b00)", color: "#fff" }}
              >
                Delete
              </button>
            </div>
          </div>
        ))}
      </div>
    </main>
  );
}
