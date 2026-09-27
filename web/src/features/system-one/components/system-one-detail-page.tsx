import { useEffect, useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  ArrowLeft,
  CheckCircle2,
  Loader2,
  Power,
  PowerOff,
  Save,
  Zap,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  disableSystemOne,
  enableSystemOne,
  getSystemOne,
  testSystemOne,
  updateSystemOne,
} from "../api";
import type { SystemOneInput, SystemOneTestResult } from "../api";
import {
  canSaveSystemOne,
  emptySystemOneForm,
  SystemOneFormFields,
  SystemOneTestPanel,
  SystemOneToolCard,
} from "./system-one-form";

export function SystemOneDetailPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { id } = useParams({ strict: false }) as { id: string };
  const configID = Number(id);
  const queryClient = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: ["system-one", id],
    queryFn: () => getSystemOne(configID),
  });
  const config = data?.data;
  const [form, setForm] = useState<SystemOneInput>(emptySystemOneForm);
  const [testResult, setTestResult] = useState<SystemOneTestResult | null>(
    null,
  );

  useEffect(() => {
    if (config)
      setForm({
        name: config.name,
        description: config.description,
        provider: config.provider,
        endpoint_url: config.endpoint_url,
        model_name: config.model_name,
        api_key: "",
      });
  }, [config]);

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ["system-one"] });
    queryClient.invalidateQueries({ queryKey: ["services"] });
  };
  const updateMutation = useMutation({
    mutationFn: () => updateSystemOne(configID, form),
    onSuccess: () => {
      toast.success(t("systemOne.updateSuccess"));
      refresh();
      navigate({ to: "/system-one" });
    },
    onError: () => toast.error(t("systemOne.failed")),
  });
  const toggleMutation = useMutation({
    mutationFn: () =>
      config.auto_register
        ? disableSystemOne(configID)
        : enableSystemOne(configID),
    onSuccess: () => {
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
  const testMutation = useMutation({
    mutationFn: () => testSystemOne({ ...form, config_id: configID }),
    onSuccess: (result) => setTestResult(result.data),
    onError: () =>
      setTestResult({ success: false, error: t("systemOne.failed") }),
  });

  if (isLoading)
    return (
      <div className="flex items-center justify-center py-20 text-muted-foreground">
        <Loader2 className="mr-2 h-5 w-5 animate-spin" />
        {t("common.loading")}
      </div>
    );
  if (!config)
    return (
      <div className="flex items-center justify-center py-20 text-muted-foreground">
        {t("systemOne.notFound")}
      </div>
    );

  return (
    <div className="space-y-6 p-6 lg:p-8">
      <div className="flex flex-col items-start gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => navigate({ to: "/system-one" })}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <h1 className="text-xl font-semibold">{config.name}</h1>
            <p className="mt-0.5 text-sm text-muted-foreground">
              {config.provider === "typesafe"
                ? t("systemOne.typesafe")
                : "OpenRouter Decisions"}{" "}
              / {config.model_name}
            </p>
          </div>
        </div>
        <Button
          variant={config.auto_register ? "outline" : "default"}
          size="sm"
          className="gap-1.5"
          disabled={toggleMutation.isPending}
          onClick={() => toggleMutation.mutate()}
        >
          {toggleMutation.isPending ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : config.auto_register ? (
            <PowerOff className="h-3.5 w-3.5" />
          ) : (
            <Power className="h-3.5 w-3.5" />
          )}
          {config.auto_register
            ? t("systemOne.disable")
            : t("systemOne.enable")}
        </Button>
      </div>
      {config.auto_register && config.registered_service_id != null && (
        <div className="flex items-center gap-2 rounded-lg border border-emerald-500/30 bg-emerald-500/5 px-4 py-3">
          <CheckCircle2 className="h-4 w-4 text-emerald-600 dark:text-emerald-400" />
          <span className="text-sm text-emerald-700 dark:text-emerald-300">
            {t("systemOne.registered", { id: config.registered_service_id })}
          </span>
        </div>
      )}
      <div className="space-y-5 rounded-xl border bg-card p-5">
        <h2 className="text-sm font-semibold">{t("systemOne.basicConfig")}</h2>
        <SystemOneFormFields
          form={form}
          onChange={setForm}
          hasSavedKey={config.has_api_key}
          detail
        />
        <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-4">
          <Button
            variant="outline"
            className="gap-2"
            disabled={
              testMutation.isPending ||
              !form.model_name.trim() ||
              (!config.has_api_key && !form.api_key.trim())
            }
            onClick={() => {
              setTestResult(null);
              testMutation.mutate();
            }}
          >
            {testMutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Zap className="h-4 w-4" />
            )}
            {t("systemOne.test")}
          </Button>
          <Button
            className="gap-2"
            disabled={
              updateMutation.isPending ||
              !canSaveSystemOne(form, config.has_api_key)
            }
            onClick={() => updateMutation.mutate()}
          >
            {updateMutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Save className="h-4 w-4" />
            )}
            {t("systemOne.save")}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          {t("systemOne.testHint")}
        </p>
        <SystemOneTestPanel result={testResult} />
      </div>
      <SystemOneToolCard configID={configID} />
    </div>
  );
}
