import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthUser {
  id: number
  username: string
  role: 'user' | 'admin'
  email?: string
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
        reset: () => {
          localStorage.removeItem('newmcp-token')
          set((state) => ({ auth: { ...state.auth, user: null } }))
        },
      },
    }),
    {
      name: 'newmcp-auth',
      partialize: (state) => ({ auth: { user: state.auth.user } }),
      merge: (persistedState, currentState) => ({
        ...currentState,
        auth: {
          ...currentState.auth,
          ...((persistedState as AuthState).auth || {}),
        },
      }),
    }
  )
)
