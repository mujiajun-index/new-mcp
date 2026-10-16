import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Film,
  Loader2,
  MoreHorizontal,
  Plus,
  Power,
  PowerOff,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { MobileListCard } from "@/components/ui/mobile-list-card";
import { useIsMobile } from "@/hooks/use-mobile";
import {
  deleteVideoGenConfig,
  getVideoGenConfigs,
  toggleVideoGenConfig,
} from "../api";
import type { VideoGenConfig } from "../api";
import { VideoTasks } from "./video-tasks";

function EnabledBadge({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation();
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium whitespace-nowrap ${
        enabled
          ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
          : "bg-zinc-500/10 text-zinc-500"
      }`}
    >
      {t(enabled ? "videogen.enabled" : "videogen.disabled")}
    </span>
  );
}

function ConfigActions({
  config,
  mobile,
  togglePending,
  deletePending,
  onToggle,
  onDelete,
}: {
  config: VideoGenConfig;
  mobile: boolean;
  togglePending: boolean;
  deletePending: boolean;
  onToggle: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const handleDelete = () => {
    if (confirm(t("videogen.deleteConfigConfirm", { name: config.name }))) {
      onDelete();
    }
  };
  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        className="gap-1"
        disabled={togglePending || deletePending}
        onClick={onToggle}
      >
        {config.auto_register ? (
          <PowerOff className="h-3.5 w-3.5" />
        ) : (
          <Power className="h-3.5 w-3.5" />
        )}
        {t(config.auto_register ? "videogen.disable" : "videogen.enable")}
      </Button>
      <Link to="/videogen/$id" params={{ id: String(config.id) }}>
        <Button variant="ghost" size="sm">
          {t("videogen.detail")}
        </Button>
      </Link>
      {mobile ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              aria-label={t("common.more")}
            >
              <MoreHorizontal className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              className="text-destructive focus:text-destructive"
              disabled={togglePending || deletePending}
              onClick={handleDelete}
            >
              <Trash2 className="mr-2 h-4 w-4" />
              {t("common.delete")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ) : (
        <Button
          variant="ghost"
          size="sm"
          className="text-destructive hover:text-destructive"
          aria-label={t("common.delete")}
          title={t("common.delete")}
          disabled={togglePending || deletePending}
          onClick={handleDelete}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      )}
    </>
  );
}

export function VideoGenListPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const isMobile = useIsMobile();
  const [search, setSearch] = useState("");
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["videogen"],
    queryFn: getVideoGenConfigs,
  });
  const refresh = () => {
    for (const key of ["videogen", "services", "groups"])
      queryClient.invalidateQueries({ queryKey: [key] });
  };
  const remove = useMutation({
    mutationFn: deleteVideoGenConfig,
    onSuccess: () => {
      toast.success(t("videogen.deleteSuccess"));
      refresh();
    },
    onError: (err) => toast.error(err.message),
  });
  const toggle = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      toggleVideoGenConfig(id, enabled),
    onSuccess: (_, variables) => {
      toast.success(
        t(
          variables.enabled
            ? "videogen.enableSuccess"
            : "videogen.disableSuccess",
        ),
      );
      refresh();
    },
    onError: (err) => toast.error(err.message),
  });
  const configs = data?.data ?? [];
  const keyword = search.trim().toLowerCase();
  const filtered = keyword
    ? configs.filter((config) =>
        `${config.name} ${config.model_name} ${config.protocol}`
          .toLowerCase()
          .includes(keyword),
      )
    : configs;

  const actions = (config: VideoGenConfig) => (
    <ConfigActions
      config={config}
      mobile={isMobile}
      togglePending={toggle.isPending}
      deletePending={remove.isPending}
      onToggle={() =>
        toggle.mutate({ id: config.id, enabled: !config.auto_register })
      }
      onDelete={() => remove.mutate(config.id)}
    />
  );

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {t("videogen.title")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("videogen.subtitle")}
          </p>
        </div>
        <Link to="/videogen/create">
          <Button className="gap-2">
            <Plus className="h-4 w-4" />
            {t("videogen.create")}
          </Button>
        </Link>
      </div>

      <div className="relative max-w-sm">
        <Film className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          aria-label={t("videogen.search")}
          placeholder={t("videogen.search")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="pl-9"
        />
      </div>

      <div className="overflow-hidden rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-5 w-5 animate-spin" />
            {t("common.loading")}
          </div>
        ) : isError ? (
          <div className="flex flex-col items-center gap-3 py-16 text-sm">
            <p className="text-destructive">{t("videogen.loadFailed")}</p>
            <Button variant="outline" size="sm" onClick={() => refetch()}>
              {t("common.refresh")}
            </Button>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Film className="mb-3 h-10 w-10 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">
              {t(
                configs.length ? "videogen.noSearchResults" : "videogen.empty",
              )}
            </p>
            {!configs.length && (
              <p className="mt-1 text-xs text-muted-foreground/60">
                {t("videogen.emptyHint")}
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
                    to="/videogen/$id"
                    params={{ id: String(config.id) }}
                    className="break-words font-medium transition-colors hover:text-primary"
                  >
                    {config.name}
                  </Link>
                }
                badge={<EnabledBadge enabled={config.auto_register} />}
                meta={[
                  {
                    label: t("videogen.protocol"),
                    value: t(
                      config.protocol === "openrouter_video"
                        ? "videogen.openRouterProtocolLabel"
                        : "videogen.protocolLabel",
                    ),
                  },
                  {
                    label: t("videogen.model"),
                    value: (
                      <span className="font-mono">{config.model_name}</span>
                    ),
                  },
                  {
                    label: t("videogen.capabilities"),
                    value: t(`videogen.operation.${config.operation}`),
                  },
                  ...(config.operation === "generate"
                    ? []
                    : [
                        {
                          label: t("videogen.imageFormat"),
                          value:
                            config.protocol === "openrouter_video"
                              ? "JSON"
                              : config.image_request_format.toUpperCase(),
                        },
                      ]),
                ]}
                actions={actions(config)}
              />
            ))}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm" aria-label={t("videogen.title")}>
              <thead>
                <tr className="border-b bg-muted/50">
                  {[
                    "videogen.name",
                    "videogen.protocol",
                    "videogen.model",
                    "videogen.capabilities",
                    "common.status",
                  ].map((label) => (
                    <th
                      key={label}
                      scope="col"
                      className="whitespace-nowrap px-4 py-3 text-left font-medium text-muted-foreground"
                    >
                      {t(label)}
                    </th>
                  ))}
                  <th
                    scope="col"
                    className="px-4 py-3 text-right font-medium text-muted-foreground"
                  >
                    {t("common.actions")}
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
                        to="/videogen/$id"
                        params={{ id: String(config.id) }}
                        className="block max-w-64 break-words font-medium transition-colors hover:text-primary"
                      >
                        {config.name}
                      </Link>
                    </td>
                    <td className="px-4 py-3">
                      <span className="text-xs text-muted-foreground">
                        {t(
                          config.protocol === "openrouter_video"
                            ? "videogen.openRouterProtocolLabel"
                            : "videogen.protocolLabel",
                        )}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className="block max-w-64 break-all font-mono text-xs">
                        {config.model_name}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className="whitespace-nowrap text-xs">
                        {t(`videogen.operation.${config.operation}`)}
                      </span>
                      {config.operation !== "generate" && (
                        <p
                          className="mt-1 text-xs text-muted-foreground"
                          title={t("videogen.imageFormat")}
                        >
                          {config.protocol === "openrouter_video"
                            ? "JSON"
                            : config.image_request_format.toUpperCase()}
                        </p>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <EnabledBadge enabled={config.auto_register} />
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        {actions(config)}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <VideoTasks />
    </div>
  );
}
