export function queryNames(raw: string): string[] {
  try { return [...new Set(new URL(raw).searchParams.keys())] } catch { return [] }
}

export function queryValue(raw: string, name: string): string {
  try { return new URL(raw).searchParams.get(name) || '' } catch { return '' }
}

export function withQueryValue(raw: string, name: string, value: string): string {
  if (!name || !value) return raw
  try {
    const url = new URL(raw)
    url.searchParams.set(name, value)
    return url.toString()
  } catch { return raw }
}

export function withoutQueryValue(raw: string, name: string): string {
  if (!name) return raw
  try {
    const url = new URL(raw)
    url.searchParams.delete(name)
    return url.toString()
  } catch { return raw }
}
