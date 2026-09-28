import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

type Provider = "typesafe" | "openrouter";

interface SmartSearchConfig {
  enabled: boolean;
  provider: Provider;
  endpoint_url: string;
  model_name: string;
  has_api_key: boolean;
  all_groups: boolean;
  groups: string[];
  batch_size: number;
  concurrency: number;
}

type SmartSearchForm = SmartSearchConfig & { api_key: string };

const defaults: SmartSearchForm = {
  enabled: false,
  provider: "typesafe",
  endpoint_url: "",
  model_name: "jev-latest",
  has_api_key: false,
  api_key: "",
  all_groups: true,
  groups: [],
  batch_size: 200,
  concurrency: 4,
};

export function SmartSearchSettings({ userGroups }: { userGroups: string[] }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<SmartSearchForm>(defaults);
  const initialized = useRef(false);
  const { data, isLoading } = useQuery<SmartSearchConfig>({
    queryKey: ["admin-smart-search"],
    queryFn: async () =>
      (await api.get("/admin/settings/smart-search")).data.data,
  });
  useEffect(() => {
    if (data && !initialized.current) {
      setForm({ ...data, api_key: "" });
      initialized.current = true;
    }
  }, [data]);
  const save = useMutation({
    mutationFn: async (next: SmartSearchForm) =>
      (await api.put("/admin/settings/smart-search", next)).data,
    onSuccess: (response) => {
      toast.success(t("settings.saveSuccess"));
      setForm({ ...response.data, api_key: "" });
      queryClient.setQueryData(["admin-smart-search"], response.data);
    },
    onError: (err: any) =>
      toast.error(err?.response?.data?.message || t("settings.saveFailed")),
  });
  const toggle = useMutation({
    mutationFn: async (enabled: boolean) =>
      (await api.patch("/admin/settings/smart-search/enabled", { enabled }))
        .data,
    onSuccess: (response) => {
      setForm((current) => ({ ...current, enabled: response.data.enabled }));
      queryClient.setQueryData(["admin-smart-search"], response.data);
      toast.success(t("settings.saveSuccess"));
    },
    onError: (err: any) =>
      toast.error(err?.response?.data?.message || t("settings.saveFailed")),
  });
  const test = useMutation({
    mutationFn: async () =>
      (await api.post("/admin/settings/smart-search/test", form)).data,
    onSuccess: () => toast.success(t("settings.smartSearchTestSuccess")),
    onError: (err: any) =>
      toast.error(
        err?.response?.data?.message || t("settings.smartSearchTestFailed"),
      ),
  });
  const update = (patch: Partial<SmartSearchForm>) =>
    setForm((current) => ({ ...current, ...patch }));
  const toggleGroup = (group: string) =>
    update({
      groups: form.groups.includes(group)
        ? form.groups.filter((item) => item !== group)
        : [...form.groups, group],
    });

  if (isLoading)
    return (
      <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
    );
  return (
    <div className="max-w-2xl space-y-5 rounded-xl border bg-card p-5">
      <div>
        <h2 className="text-sm font-semibold">{t("settings.smartSearch")}</h2>
        <p className="mt-1 text-xs text-muted-foreground">
          {t("settings.smartSearchDesc")}
        </p>
      </div>
      <div className="flex items-center justify-between gap-4">
        <div>
          <p className="text-sm font-medium">
            {t("settings.smartSearchEnabled")}
          </p>
          <p className="text-xs text-muted-foreground">
            {t("settings.smartSearchEnabledDesc")}
          </p>
        </div>
        <Switch
          checked={form.enabled}
          disabled={save.isPending || toggle.isPending}
          onCheckedChange={(enabled) => {
            if (enabled && !form.has_api_key && form.api_key.trim()) {
              save.mutate({ ...form, enabled });
            } else {
              toggle.mutate(enabled);
            }
          }}
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <label className="space-y-2 text-sm font-medium">
          <span>{t("settings.smartSearchProvider")}</span>
          <Select
            value={form.provider}
            onValueChange={(value: Provider) =>
              update({
                provider: value,
                endpoint_url: "",
                model_name:
                  value === "typesafe" ? "jev-latest" : "~typesafe/jev-latest",
              })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="typesafe">TypeSafe / CLM / Laya</SelectItem>
              <SelectItem value="openrouter">OpenRouter Decisions</SelectItem>
            </SelectContent>
          </Select>
        </label>
        <label className="space-y-2 text-sm font-medium">
          <span>{t("settings.smartSearchModel")}</span>
          <Input
            value={form.model_name}
            onChange={(e) => update({ model_name: e.target.value })}
            placeholder="jev-latest"
          />
        </label>
        <label className="space-y-2 text-sm font-medium sm:col-span-2">
          <span>{t("settings.smartSearchEndpoint")}</span>
          <Input
            value={form.endpoint_url}
            onChange={(e) => update({ endpoint_url: e.target.value })}
            placeholder={
              form.provider === "typesafe"
                ? "https://api.typesafe.ai"
                : "https://openrouter.ai/api/alpha/decisions"
            }
          />
          <span className="block text-xs font-normal text-muted-foreground">
            {t("settings.smartSearchEndpointHint")}
          </span>
        </label>
        <label className="space-y-2 text-sm font-medium sm:col-span-2">
          <span>{t("settings.smartSearchKey")}</span>
          <Input
            type="password"
            autoComplete="new-password"
            value={form.api_key}
            onChange={(e) => update({ api_key: e.target.value })}
            placeholder={
              form.has_api_key ? t("settings.smartSearchKeySaved") : ""
            }
          />
          <span className="block text-xs font-normal text-muted-foreground">
            {t("settings.smartSearchKeyHint")}
          </span>
        </label>
      </div>
      <div className="space-y-3 border-t pt-4">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">
            {t("settings.smartSearchAllGroups")}
          </span>
          <Switch
            checked={form.all_groups}
            onCheckedChange={(all_groups) => update({ all_groups })}
          />
        </div>
        {!form.all_groups && (
          <div className="flex flex-wrap gap-x-4 gap-y-2">
            {userGroups.map((group) => (
              <label key={group} className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={form.groups.includes(group)}
                  onChange={() => toggleGroup(group)}
                />
                {group}
              </label>
            ))}
          </div>
        )}
        {!form.all_groups && form.groups.length === 0 && (
          <p className="text-xs text-muted-foreground">
            {t("settings.smartSearchNoGroups")}
          </p>
        )}
      </div>
      <div className="grid gap-4 border-t pt-4 sm:grid-cols-2">
        <label className="space-y-2 text-sm font-medium">
          <span>{t("settings.smartSearchBatchSize")}</span>
          <Input
            type="number"
            min={2}
            max={254}
            value={form.batch_size}
            onChange={(e) => update({ batch_size: Number(e.target.value) })}
          />
        </label>
        <label className="space-y-2 text-sm font-medium">
          <span>{t("settings.smartSearchConcurrency")}</span>
          <Input
            type="number"
            min={1}
            max={16}
            value={form.concurrency}
            onChange={(e) => update({ concurrency: Number(e.target.value) })}
          />
        </label>
        <p className="text-xs text-muted-foreground sm:col-span-2">
          {t("settings.smartSearchBatchHint")}
        </p>
      </div>
      <div className="flex gap-2 border-t pt-4">
        <Button
          onClick={() => save.mutate(form)}
          disabled={save.isPending || toggle.isPending}
        >
          {t("common.save")}
        </Button>
        <Button
          variant="outline"
          onClick={() => test.mutate()}
          disabled={test.isPending}
        >
          {t("settings.smartSearchTest")}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        {t("settings.smartSearchTestHint")}
      </p>
    </div>
  );
}
