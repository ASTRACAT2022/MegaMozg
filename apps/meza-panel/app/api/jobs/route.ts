import { NextResponse } from "next/server";

import { coreJson } from "@/lib/server-core";

type CreateJobBody = {
  type: string;
  target_selector: string;
  strategy: string;
  payload?: Record<string, unknown>;
  created_by?: string;
  summary?: string;
};

export async function GET() {
  try {
    const jobs = await coreJson("/api/v1/jobs");
    return NextResponse.json(jobs);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load jobs" },
      { status: 500 }
    );
  }
}

export async function POST(request: Request) {
  try {
    const body = (await request.json()) as CreateJobBody;
    const created = await coreJson("/api/v1/jobs", {
      method: "POST",
      body: JSON.stringify(body),
    });
    return NextResponse.json(created);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to create job" },
      { status: 500 }
    );
  }
}

