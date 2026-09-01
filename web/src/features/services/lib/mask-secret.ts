// maskSecret 与后端 service/marketplace.go maskSecret 同口径:
// ≥10 字符保留首4尾4,其余整体遮蔽。仅用于前端本地回显(接口本就不回明文)。
export function maskSecret(v: string): string {
  if (v.length < 10) return '****'
  return `${v.slice(0, 4)}...${v.slice(-4)}`
}
