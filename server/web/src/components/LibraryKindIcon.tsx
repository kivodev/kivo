import { Icon, type IconName } from "./Icon";

const icons: Record<string, IconName> = { movies: "film", shows: "tv", videos: "video", audiobooks: "book" };

export function LibraryKindIcon({ kind, className }: { kind: string; className?: string }) {
  return <Icon name={icons[kind] ?? "folder"} className={className} />;
}
