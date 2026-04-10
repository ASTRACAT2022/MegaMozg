import { cn } from "@/lib/utils";

function Progress({ className, value = 0 }: { className?: string; value?: number }) {
  return (
    <div className={cn("bg-secondary relative h-2 w-full overflow-hidden rounded-full", className)}>
      <div className="bg-primary h-full transition-all" style={{ width: `${Math.max(0, Math.min(100, value))}%` }} />
    </div>
  );
}

export { Progress };
