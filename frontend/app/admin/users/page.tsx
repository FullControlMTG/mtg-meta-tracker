import { redirect } from "next/navigation";

// /admin/users has no page of its own; its work is split across manage and add.
export default function AdminUsersIndex() {
  redirect("/admin/users/manage");
}
