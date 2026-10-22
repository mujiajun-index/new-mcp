import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  AudioLines,
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
import * as audio from "../api";
import { AudioTasks } from "./audio-tasks";

function EnabledBadge({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation();
  return (
    <span
      className={`inline-flex items-center whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium ${
        enabled
          ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
          : "bg-zinc-500/10 text-zinc-500"
      }`}
    >
      {t(enabled ? "audiogen.enabled" : "audiogen.disabled")}
    </span>
  );
}

function SlotModel({
  config,
  kind,
}: {
  config: audio.AudioConfig;
  kind: audio.Kind;
}) {
  const { t } = useTranslation();
  const slot = config.models.find((s) => s.kind === kind);
  if (!slot) return <span className="text-xs text-muted-foreground">—</span>;
  const connection = config.connections.find(
    (c) => c.key === slot.connection_key,
  );
  return (
    <div
      className={`min-w-32 max-w-64 space-y-1 ${slot.enabled ? "" : "opacity-50"}`}
    >
      <span className="block break-all font-mono text-xs" title={slot.model}>
        {slot.model}
      </span>
      <p className="text-xs text-muted-foreground">
        {connection && t(`audiogen.channelProtocol.${connection.protocol}`)}
        {!slot.enabled && (
          <span>
            {connection ? " · " : ""}
            {t("audiogen.disabled")}
          </span>
        )}
      </p>
    </div>
  );
}

function ConfigActions({
  config,
  mobile,
  busy,
  onToggle,
  onDelete,
}: {
  config: audio.AudioConfig;
  mobile: boolean;
  busy: boolean;
  onToggle: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const handleDelete = () => {
    if (confirm(t("audiogen.deleteConfigConfirm"))) onDelete();
  };
  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        className="gap-1"
        disabled={busy}
        onClick={onToggle}
      >
        {config.auto_register ? (
          <PowerOff className="h-3.5 w-3.5" />
        ) : (
          <Power className="h-3.5 w-3.5" />
        )}
        {t(config.auto_register ? "audiogen.disable" : "audiogen.enable")}
      </Button>
      <Link to="/audiogen/$id" params={{ id: String(config.id) }}>
        <Button variant="ghost" size="sm">
          {t("audiogen.edit")}
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
              disabled={busy}
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
          disabled={busy}
          onClick={handleDelete}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      )}
    </>
  );
}

