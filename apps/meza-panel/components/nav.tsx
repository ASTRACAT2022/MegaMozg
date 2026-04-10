import { Bell, LayoutDashboard, ShieldCheck } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

const items = [
  { label: "Overview", href: "#" },
  { label: "Nodes", href: "#nodes" },
  { label: "Jobs", href: "#jobs" },
  { label: "Copilot", href: "#copilot" },
];

export function Nav() {
  return (
    <header className="sticky top-0 z-20 rounded-xl border bg-background/80 backdrop-blur">
      <div className="flex flex-col gap-4 p-4 md:flex-row md:items-center md:justify-between">
        <div className="flex items-start gap-3">
          <div className="flex size-11 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-sm">
            <LayoutDashboard className="size-5" />
          </div>
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <h1 className="text-lg font-semibold tracking-tight">MezaMozg Panel</h1>
              <Badge variant="secondary" className="gap-1">
                <ShieldCheck className="size-3.5" />
                shadcn/ui
              </Badge>
            </div>
            <p className="text-muted-foreground text-sm">
              Fleet operations, approvals, rollout visibility, and AI-assisted planning.
            </p>
          </div>
        </div>

        <div className="flex flex-col gap-3 md:items-end">
          <nav className="flex flex-wrap items-center gap-1">
            {items.map((item) => (
              <Button key={item.label} variant="ghost" size="sm" asChild>
                <a href={item.href}>{item.label}</a>
              </Button>
            ))}
          </nav>

          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm">
              <Bell className="size-4" />
              Alerts
            </Button>
            <Button size="sm">Create Job</Button>
          </div>
        </div>
      </div>
    </header>
  );
}
