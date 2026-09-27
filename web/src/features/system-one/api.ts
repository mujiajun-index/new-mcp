import { api } from "@/lib/api";

export interface SystemOneConfig {
  id: number;
  name: string;
  description: string;
  provider: "typesafe" | "openrouter";
  endpoint_url: string;
  resolved_url: string;
  model_name: string;
  has_api_key: boolean;
  auto_register: boolean;
  registered_service_id: number | null;
  created_at: string;
  updated_at: string;
}

export interface SystemOneInput {
  name: string;
  description: string;
  provider: "typesafe" | "openrouter";
  endpoint_url: string;
  model_name: string;
  api_key: string;
}

export interface SystemOneTestResult {
  success: boolean;
  result?: string;
  error?: string;
}

export const listSystemOne = async () => (await api.get("/system-one")).data;
export const getSystemOne = async (id: number) =>
  (await api.get(`/system-one/${id}`)).data;
export const createSystemOne = async (data: SystemOneInput) =>
  (await api.post("/system-one", data)).data;
export const updateSystemOne = async (id: number, data: SystemOneInput) =>
  (await api.put(`/system-one/${id}`, data)).data;
export const deleteSystemOne = async (id: number) =>
  (await api.delete(`/system-one/${id}`)).data;
export const enableSystemOne = async (id: number) =>
  (await api.post(`/system-one/${id}/enable`)).data;
export const disableSystemOne = async (id: number) =>
  (await api.post(`/system-one/${id}/disable`)).data;
export const testSystemOne = async (
  data: Partial<SystemOneInput> & { config_id?: number },
) => (await api.post("/system-one/test", data)).data;
