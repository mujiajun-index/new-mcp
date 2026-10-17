import { useState } from "react";
import { Copy, ExternalLink, ImageOff } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { copyText } from "@/features/services/components/test-result-panel";

export interface TaskReferenceImage {
  url?: string;
  role: "reference" | "mask";
  status: "ready" | "expired";
  mime: string;
  size: number;
  width: number;
  height: number;
  expires_at: string | null;
}

function ReferenceImage({
  image,
  index,
}: {
  image: TaskReferenceImage;
  index: number;
}) {
  const { t } = useTranslation();
  const [failedURL, setFailedURL] = useState<string | undefined>();
  const label =
    image.role === "mask"
      ? t("imagegen.referenceMask")
      : t("imagegen.referenceImageLabel", { index: index + 1 });
  const available = image.status === "ready" && !!image.url;

  return (
    <Dialog>
      <DialogTrigger asChild>
        <button
          type="button"
          className="flex h-20 w-20 shrink-0 items-center justify-center overflow-hidden rounded-lg border bg-muted/30 p-1.5 transition-colors hover:border-primary/50 hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
          aria-label={label}
          title={label}
        >
          {available && failedURL !== image.url ? (
            <img
              src={image.url}
              alt={label}
              loading="lazy"
              referrerPolicy="no-referrer"
              className="max-h-full max-w-full rounded object-contain"
              onError={() => setFailedURL(image.url)}
            />
          ) : (
            <ImageOff className="h-5 w-5 text-muted-foreground" />
          )}
        </button>
      </DialogTrigger>
      <DialogContent className="space-y-4 sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{label}</DialogTitle>
          <DialogDescription>
            {image.width > 0 &&
              image.height > 0 &&
              `${image.width} × ${image.height} · `}
            {image.mime} · {(image.size / 1024 / 1024).toFixed(2)} MB
          </DialogDescription>
        </DialogHeader>
        <div className="flex min-h-40 items-center justify-center rounded-lg bg-muted/30 p-3">
          {available && failedURL !== image.url ? (
            <a
              href={image.url}
              target="_blank"
              rel="noopener noreferrer"
              className="flex w-full items-center justify-center"
              aria-label={label}
            >
              <img
                src={image.url}
                alt={label}
                loading="lazy"
                referrerPolicy="no-referrer"
                className="max-h-[50vh] max-w-full rounded object-contain"
                onError={() => setFailedURL(image.url)}
              />
            </a>
          ) : (
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <ImageOff className="h-4 w-4 shrink-0" />
              {t(
                available
                  ? "imagegen.referencePreviewFailed"
                  : "imagegen.imageExpired",
              )}
            </div>
          )}
        </div>
        <div className="space-y-2">
          {available && (
            <>
              <a
                href={image.url}
                target="_blank"
                rel="noopener noreferrer"
                className="block break-all text-xs text-primary underline-offset-4 hover:underline"
              >
                {image.url}
              </a>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-7 gap-1.5 px-2 text-xs"
                  onClick={() =>
                    copyText(image.url!)
                      .then(() => toast.success(t("common.copied")))
                      .catch(() => toast.error(t("common.copyFailed")))
                  }
                >
                  <Copy className="h-3 w-3" />
                  {t("imagegen.copyLink")}
                </Button>
                <Button
                  asChild
                  variant="ghost"
                  size="sm"
                  className="h-7 gap-1.5 px-2 text-xs"
                >
                  <a href={image.url} target="_blank" rel="noopener noreferrer">
                    <ExternalLink className="h-3 w-3" />
                    {t("imagegen.viewReferenceImage")}
                  </a>
                </Button>
              </div>
            </>
          )}
          <p className="text-xs text-muted-foreground">
            {image.status !== "ready"
              ? t("imagegen.imageExpired")
              : image.expires_at
                ? t("imagegen.imageExpires", {
                    date: new Date(image.expires_at).toLocaleString(),
                  })
                : t("imagegen.permanent")}
          </p>
        </div>
      </DialogContent>
    </Dialog>
  );
}

export function TaskReferenceImages({
  images = [],
}: {
  images?: TaskReferenceImage[];
}) {
  const { t } = useTranslation();
  return (
    <section className="space-y-2">
      <h3 className="text-sm font-medium">{t("imagegen.taskReferences")}</h3>
      {images.length ? (
        <div className="flex flex-wrap gap-2">
          {images.map((image, index) => (
            <ReferenceImage
              key={`${index}:${image.url || ""}`}
              image={image}
              index={index}
            />
          ))}
        </div>
      ) : (
        <p className="rounded-lg bg-muted/50 p-3 text-xs text-muted-foreground">
          {t("imagegen.referencesNotRecorded")}
        </p>
      )}
    </section>
  );
}
