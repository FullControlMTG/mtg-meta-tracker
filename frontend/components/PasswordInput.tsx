"use client";

import { useEffect, useState } from "react";

// A masked password field with a deliberate reveal. Used where an admin types a password
// *for someone else* and needs to read it back to hand it over — masked by default so it
// isn't sitting in the clear on a shared screen, revealed only when they ask for it.
export function PasswordInput({
  id,
  value,
  onChange,
  placeholder,
  autoComplete = "new-password",
  minLength,
  required,
  style,
}: {
  id?: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  autoComplete?: string;
  minLength?: number;
  required?: boolean;
  style?: React.CSSProperties;
}) {
  const [shown, setShown] = useState(false);

  // A cleared field means the form was submitted or cancelled; don't leave the next
  // password revealed because the last one was.
  useEffect(() => {
    if (value === "") setShown(false);
  }, [value]);

  return (
    <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
      <input
        id={id}
        type={shown ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        autoComplete={autoComplete}
        minLength={minLength}
        required={required}
        style={style}
      />
      <button
        // Without this it defaults to submit and revealing would send the form.
        type="button"
        className="button"
        onClick={() => setShown((s) => !s)}
        aria-label={shown ? "Hide password" : "Show password"}
        style={{
          background: "var(--surface)",
          color: "var(--text)",
          border: "1px solid var(--border)",
        }}
      >
        {shown ? "Hide" : "Show"}
      </button>
    </div>
  );
}