export function AudioGenListPage() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const isMobile = useIsMobile();
  const [search, setSearch] = useState("");
  const query = useQuery({ queryKey: ["audiogen"], queryFn: audio.configs });
  const refresh = () => {
    for (const key of ["audiogen", "services", "groups"])
      qc.invalidateQueries({ queryKey: [key] });
  };
  const remove = useMutation({
    mutationFn: audio.removeConfig,
    onSuccess: () => {
      toast.success(t("audiogen.deleteSuccess"));
      refresh();
    },
    onError: (err) => toast.error(err.message),
  });
  const toggle = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      audio.toggle(id, enabled),
    onSuccess: (_, variables) => {
      toast.success(
        t(
          variables.enabled
            ? "audiogen.enableSuccess"
            : "audiogen.disableSuccess",
        ),
      );
      refresh();
    },
    onError: (err) => toast.error(err.message),
  });
  const configs = query.data || [];
  const keyword = search.trim().toLowerCase();
  const list = keyword
    ? configs.filter((c) =>
        [
          c.name,
          c.description,
          ...c.models.map((s) => s.model),
          ...c.connections.map((connection) =>
            t(`audiogen.channelProtocol.${connection.protocol}`),
          ),
        ]
          .join(" ")
          .toLowerCase()
          .includes(keyword),
      )
    : configs;
  const actions = (config: audio.AudioConfig) => (
    <ConfigActions
      config={config}
      mobile={isMobile}
      busy={toggle.isPending || remove.isPending}
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
            {t("audiogen.title")}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("audiogen.subtitle")}
          </p>
        </div>
        <Link to="/audiogen/create">
          <Button className="gap-2">
            <Plus className="h-4 w-4" />
            {t("audiogen.create")}
          </Button>
        </Link>
      </div>
      <div className="relative max-w-sm">
        <AudioLines className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          aria-label={t("audiogen.search")}
          placeholder={t("audiogen.search")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="pl-9"
        />
      </div>
      <div className="overflow-hidden rounded-xl border bg-card">
        {query.isPending ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-5 w-5 animate-spin" />
            {t("common.loading")}
          </div>
        ) : query.isError ? (
          <div className="flex flex-col items-center gap-3 py-16 text-sm">
            <p role="alert" className="text-destructive">
              {t("audiogen.loadFailed")}
            </p>
            <Button variant="outline" size="sm" onClick={() => query.refetch()}>
              {t("common.refresh")}
            </Button>
          </div>
        ) : !list.length ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <AudioLines className="mb-3 h-10 w-10 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">
              {t(
                configs.length ? "audiogen.emptyConfigs" : "audiogen.noConfigs",
              )}
            </p>
            {!configs.length && (
              <p className="mt-1 text-xs text-muted-foreground/60">
                {t("audiogen.noConfigsHint")}
              </p>
            )}
          </div>
        ) : isMobile ? (
          <div className="divide-y">
            {list.map((config) => (
              <MobileListCard
                key={config.id}
                title={
                  <Link
                    to="/audiogen/$id"
                    params={{ id: String(config.id) }}
                    className="break-words font-medium transition-colors hover:text-primary"
                  >
                    {config.name}
                  </Link>
                }
                badge={<EnabledBadge enabled={config.auto_register} />}
                meta={audio.kinds
                  .filter((kind) => config.models.some((s) => s.kind === kind))
                  .map((kind) => ({
                    label: t("audiogen.slotModel", {
                      kind: t(`audiogen.${kind}`),
                    }),
                    value: <SlotModel config={config} kind={kind} />,
                  }))}
                note={
                  config.description && (
                    <p className="line-clamp-2 break-words text-muted-foreground">
                      {config.description}
                    </p>
                  )
                }
                actions={actions(config)}
              />
            ))}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm" aria-label={t("audiogen.title")}>
              <thead>
                <tr className="border-b bg-muted/50">
                  <th
                    scope="col"
                    className="whitespace-nowrap px-4 py-3 text-left font-medium text-muted-foreground"
                  >
                    {t("audiogen.name")}
                  </th>
                  {audio.kinds.map((kind) => (
                    <th
                      key={kind}
                      scope="col"
                      className="whitespace-nowrap px-4 py-3 text-left font-medium text-muted-foreground"
                    >
                      {t("audiogen.slotModel", { kind: t(`audiogen.${kind}`) })}
                    </th>
                  ))}
                  <th
                    scope="col"
                    className="whitespace-nowrap px-4 py-3 text-left font-medium text-muted-foreground"
                  >
                    {t("common.status")}
                  </th>
                  <th
                    scope="col"
                    className="px-4 py-3 text-right font-medium text-muted-foreground"
                  >
                    {t("common.actions")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {list.map((config) => (
                  <tr
                    key={config.id}
                    className="border-b transition-colors last:border-0 hover:bg-muted/30"
                  >
                    <td className="px-4 py-3">
                      <Link
                        to="/audiogen/$id"
                        params={{ id: String(config.id) }}
                        className="block max-w-64 break-words font-medium transition-colors hover:text-primary"
                      >
                        {config.name}
                      </Link>
                      {config.description && (
                        <p
                          className="mt-1 line-clamp-2 max-w-64 break-words text-xs text-muted-foreground"
                          title={config.description}
                        >
                          {config.description}
                        </p>
                      )}
                    </td>
                    {audio.kinds.map((kind) => (
                      <td key={kind} className="px-4 py-3">
                        <SlotModel config={config} kind={kind} />
                      </td>
                    ))}
                    <td className="px-4 py-3">
                      <EnabledBadge enabled={config.auto_register} />
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1 whitespace-nowrap">
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
      <AudioTasks />
    </div>
  );
}
