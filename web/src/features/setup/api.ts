import axios from 'axios'

export interface SetupStatus {
  status: boolean
  admin_init: boolean
  database_type?: string
}

// Use raw axios to avoid the shared interceptor affecting setup flow
export async function getSetupStatus() {
  const res = await axios.get('/api/v1/setup', {
    params: { t: Date.now() },
    headers: { 'Cache-Control': 'no-store' },
  })
  return res.data
}

export async function submitSetup(payload: {
  username: string
  password: string
  confirm_password: string
}) {
  const res = await axios.post('/api/v1/setup', payload)
  return res.data
}
