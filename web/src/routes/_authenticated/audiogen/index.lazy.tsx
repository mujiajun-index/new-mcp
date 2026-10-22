import { createLazyFileRoute } from "@tanstack/react-router";
import { AudioGenListPage } from "@/features/audiogen/components/audiogen-list-page";
export const Route = createLazyFileRoute("/_authenticated/audiogen/")({
  component: AudioGenListPage,
});
