import { NextResponse } from "next/server";

import { coreJson } from "@/lib/server-core";

export async function DELETE(_: Request, context: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await context.params;
    const deleted = await coreJson(`/api/v1/jobs/${id}`, {
      method: "DELETE",
      body: JSON.stringify({ actor: "panel-operator" }),
    });
    return NextResponse.json(deleted);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to delete job" },
      { status: 500 }
    );
  }
}
