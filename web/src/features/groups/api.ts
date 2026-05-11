import { api } from '@/lib/api'
import type {
  ListParams,
  CreateGroupReq, UpdateGroupReq, BatchToolUpdate,
} from '@/types'

export async function getGroups(params?: ListParams) {
  const res = await api.get('/groups', { params })
  return res.data
}

export async function getGroup(id: number) {
  const res = await api.get(`/groups/${id}`)
  return res.data
}

export async function createGroup(data: CreateGroupReq) {
  const res = await api.post('/groups', data)
  return res.data
}

export async function updateGroup(id: number, data: UpdateGroupReq) {
  const res = await api.put(`/groups/${id}`, data)
  return res.data
}

export async function deleteGroup(id: number) {
  const res = await api.delete(`/groups/${id}`)
  return res.data
}

export async function addGroupServices(groupId: number, serviceIds: number[]) {
  const res = await api.post(`/groups/${groupId}/services`, { service_ids: serviceIds })
  return res.data
}

export async function removeGroupService(groupId: number, serviceId: number) {
  const res = await api.delete(`/groups/${groupId}/services/${serviceId}`)
  return res.data
}

export async function getGroupTools(groupId: number) {
  const res = await api.get(`/groups/${groupId}/tools`)
  return res.data
}

export async function updateGroupTool(groupId: number, toolName: string, data: {
  service_id?: number
  enabled?: boolean
  name_override?: string
  description_override?: string
}) {
  const res = await api.put(`/groups/${groupId}/tools/${encodeURIComponent(toolName)}`, data)
  return res.data
}

export async function batchUpdateGroupTools(groupId: number, tools: BatchToolUpdate[]) {
  const res = await api.put(`/groups/${groupId}/tools/batch`, { tools })
  return res.data
}

export async function refreshGroup(groupId: number) {
  const res = await api.post(`/groups/${groupId}/refresh`)
  return res.data
}

export async function getGroupEndpoint(groupId: number) {
  const res = await api.get(`/groups/${groupId}/endpoint`)
  return res.data
}
