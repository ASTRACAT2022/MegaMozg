import { NextResponse } from "next/server";

import { loadPanelData } from "@/lib/api";
import { buildAlerts } from "@/lib/alerts";

export async function GET() {
  const state = await loadPanelData();
  return NextResponse.json({
    ...state,
    alerts: buildAlerts(state),
  });
}

