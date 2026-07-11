"use client";

import Link from "next/link";
import { useEffect } from "react";

// Without this boundary a client throw renders only "Application error: a
// client-side exception has occurred", with the cause reachable solely from the
// browser console. Show the message.
export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <main className="container">
      <h1>Something went wrong</h1>
      <div className="card" style={{ marginTop: "1rem" }}>
        <p style={{ margin: 0, fontFamily: "ui-monospace, monospace", fontSize: "0.9rem" }}>
          {error.message || "Unknown error"}
        </p>
        {error.digest && (
          <p className="muted" style={{ fontSize: "0.8rem", marginBottom: 0 }}>
            digest {error.digest}
          </p>
        )}
      </div>
      <p style={{ marginTop: "1rem", display: "flex", gap: "1rem", alignItems: "center" }}>
        <button onClick={reset} className="button">
          Try again
        </button>
        <Link href="/">Back to the meta</Link>
      </p>
    </main>
  );
}
