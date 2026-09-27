import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ArrowLeft, Check, Loader2, Zap } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { createSystemOne, testSystemOne } from "../api";
import type { SystemOneTestResult } from "../api";
import {
  canSaveSystemOne,
  emptySystemOneForm,
  SystemOneFormFields,
  SystemOneTestPanel,
} from "./system-one-form";

export function SystemOneCreatePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [form, setForm] = useState(emptySystemOneForm);
  const [testResult, setTestResult] = useState<SystemOneTestResult | null>(
    null,
  );
  const createMutation = useMutation({
    mutationFn: () => createSystemOne(form),
    onSuccess: () => {
      toast.success(t("systemOne.createSuccess"));
      queryClient.invalidateQueries({ queryKey: ["system-one"] });
      navigate({ to: "/system-one" });
    },
    onError: () => toast.error(t("systemOne.failed")),
  });
  const testMutation = useMutation({
    mutationFn: () => testSystemOne(form),
    onSuccess: (result) => setTestResult(result.data),
    onError: () =>
      setTestResult({ success: false, error: t("systemOne.failed") }),
  });

  return (
    <div className="mx-auto max-w-2xl space-y-6 p-6 lg:p-8">
      <div className="flex items-center gap-3">
        <Button
          variant="ghost"
          size="icon"
          onClick={() => navigate({ to: "/system-one" })}
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-xl font-semibold">{t("systemOne.create")}</h1>
          <p className="text-sm text-muted-foreground">
            {t("systemOne.createSubtitle")}
          </p>
        </div>
      </div>
      <div className="space-y-5 rounded-xl border bg-card p-6">
        <SystemOneFormFields form={form} onChange={setForm} />
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <Button
          variant="outline"
          className="gap-2"
          disabled={
            testMutation.isPending ||
            !form.model_name.trim() ||
            !form.api_key.trim()
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
          disabled={createMutation.isPending || !canSaveSystemOne(form, false)}
          onClick={() => createMutation.mutate()}
        >
          {createMutation.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Check className="h-4 w-4" />
          )}
          {t("common.save")}
        </Button>
        <Button variant="ghost" onClick={() => navigate({ to: "/system-one" })}>
          {t("common.cancel")}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">{t("systemOne.testHint")}</p>
      <SystemOneTestPanel result={testResult} />
    </div>
  );
}
