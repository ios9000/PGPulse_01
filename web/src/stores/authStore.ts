import { create } from 'zustand'
import type { Permission, Role } from '@/lib/permissions'
import { getPermissions } from '@/lib/permissions'

const REFRESH_TOKEN_KEY = 'pgpulse_refresh_token'

interface AuthUser {
  id: number
  username: string
  role: Role
  active: boolean
  permissions: Permission[]
}

interface AuthState {
  accessToken: string | null
  user: AuthUser | null
  isAuthenticated: boolean
  isLoading: boolean
  permissions: Permission[]

  login: (username: string, password: string) => Promise<void>
  logout: () => void
  refresh: () => Promise<boolean>
  initialize: () => Promise<void>
  scheduleRefresh: (expiresIn: number) => void
}

let refreshTimer: ReturnType<typeof setTimeout> | null = null

export const useAuthStore = create<AuthState>()((set, get) => ({
  accessToken: null,
  user: null,
  isAuthenticated: false,
  isLoading: true,
  permissions: [],

  login: async (username: string, password: string) => {
    const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api/v1'
    const res = await fetch(`${API_BASE}/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    })

    if (!res.ok) {
      const body = await res.json().catch(() => ({ error: { message: res.statusText } }))
      const err = new Error(body?.error?.message || 'Login failed') as Error & { status: number; retryAfter?: number }
      err.status = res.status
      if (res.status === 429) {
        err.retryAfter = parseInt(res.headers.get('Retry-After') || '0', 10)
      }
      throw err
    }

    const data = await res.json()
    const role = data.user.role as Role
    const permissions = getPermissions(role)

    localStorage.setItem(REFRESH_TOKEN_KEY, data.refresh_token)

    set({
      accessToken: data.access_token,
      user: {
        id: data.user.id,
        username: data.user.username,
        role,
        active: data.user.active,
        permissions,
      },
      isAuthenticated: true,
      isLoading: false,
      permissions,
    })

    get().scheduleRefresh(data.expires_in)
  },

  logout: () => {
    if (refreshTimer) {
      clearTimeout(refreshTimer)
      refreshTimer = null
    }
    localStorage.removeItem(REFRESH_TOKEN_KEY)
    set({
      accessToken: null,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      permissions: [],
    })
  },

  refresh: async () => {
    const refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY)
    if (!refreshToken) return false

    try {
      const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api/v1'
      const res = await fetch(`${API_BASE}/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      })

      if (!res.ok) {
        get().logout()
        return false
      }

      const data = await res.json()
      set({ accessToken: data.access_token })
      get().scheduleRefresh(data.expires_in)

      // If user is not loaded yet (page reload), fetch /auth/me
      if (!get().user) {
        const meRes = await fetch(`${API_BASE}/auth/me`, {
          headers: { Authorization: `Bearer ${data.access_token}` },
        })
        if (meRes.ok) {
          const meData = await meRes.json()
          const role = meData.role as Role
          const permissions = getPermissions(role)
          set({
            user: {
              id: meData.id,
              username: meData.username,
              role,
              active: meData.active,
              permissions,
            },
            permissions,
          })
        }
      }

      return true
    } catch {
      get().logout()
      return false
    }
  },

  initialize: async () => {
    const refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY)
    if (!refreshToken) {
      set({ isLoading: false })
      return
    }

    const success = await get().refresh()
    if (!success) {
      set({ isLoading: false })
    } else {
      set({ isAuthenticated: true, isLoading: false })
    }
  },

  scheduleRefresh: (expiresIn: number) => {
    if (refreshTimer) clearTimeout(refreshTimer)
    // Refresh 60 seconds before expiry, minimum 10 seconds
    const delay = Math.max((expiresIn - 60) * 1000, 10000)
    refreshTimer = setTimeout(() => {
      get().refresh()
    }, delay)
  },
}))

// Handle visibility change — refresh token when tab becomes visible
if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') {
      const state = useAuthStore.getState()
      if (state.isAuthenticated) {
        state.refresh()
      }
    }
  })
}
