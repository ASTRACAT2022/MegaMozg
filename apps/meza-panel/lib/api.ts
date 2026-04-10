export type DashboardSummary = {
  total_nodes: number;
  online_nodes: number;
  degraded_nodes: number;
  running_jobs: number;
  awaiting_approvals: number;
  audit_events: number;
  average_cpu: number;
  average_ram: number;
  average_disk: number;
};

type NodeMetrics = {
  cpu_percent: number;
  ram_percent: number;
  disk_percent: number;
  network_kbps: number;
};

export type NodeItem = {
  id: string;
  name: string;
  region: string;
  tags: string[];
  status: string;
  metrics: NodeMetrics;
  last_seen_at: string;
};

export type JobItem = {
  id: string;
  type: string;
  target_selector: string;
  strategy: string;
  status: string;
  created_by: string;
  requires_approval: boolean;
  summary: string;
  matched_nodes: string[];
};

export type AuditItem = {
  id: string;
  actor: string;
  action: string;
  resource_type: string;
  resource_id: string;
  message: string;
  created_at: string;
};

export type ApiList<T> = { items: T[] };

export type AIPlanResult = {
  prompt: string;
  provider: string;
  summary: string;
  intent: string;
  requires_review: boolean;
  job_preview: {
    type: string;
    target_selector: string;
    strategy: string;
  };
};

export type PanelData = {
  dashboard: DashboardSummary;
  nodes: NodeItem[];
  jobs: JobItem[];
  audit: AuditItem[];
  apiReachable: boolean;
  samplePlan: AIPlanResult;
};

const fallbackData: PanelData = {
  dashboard: {
    total_nodes: 3,
    online_nodes: 2,
    degraded_nodes: 1,
    running_jobs: 0,
    awaiting_approvals: 0,
    audit_events: 1,
    average_cpu: 62,
    average_ram: 60,
    average_disk: 61,
  },
  nodes: [
    {
      id: "node-argentina-17",
      name: "argentina-17",
      region: "south-america",
      tags: ["docker", "prod", "argentina"],
      status: "online",
      metrics: { cpu_percent: 37, ram_percent: 44, disk_percent: 52, network_kbps: 940 },
      last_seen_at: new Date().toISOString(),
    },
    {
      id: "node-moscow-01",
      name: "moscow-01",
      region: "ru-central",
      tags: ["frontend", "staging", "moscow"],
      status: "online",
      metrics: { cpu_percent: 61, ram_percent: 58, disk_percent: 48, network_kbps: 670 },
      last_seen_at: new Date().toISOString(),
    },
    {
      id: "node-berlin-05",
      name: "berlin-05",
      region: "eu-central",
      tags: ["db", "prod", "legacy"],
      status: "degraded",
      metrics: { cpu_percent: 88, ram_percent: 79, disk_percent: 84, network_kbps: 420 },
      last_seen_at: new Date().toISOString(),
    },
  ],
  jobs: [
    {
      id: "job-1",
      type: "package_refresh",
      target_selector: "tag:staging",
      strategy: "rolling:10,25,50,100",
      status: "approved",
      created_by: "system",
      requires_approval: false,
      summary: "Refresh package metadata on staging nodes.",
      matched_nodes: ["moscow-01"],
    },
  ],
  audit: [
    {
      id: "audit-1",
      actor: "system",
      action: "bootstrap",
      resource_type: "fleet",
      resource_id: "seed",
      message: "Initialized in-memory demo data",
      created_at: new Date().toISOString(),
    },
  ],
  apiReachable: false,
  samplePlan: {
    prompt: "обнови docker на ноде argentina-17",
    provider: "stub",
    summary: "Planned a Docker update job. Because this is a privileged package operation, review and approval are required before execution.",
    intent: "update_docker",
    requires_review: true,
    job_preview: {
      type: "update_docker",
      target_selector: "node:argentina-17",
      strategy: "rolling:10,25,50,100",
    },
  },
};

const baseUrl = process.env.MEZA_CORE_BASE_URL ?? "http://127.0.0.1:8080";
const operatorToken =
  process.env.MEZA_PANEL_OPERATOR_TOKEN ?? process.env.MEZA_OPERATOR_TOKEN ?? "";

function defaultHeaders() {
  const headers = new Headers();
  if (operatorToken) {
    headers.set("Authorization", `Bearer ${operatorToken}`);
  }
  return headers;
}

async function fetchJson<T>(path: string): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    cache: "no-store",
    headers: defaultHeaders(),
  });
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json() as Promise<T>;
}

async function postJson<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    method: "POST",
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      ...Object.fromEntries(defaultHeaders().entries()),
    },
    body: JSON.stringify(body),
  });

  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }

  return response.json() as Promise<T>;
}

export async function loadPanelData(): Promise<PanelData> {
  try {
    const [dashboard, nodes, jobs, audit, samplePlan] = await Promise.all([
      fetchJson<DashboardSummary>("/api/v1/dashboard"),
      fetchJson<ApiList<NodeItem>>("/api/v1/nodes"),
      fetchJson<ApiList<JobItem>>("/api/v1/jobs"),
      fetchJson<ApiList<AuditItem>>("/api/v1/audit"),
      postJson<PanelData["samplePlan"]>("/api/v1/ai/interpret", {
        prompt: "обнови docker на ноде argentina-17",
      }),
    ]);

    return {
      dashboard,
      nodes: nodes.items,
      jobs: jobs.items,
      audit: audit.items.slice(0, 5),
      apiReachable: true,
      samplePlan,
    };
  } catch {
    return fallbackData;
  }
}
