import { NextResponse } from "next/server";

import type { AIPlanResult, JobItem } from "@/lib/api";
import { summarizeJob } from "@/lib/alerts";
import { coreJson } from "@/lib/server-core";

type ChatRequestBody = {
  session_id?: string;
  message: string;
  auto_execute?: boolean;
};

type PlanAndCreateResponse = {
  plan: AIPlanResult;
  job: JobItem;
};

export async function POST(request: Request) {
  try {
    const body = (await request.json()) as ChatRequestBody;
    if (!body.message || !body.message.trim()) {
      return NextResponse.json({ error: "message is required" }, { status: 400 });
    }

    const planResult = await coreJson<PlanAndCreateResponse>("/api/v1/ai/plan-and-create", {
      method: "POST",
      body: JSON.stringify({ prompt: body.message }),
    });

    let job = planResult.job;
    let executed = false;
    let note = "";

    if (body.auto_execute) {
      if (job.requires_approval) {
        note = "Задача создана, но требует подтверждения перед запуском.";
      } else {
        job = await coreJson<JobItem>(`/api/v1/jobs/${job.id}/start`, {
          method: "POST",
          body: JSON.stringify({ actor: "ai-copilot" }),
        });
        executed = true;
        note = "Задача была автоматически запущена.";
      }
    }

    const assistantMessage =
      `Понял. Сформировал задачу ${job.id}. ${summarizeJob(job)} ` +
      `${note || "При необходимости могу запустить после подтверждения."}`;

    return NextResponse.json({
      session_id: body.session_id ?? "default",
      plan: planResult.plan,
      job,
      executed,
      assistant_message: assistantMessage,
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "agent request failed" },
      { status: 500 }
    );
  }
}

