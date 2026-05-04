import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthUser {
  id: number
  username: string
  email: string
  role: 'user' | 'admin'
}

interface AuthState {
  auth: {
    user: AuthUser | null
    setUser: (user: AuthUser) => void
    reset: () => void
  }
}

const initialAuth = {
  user: null,
  setUser: () => {},
  reset: () => {},
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      auth: {
        ...initialAuth,
        setUser: (user) =>
          set((state) => ({ auth: { ...state.auth, user } })),
        reset: () =>
          set((state) => ({ auth: { ...state.auth, user: null } })),
      },
    }),
    {
      name: 'newmcp-auth',
      partialize: (state) => ({ auth: { user: state.auth.user } }),
    }
  )
)
