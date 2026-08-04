"use client";

import { useState } from "react";
import type { PublicUser } from "@/lib/api";

// A typeahead for picking a user to invite. Shared by the create-cube form and the
// Manage-cube members panel so the two read the same. The parent passes the candidate
// list (already minus whoever's chosen/a member) and handles what a pick means —
// staging it, or inviting immediately.
export function UserSearch({
  users,
  onSelect,
  placeholder = "Search users…",
}: {
  users: PublicUser[];
  onSelect: (u: PublicUser) => void;
  placeholder?: string;
}) {
  const [query, setQuery] = useState("");
  const q = query.trim().toLowerCase();
  const matches = q
    ? users
        .filter(
          (u) =>
            u.display_name.toLowerCase().includes(q) || u.username.toLowerCase().includes(q),
        )
        .slice(0, 6)
    : [];

  return (
    <div className="user-search">
      <input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={placeholder}
        aria-label={placeholder}
      />
      {matches.length > 0 && (
        <div className="user-search-results">
          {matches.map((u) => (
            <button
              key={u.id}
              type="button"
              className="user-search-option"
              onClick={() => {
                onSelect(u);
                setQuery("");
              }}
            >
              {u.display_name} <span className="muted">@{u.username}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// One picked/enrolled user with a remove control. Shared so the create form's staged
// invitees and the members panel's rows look identical.
export function MemberRow({ user, onRemove }: { user: PublicUser; onRemove: () => void }) {
  return (
    <div className="member-row">
      <span>
        {user.display_name} <span className="muted">@{user.username}</span>
      </span>
      <button type="button" className="ghost-button" onClick={onRemove} aria-label={`Remove ${user.display_name}`}>
        Remove
      </button>
    </div>
  );
}
