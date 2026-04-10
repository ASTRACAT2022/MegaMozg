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
  display_name: string;
  ip_address: string;
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

export type AIConfig = {
  provider: string;
  gemini_model: string;
  gemini_base_url: string;
  has_gemini_api_key: boolean;
  updated_at?: string;
};

export type PanelData = {
  dashboard: DashboardSummary;
  nodes: NodeItem[];
  jobs: JobItem[];
  audit: AuditItem[];
  aiConfig: AIConfig;
  apiReachable: boolean;
  samplePlan: AIPlanResult;
};

const fallbackData: PanelData = {
  dashboard: {
    total_nodes: 0,
    online_nodes: 0,
    degraded_nodes: 0,
    running_jobs: 0,
    awaiting_approvals: 0,
    audit_events: 0,
    average_cpu: 0,
    average_ram: 0,
    average_disk: 0,
  },
  nodes: [],
  jobs: [],
  audit: [],
  aiConfig: {
    provider: "stub",
    gemini_model: "gemini-2.5-flash",
    gemini_base_url: "https://generativelanguage.googleapis.com/v1beta",
    has_gemini_api_key: false,
  },
  apiReachable: false,
  samplePlan: {
    prompt: "",
    provider: "unavailable",
    summary: "AI временно недоступен. Проверьте подключение Meza-Core API.",
    intent: "unavailable",
    requires_review: false,
    job_preview: {
      type: "",
      target_selector: "",
      strategy: "",
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
    const [dashboard, nodes, jobs, audit, aiConfig, samplePlan] = await Promise.all([
      fetchJson<DashboardSummary>("/api/v1/dashboard"),
      fetchJson<ApiList<NodeItem>>("/api/v1/nodes"),
      fetchJson<ApiList<JobItem>>("/api/v1/jobs"),
      fetchJson<ApiList<AuditItem>>("/api/v1/audit"),
      fetchJson<AIConfig>("/api/v1/ai/config"),
      postJson<PanelData["samplePlan"]>("/api/v1/ai/interpret", {
        prompt: "обнови docker на ноде argentina-17",
      }),
    ]);

    return {
      dashboard,
      nodes: nodes.items,
      jobs: jobs.items,
      audit: audit.items.slice(0, 5),
      aiConfig,
      apiReachable: true,
      samplePlan,
    };
  } catch {
    return fallbackData;
  }
}
