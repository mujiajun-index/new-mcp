// 角色判定辅助。super_admin（超级管理员，固定为 id=1）与 admin（普通管理员）都属于管理员级别。
export function isAdminRole(role?: string): boolean {
  return role === 'super_admin' || role === 'admin'
}

export function isSuperAdminRole(role?: string): boolean {
  return role === 'super_admin'
}
