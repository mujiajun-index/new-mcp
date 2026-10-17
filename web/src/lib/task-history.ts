export function historyPageCount(total: number, pageSize: number) {
  return Math.max(1, Math.ceil(Math.max(0, total) / pageSize));
}

// Reuse the individual authenticated endpoints and their running-task/lease
// checks. Bound concurrency so video storage deletes do not flood the server.
export async function deleteTaskHistory(
  ids: string[],
  remove: (id: string) => Promise<unknown>,
) {
  const unique = [...new Set(ids)];
  const deleted: string[] = [];
  const failed: Array<{ id: string; error: unknown }> = [];
  for (let offset = 0; offset < unique.length; offset += 4) {
    const batch = unique.slice(offset, offset + 4);
    const results = await Promise.all(
      batch.map(async (id) => {
        try {
          await remove(id);
          return { id, deleted: true as const };
        } catch (error) {
          return { id, deleted: false as const, error };
        }
      }),
    );
    results.forEach((result) => {
      if (result.deleted) deleted.push(result.id);
      else failed.push({ id: result.id, error: result.error });
    });
  }
  return { deleted, failed };
}
