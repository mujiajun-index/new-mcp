import {
  CheckSquare,
  Loader2,
  MinusSquare,
  Square,
  Trash2,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export function TaskHistoryStatusFilter({
  kind,
  value,
  disabled,
  onChange,
}: {
  kind: "image" | "video";
  value: string;
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const statuses =
    kind === "image"
      ? [
          "completed",
          "failed",
          "queued",
          "running",
          "saving",
          "partial",
          "submission_unknown",
          "canceled",
          "interrupted",
        ]
      : [
          "completed",
          "failed",
          "submitting",
          "queued",
          "in_progress",
          "unknown",
          "submission_unknown",
        ];
  return (
    <Select value={value} onValueChange={onChange} disabled={disabled}>
      <SelectTrigger
        className="h-8 w-36 text-xs [&>span]:truncate"
        aria-label={t("taskHistory.filterStatus")}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{t("taskHistory.allStatuses")}</SelectItem>
        {statuses.map((status) => (
          <SelectItem key={status} value={status}>
            {status === "completed"
              ? t("taskHistory.success")
              : t(
                  `${kind === "image" ? "imagegen.taskStatus" : "videogen.status"}.${status}`,
                )}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export function TaskHistoryCheckbox({
  checked,
  disabled,
  label,
  onChange,
}: {
  checked: boolean | "mixed";
  disabled?: boolean;
  label: string;
  onChange: () => void;
}) {
  const Icon =
    checked === "mixed" ? MinusSquare : checked ? CheckSquare : Square;
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={checked}
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={onChange}
      className="flex h-6 w-6 items-center justify-center rounded text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-40"
    >
      <Icon className={`h-4 w-4 ${checked ? "text-primary" : ""}`} />
    </button>
  );
}

export function TaskHistoryBatchActions({
  count,
  busy,
  kind,
  onDelete,
  onClear,
}: {
  count: number;
  busy: boolean;
  kind: "image" | "video";
  onDelete: () => void;
  onClear: () => void;
}) {
  const { t } = useTranslation();
  if (!count) return null;
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-xl border bg-card p-3">
      <span className="text-sm text-muted-foreground">
        {t("apiKeys.selected", { count })}
      </span>
      <Button
        type="button"
        variant="destructive"
        size="sm"
        className="gap-1.5"
        disabled={busy}
        onClick={() => {
          if (confirm(t(`taskHistory.${kind}DeleteConfirm`, { count })))
            onDelete();
        }}
      >
        {busy ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : (
          <Trash2 className="h-3.5 w-3.5" />
        )}
        {t("taskHistory.batchDelete")}
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={busy}
        onClick={onClear}
      >
        {t("apiKeys.clearSelection")}
      </Button>
    </div>
  );
}
