import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Binary,
  Loader2,
  MoreHorizontal,
  Plus,
  Power,
  PowerOff,
  Search,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { MobileListCard } from "@/components/ui/mobile-list-card";
import { useIsMobile } from "@/hooks/use-mobile";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  deleteSystemOne,
  disableSystemOne,
  enableSystemOne,
  listSystemOne,
} from "../api";
import type { SystemOneConfig } from "../api";

function StatusBadge({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation();
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${enabled ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400" : "bg-zinc-500/10 text-zinc-500"}`}
    >
      {t(enabled ? "systemOne.enabled" : "systemOne.disabled")}
    </span>
  );
}

export function SystemOneListPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const isMobile = useIsMobile();
  const [searchInput, setSearchInput] = useState("");
  const [keyword, setKeyword] = useState("");
  const [deletingId, setDeletingId] = useState<number | null>(null);
  const { data, isLoading } = useQuery({
    queryKey: ["system-one"],
    queryFn: listSystemOne,
  });
  const configs: SystemOneConfig[] = data?.data || [];
  const filtered = keyword.trim()
    ? configs.filter((config) =>
        [
          config.name,
          config.description,
          config.provider,
          config.model_name,
        ].some((value) =>
          value.toLowerCase().includes(keyword.trim().toLowerCase()),
        ),
      )
    : configs;
  const deletingConfig = configs.find((config) => config.id === deletingId);
  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ["system-one"] });
    queryClient.invalidateQueries({ queryKey: ["services"] });
  };
  const toggleMutation = useMutation({
    mutationFn: (config: SystemOneConfig) =>
      config.auto_register
        ? disableSystemOne(config.id)
        : enableSystemOne(config.id),
    onSuccess: (_result, config) => {
      toast.success(
        t(
          config.auto_register
            ? "systemOne.disableSuccess"
            : "systemOne.enableSuccess",
        ),
      );
      refresh();
    },
    onError: () => toast.error(t("systemOne.failed")),
  });
  const deleteMutation = useMutation({
    mutationFn: deleteSystemOne,
    onSuccess: () => {
      toast.success(t("systemOne.deleted"));
      setDeletingId(null);
      refresh();
    },
    onError: () => toast.error(t("systemOne.failed")),
  });
  const providerLabel = (config: SystemOneConfig) =>
    config.provider === "typesafe"
      ? t("systemOne.typesafe")
      : "OpenRouter Decisions";
  const toggleButton = (
    config: SystemOneConfig,
    variant: "ghost" | "outline" = "ghost",
  ) => (
    <Button
      variant={variant}
      size="sm"
      className="gap-1"
      disabled={toggleMutation.isPending}
      onClick={() => toggleMutation.mutate(config)}
    >
      {toggleMutation.isPending &&
      toggleMutation.variables?.id === config.id ? (
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
      ) : config.auto_register ? (
        <PowerOff className="h-3.5 w-3.5" />
      ) : (
        <Power className="h-3.5 w-3.5" />
      )}
      {t(config.auto_register ? "systemOne.disable" : "systemOne.enable")}
    </Button>
  );

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {t("systemOne.title")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("systemOne.subtitle")}
          </p>
        </div>
        <Link to="/system-one/create">
          <Button className="gap-2">
            <Plus className="h-4 w-4" />
            {t("systemOne.create")}
          </Button>
        </Link>
      </div>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          setKeyword(searchInput);
        }}
        className="relative max-w-sm"
      >
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          className="pl-9"
          placeholder={t("systemOne.searchPlaceholder")}
          value={searchInput}
          onChange={(event) => setSearchInput(event.target.value)}
        />
      </form>
      <div className="overflow-hidden rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-5 w-5 animate-spin" />
            {t("common.loading")}
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Binary className="mb-3 h-10 w-10 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">
              {configs.length === 0
                ? t("systemOne.empty")
                : t("systemOne.noMatches")}
            </p>
            {configs.length === 0 && (
              <p className="mt-1 text-xs text-muted-foreground/60">
                {t("systemOne.emptyHint")}
              </p>
            )}
          </div>
        ) : isMobile ? (
          <div className="divide-y">
            {filtered.map((config) => (
              <MobileListCard
                key={config.id}
                title={
                  <Link
                    to="/system-one/$id"
                    params={{ id: String(config.id) }}
                    className="font-medium transition-colors hover:text-primary"
                  >
                    {config.name}
                  </Link>
                }
                badge={<StatusBadge enabled={config.auto_register} />}
                meta={[
                  {
                    label: t("systemOne.provider"),
                    value: providerLabel(config),
                  },
                  {
                    label: t("systemOne.model"),
                    value: (
                      <span className="font-mono">{config.model_name}</span>
                    ),
                  },
                ]}
                actions={
                  <>
                    {toggleButton(config)}
                    <Link
                      to="/system-one/$id"
                      params={{ id: String(config.id) }}
                    >
                      <Button variant="ghost" size="sm">
                        {t("systemOne.detail")}
                      </Button>
                    </Link>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon" className="h-8 w-8">
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem
                          className="text-destructive focus:text-destructive"
                          onClick={() => setDeletingId(config.id)}
                        >
                          <Trash2 className="mr-2 h-4 w-4" />
                          {t("common.delete")}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </>
                }
              />
            ))}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">
                    {t("systemOne.name")}
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">
                    {t("systemOne.provider")}
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">
                    {t("systemOne.model")}
                  </th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">
                    {t("common.status")}
                  </th>
                  <th className="px-4 py-3 text-right font-medium text-muted-foreground">
                    {t("systemOne.actions")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((config) => (
                  <tr
                    key={config.id}
                    className="border-b transition-colors last:border-0 hover:bg-muted/30"
                  >
                    <td className="px-4 py-3">
                      <Link
                        to="/system-one/$id"
                        params={{ id: String(config.id) }}
                        className="font-medium transition-colors hover:text-primary"
                      >
                        {config.name}
                      </Link>
                    </td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      {providerLabel(config)}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs">
                      {config.model_name}
                    </td>
                    <td className="px-4 py-3">
                      <StatusBadge enabled={config.auto_register} />
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        {toggleButton(config)}
                        <Link
                          to="/system-one/$id"
                          params={{ id: String(config.id) }}
                        >
                          <Button variant="ghost" size="sm">
                            {t("systemOne.detail")}
                          </Button>
                        </Link>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          title={t("common.delete")}
                          onClick={() => setDeletingId(config.id)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <AlertDialog
        open={deletingId !== null}
        onOpenChange={(open) => {
          if (!open) setDeletingId(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("systemOne.deleteConfirmTitle")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("systemOne.deleteConfirm", {
                name: deletingConfig?.name || "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              disabled={deleteMutation.isPending}
              onClick={() => {
                if (deletingId !== null) deleteMutation.mutate(deletingId);
              }}
            >
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
