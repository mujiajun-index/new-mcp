import axios from 'axios'

let setupChecked = false
let needsSetup = false

export async function checkSetupStatus(): Promise<boolean> {
  if (setupChecked) return needsSetup

  try {
    const res = await axios.get('/api/v1/setup', {
      params: { t: Date.now() },
      headers: { 'Cache-Control': 'no-store' },
    })
    const data = res.data
    if (data?.success && data?.data && !data.data.status) {
      needsSetup = true
    }
  } catch {
    // If the request fails, assume initialized and let the app proceed
    needsSetup = false
  }

  setupChecked = true
  return needsSetup
}

export function isSetupNeeded() {
  return needsSetup
}

export function markSetupDone() {
  needsSetup = false
  setupChecked = true
}
