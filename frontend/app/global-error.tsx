"use client";

// Last-resort boundary: a throw in the root layout itself never reaches app/error.tsx,
// and global-error replaces the whole document, so it must ship its own html/body.
export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <html lang="en">
      <body
        style={{
          fontFamily: "system-ui, sans-serif",
          margin: 0,
          padding: "3rem 1.5rem",
          lineHeight: 1.5,
        }}
      >
        <main style={{ maxWidth: 640, margin: "0 auto" }}>
          <h1>Something went wrong</h1>
          <p style={{ fontFamily: "ui-monospace, monospace", fontSize: "0.9rem" }}>
            {error.message || "Unknown error"}
          </p>
          <button onClick={reset}>Try again</button>
        </main>
      </body>
    </html>
  );
}
