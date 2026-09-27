import { Wrench } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { SystemOneInput, SystemOneTestResult } from "../api";

export const emptySystemOneForm: SystemOneInput = {
  name: "",
  description: "",
  provider: "typesafe",
  endpoint_url: "",
  model_name: "jev-latest",
  api_key: "",
};

export const canSaveSystemOne = (form: SystemOneInput, hasSavedKey: boolean) =>
  Boolean(
    form.name.trim() &&
    form.model_name.trim() &&
    (hasSavedKey || form.api_key.trim()),
  );

export function SystemOneFormFields({
  form,
  onChange,
  hasSavedKey = false,
  detail = false,
}: {
  form: SystemOneInput;
  onChange: (form: SystemOneInput) => void;
  hasSavedKey?: boolean;
  detail?: boolean;
}) {
  const { t } = useTranslation();
  const update = (patch: Partial<SystemOneInput>) =>
    onChange({ ...form, ...patch });
  const defaultURL =
    form.provider === "typesafe"
      ? "https://api.typesafe.ai"
      : "https://openrouter.ai/api/alpha/decisions";
  const resolvedURL =
    form.provider === "typesafe"
      ? `${(form.endpoint_url.trim() || defaultURL).replace(/\/+$/, "")}/v1/systemone`
      : form.endpoint_url.trim() || defaultURL;

  return (
    <div className="space-y-5">
      <div className={detail ? "grid gap-4 sm:grid-cols-2" : "space-y-5"}>
        <div className="space-y-2">
          <Label htmlFor="decision-name">{t("systemOne.name")} *</Label>
          <Input
            id="decision-name"
            maxLength={128}
            placeholder={t("systemOne.namePlaceholder")}
            value={form.name}
            onChange={(e) => update({ name: e.target.value })}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="decision-description">
            {t("systemOne.description")}
          </Label>
          <Input
            id="decision-description"
            placeholder={t("systemOne.descriptionPlaceholder")}
            value={form.description}
            onChange={(e) => update({ description: e.target.value })}
          />
        </div>
      </div>
      <div className={detail ? "grid gap-4 sm:grid-cols-2" : "space-y-5"}>
        <div className="space-y-2">
          <Label htmlFor="decision-provider">{t("systemOne.provider")} *</Label>
          <Select
            value={form.provider}
            onValueChange={(value: "typesafe" | "openrouter") =>
              update({
                provider: value,
                endpoint_url: "",
                model_name:
                  value === "typesafe" ? "jev-latest" : "~typesafe/jev-latest",
              })
            }
          >
            <SelectTrigger id="decision-provider">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="typesafe">
                {t("systemOne.typesafe")}
              </SelectItem>
              <SelectItem value="openrouter">OpenRouter Decisions</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {t("systemOne.providerHint")}
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="decision-model">{t("systemOne.model")} *</Label>
          <Input
            id="decision-model"
            value={form.model_name}
            onChange={(e) => update({ model_name: e.target.value })}
            placeholder={
              form.provider === "typesafe"
                ? "jev-latest / clm-latest"
                : "~typesafe/jev-latest"
            }
          />
          <p className="text-xs text-muted-foreground">
            {t("systemOne.modelHint")}
          </p>
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor="decision-endpoint">{t("systemOne.endpoint")}</Label>
        <Input
          id="decision-endpoint"
          value={form.endpoint_url}
          placeholder={defaultURL}
          onChange={(e) => update({ endpoint_url: e.target.value })}
        />
        <p className="text-xs text-muted-foreground">
          {form.provider === "typesafe"
            ? t("systemOne.baseURLHint")
            : t("systemOne.fullURLHint")}
        </p>
        <p className="break-all text-xs text-muted-foreground">
          {t("systemOne.actualURL")}:{" "}
          <code className="text-primary">{resolvedURL}</code>
        </p>
      </div>
      <div className="space-y-2">
        <Label htmlFor="decision-api-key">
          {t("systemOne.apiKey")}
          {!hasSavedKey && " *"}
        </Label>
        <Input
          id="decision-api-key"
          type="password"
          autoComplete="new-password"
          value={form.api_key}
          placeholder={hasSavedKey ? t("systemOne.keepKey") : "API key"}
          onChange={(e) => update({ api_key: e.target.value })}
        />
        <p className="text-xs text-muted-foreground">
          {t("systemOne.keyHint")}
        </p>
      </div>
    </div>
  );
}

export function SystemOneTestPanel({
  result,
}: {
  result: SystemOneTestResult | null;
}) {
  const { t } = useTranslation();
  if (!result) return null;
  let detail = result.result || result.error || "";
  if (result.result) {
    try {
      detail = JSON.stringify(JSON.parse(result.result), null, 2);
    } catch {
      /* Keep upstream text. */
    }
  }
  return (
    <div
      className={`rounded-lg border p-3 text-sm ${result.success ? "border-emerald-500/30 bg-emerald-500/5" : "border-destructive/30 bg-destructive/5"}`}
    >
      <p className="font-medium">
        {t(result.success ? "systemOne.testSuccess" : "systemOne.testFailed")}
      </p>
      <pre className="mt-2 max-h-72 overflow-auto whitespace-pre-wrap break-words text-xs">
        {detail}
      </pre>
    </div>
  );
}

const evaluateExample = JSON.stringify(
  {
    state: { message: "My payouts have failed for 3 days." },
    questions: {
      urgent: {
        type: "noul",
        instructions: "Does the customer's message convey urgency?",
      },
      team: {
        type: "choice",
        instructions: "Which team should handle this request?",
        criteria: {
          billing: "Payments and payouts",
          technical: "Bugs and outages",
          other: "No matching team",
        },
      },
      impact: {
        type: "score",
        instructions: "How severe is the impact described by the customer?",
        criteria: ["Low impact", "Moderate impact", "High impact"],
      },
    },
  },
  null,
  2,
);

export function SystemOneToolCard({ configID }: { configID: number }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4 rounded-xl border bg-card p-5">
      <div className="flex items-center gap-2">
        <Wrench className="h-4 w-4 text-muted-foreground" />
        <h2 className="text-sm font-semibold">
          {t("systemOne.registeredTools")}
        </h2>
      </div>
      <p className="text-xs text-muted-foreground">{t("systemOne.mcpHint")}</p>
      <div className="space-y-4 rounded-lg border p-4">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <span className="inline-flex rounded-md bg-primary/10 p-1.5">
              <Wrench className="h-3.5 w-3.5 text-primary" />
            </span>
            <span className="text-sm font-semibold">evaluate</span>
          </div>
          <Badge variant="secondary">{t("systemOne.builtinTool")}</Badge>
        </div>
        <p className="text-xs text-muted-foreground">
          {t("systemOne.toolDescription")}
        </p>
        <div className="space-y-1 text-xs">
          <p className="text-muted-foreground">{t("systemOne.toolName")}</p>
          <code className="break-all">systemone_{configID}__evaluate</code>
        </div>
        <div className="space-y-1 text-xs">
          <p className="text-muted-foreground">{t("systemOne.smartToolId")}</p>
          <code className="break-all">systemone_{configID}.evaluate</code>
        </div>
        <div className="space-y-2">
          <p className="text-xs font-medium">{t("systemOne.usageTitle")}</p>
          <p className="text-xs text-muted-foreground">
            {t("systemOne.usageHint")}
          </p>
          <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted/50 p-3 text-xs">
            {evaluateExample}
          </pre>
        </div>
      </div>
    </div>
  );
}
