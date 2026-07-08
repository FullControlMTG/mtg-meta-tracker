"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import {
  apiGet,
  apiGetNoStore,
  apiPost,
  apiPatch,
  apiDelete,
  type CubeView,
  type CubeSyncStatus,
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
  const [cardList, setCardList] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  // Live sync progress, keyed by cube id, polled from /admin/cubes/{id}/sync-status.
  const [progress, setProgress] = useState<Record<string, CubeSyncStatus>>({});
  // Active poll timers, keyed by cube id, so we can stop them on completion/unmount.
  const timers = useRef<Record<string, ReturnType<typeof setInterval>>>({});

  function refresh() {
    apiGet<CubeView[]>("/cubes")
      .then(setCubes)
      .catch(() => setCubes([]));
  }

  useEffect(() => {
    refresh();
    // Stop any in-flight polls when the page unmounts.
    return () => {
      Object.values(timers.current).forEach(clearInterval);
      timers.current = {};
    };
  }, []);

  // A sync is considered active (button disabled, panel shown) in these phases.
  function isActive(s?: CubeSyncStatus): boolean {
    return s?.status === "queued" || s?.status === "resolving" || s?.status === "downloading";
  }

  function stopPolling(id: string) {
    const t = timers.current[id];
    if (t) {
      clearInterval(t);
      delete timers.current[id];
    }
  }

  function resetForm() {
    setEditingId(null);
    setName("");
    setMoxfieldUrl("");
    setDescription("");
    setCardList("");
    setErr(null);
  }

  function startEdit(cv: CubeView) {
    setEditingId(cv.cube.id);
    setName(cv.cube.name);
    setMoxfieldUrl(cv.cube.moxfield_public_id ?? "");
    setDescription(cv.cube.description ?? "");
    setCardList(cv.cube.card_list ?? "");
    setErr(null);
    window.scrollTo({ top: 0, behavior: "smooth" });
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      const body = { name, moxfield_url: moxfieldUrl, description, card_list: cardList };
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
    setErr(null);
    // Optimistic "queued" so the panel appears instantly, before the first poll.
    setProgress((p) => ({ ...p, [id]: { status: "queued" } }));
    try {
      // Runs as a background job (returns 202 before it finishes): it re-resolves
      // the pool against Scryfall and downloads any missing card art. We then poll
      // /sync-status to show live progress through to completion.
      await apiPost(`/admin/cubes/${id}/sync`);
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
      setProgress((p) => {
        const next = { ...p };
        delete next[id];
        return next;
      });
      return;
    }
    stopPolling(id); // in case a previous sync for this cube is still polling
    const poll = async () => {
      try {
        const s = await apiGetNoStore<CubeSyncStatus>(`/admin/cubes/${id}/sync-status`);
        setProgress((p) => ({ ...p, [id]: s }));
        if (s.status === "done" || s.status === "failed" || s.status === "none") {
          stopPolling(id);
          // Pick up the updated card count and synced time.
          refresh();
        }
      } catch {
        // Transient error — keep polling; the interval will try again.
      }
    };
    timers.current[id] = setInterval(poll, 1500);
    void poll(); // fire immediately rather than waiting out the first interval
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

        <label htmlFor="mox">Moxfield URL (optional, for reference)</label>
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

        <label htmlFor="cards">Card list</label>
        <textarea
          id="cards"
          value={cardList}
          onChange={(e) => setCardList(e.target.value)}
          rows={12}
          placeholder={"One card per line, standard decklist format:\n1 Sol Ring\n1 Lightning Bolt\nMana Crypt"}
          style={{ resize: "vertical", fontFamily: "monospace" }}
        />
        <p className="muted" style={{ margin: "0.25rem 0 0", fontSize: "0.8rem" }}>
          The pool is built from this list. Saving re-resolves cards against Scryfall and
          recomputes analytics.
        </p>

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
                Moxfield:{" "}
                <a
                  href={`https://www.moxfield.com/decks/${cv.cube.moxfield_public_id}`}
                  target="_blank"
                  rel="noreferrer"
                >
                  {cv.cube.moxfield_public_id}
                </a>
              </p>
            )}
            {cv.cube.description && <p style={{ margin: "0.25rem 0" }}>{cv.cube.description}</p>}
            <div style={{ display: "flex", gap: "0.75rem", marginTop: "0.5rem", flexWrap: "wrap" }}>
              <button type="button" className="button" onClick={() => startEdit(cv)}>
                Edit
              </button>
              {cv.cube.card_list && (
                <button
                  type="button"
                  className="button"
                  onClick={() => sync(cv.cube.id)}
                  disabled={isActive(progress[cv.cube.id])}
                  style={{ background: "var(--surface)", color: "var(--text)", border: "1px solid var(--border)" }}
                >
                  {isActive(progress[cv.cube.id]) ? "Syncing…" : "Sync Scryfall images"}
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
            <SyncProgress status={progress[cv.cube.id]} />
          </div>
        ))}
      </div>
    </main>
  );
}

// SyncProgress renders the live state of a "Sync Scryfall images" run for one
// cube: a phase line + progress bar while active, a green summary on success,
// and a red message on failure. Renders nothing when idle/never-synced.
function SyncProgress({ status }: { status?: CubeSyncStatus }) {
  if (!status || status.status === "none") return null;

  const barStyle = { marginTop: "0.5rem", fontSize: "0.85rem" } as const;

  if (status.status === "failed") {
    return (
      <p style={{ color: "var(--bad)", ...barStyle }}>
        ✗ Sync failed{status.error ? `: ${status.error}` : ""}
      </p>
    );
  }

  if (status.status === "done") {
    return (
      <p style={{ color: "var(--good, #0a0)", ...barStyle }}>
        ✓ Synced {status.cards_total ?? 0} cards · {status.images_done ?? 0} images
        {status.images_failed ? ` · ${status.images_failed} failed` : ""}
      </p>
    );
  }

  // Active: queued | resolving | downloading.
  const total = status.images_total ?? 0;
  const done = status.images_done ?? 0;
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;
  const label =
    status.status === "downloading"
      ? `Downloading images ${done} / ${total}`
      : status.status === "resolving"
        ? "Resolving cards…"
        : "Queued…";

  return (
    <div style={barStyle}>
      <span className="muted">{label}</span>
      {status.status === "downloading" && total > 0 && (
        <div
          style={{
            marginTop: "0.35rem",
            height: "0.5rem",
            borderRadius: "0.25rem",
            background: "var(--surface)",
            border: "1px solid var(--border)",
            overflow: "hidden",
          }}
        >
          <div
            style={{
              width: `${pct}%`,
              height: "100%",
              background: "var(--good, #0a0)",
              transition: "width 0.3s ease",
            }}
          />
        </div>
      )}
    </div>
  );
}
