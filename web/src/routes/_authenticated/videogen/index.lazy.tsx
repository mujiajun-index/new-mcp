import { createLazyFileRoute } from "@tanstack/react-router";
import { VideoGenListPage } from "@/features/videogen/components/videogen-list-page";
export const Route = createLazyFileRoute("/_authenticated/videogen/")({
  component: VideoGenListPage,
});
