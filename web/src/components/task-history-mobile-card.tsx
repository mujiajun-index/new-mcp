import type { ReactNode } from "react";
import { Copy } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  MobileListCard,
  type MobileCardMeta,
} from "@/components/ui/mobile-list-card";
import { TaskHistoryCheckbox } from "@/components/task-history-actions";
import { copyText } from "@/features/services/components/test-result-panel";

export function TaskHistoryMobileSelectAll({
  checked,
  disabled,
  onChange,
}: {
  checked: boolean | "mixed";
  disabled: boolean;
  onChange: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2 border-b bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
      <TaskHistoryCheckbox
        checked={checked}
        disabled={disabled}
        label={t("taskHistory.selectAll")}
        onChange={onChange}
        className="h-8 w-8 shrink-0"
      />
      <span>{t("taskHistory.selectAll")}</span>
    </div>
  );
}

export function TaskHistoryMobileCard({
  taskID,
  model,
  configName,
  prompt,
  badge,
  meta,
  actions,
  selected,
  disabled,
  onToggle,
  onDetails,
}: {
  taskID: string;
  model: string;
  configName?: string;
  prompt?: string;
  badge: ReactNode;
  meta: MobileCardMeta[];
  actions: ReactNode;
  selected: boolean;
  disabled: boolean;
  onToggle: () => void;
  onDetails: () => void;
}) {
  const { t } = useTranslation();
  return (
    <MobileListCard
      className={`min-w-0 ${selected ? "bg-muted/40" : ""}`}
      title={
        <div className="flex min-w-0 items-start gap-2">
          <TaskHistoryCheckbox
            checked={selected}
            disabled={disabled}
            label={t("taskHistory.selectTask", { id: taskID })}
            onChange={onToggle}
            className="h-8 w-8 shrink-0"
          />
          <div className="min-w-0 space-y-1">
            <button
              type="button"
              onClick={onDetails}
              className="block text-left font-mono text-xs transition-colors hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <span className="line-clamp-2 break-all" title={model}>
                {model}
              </span>
            </button>
            {configName && (
              <p
                className="truncate text-xs font-normal text-muted-foreground"
                title={configName}
              >
                {configName}
              </p>
            )}
            <div className="flex min-w-0 items-center gap-1">
              <code
                className="truncate text-[10px] font-normal text-muted-foreground"
                title={taskID}
              >
                {taskID}
              </code>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-7 w-7 shrink-0"
                aria-label={t("common.copy")}
                onClick={() =>
                  copyText(taskID)
                    .then(() => toast.success(t("common.copied")))
                    .catch(() => toast.error(t("common.copyFailed")))
                }
              >
                <Copy className="h-3 w-3" />
              </Button>
            </div>
          </div>
        </div>
      }
      badge={
        <div className="flex max-w-32 flex-col items-end gap-1 [&>*]:max-w-full [&>*]:whitespace-normal [&_svg]:shrink-0">
          {badge}
        </div>
      }
      meta={meta.map((item) => ({
        ...item,
        value:
          typeof item.value === "string" ? (
            <span title={item.value}>{item.value}</span>
          ) : (
            item.value
          ),
      }))}
      note={
        prompt && (
          <p className="line-clamp-2 whitespace-pre-wrap break-words leading-relaxed text-muted-foreground [overflow-wrap:anywhere]">
            {prompt}
          </p>
        )
      }
      actions={
        <div className="flex min-w-0 flex-wrap items-center justify-end gap-1">
          {actions}
        </div>
      }
    />
  );
}
