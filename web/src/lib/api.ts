import { useAuthStore } from '@/stores/authStore'

const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api/v1'

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public retryAfter?: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

interface ApiFetchOptions extends RequestInit {
  skipAuth?: boolean
}

let isRefreshing = false
let refreshQueue: Array<{ resolve: (token: string) => void; reject: (err: Error) => void }> = []

function processRefreshQueue(token: string | null, error: Error | null) {
  refreshQueue.forEach(({ resolve, reject }) => {
    if (error) {
      reject(error)
    } else {
      resolve(token!)
    }
  })
  refreshQueue = []
}

export async function apiFetch(path: string, options: ApiFetchOptions = {}): Promise<Response> {
  const { skipAuth, ...fetchOptions } = options
  const headers: Record<string, string> = {
    ...(fetchOptions.headers as Record<string, string>),
  }

  // Add Content-Type for requests with body
  if (fetchOptions.body && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json'
  }

  if (!skipAuth) {
    const token = useAuthStore.getState().accessToken
    if (token) {
      headers['Authorization'] = `Bearer ${token}`
    }
  }

  const response = await fetch(`${API_BASE}${path}`, {
    ...fetchOptions,
    headers,
  })

  if (response.status === 401 && !skipAuth) {
    // Try to refresh the token
    if (!isRefreshing) {
      isRefreshing = true
      try {
        const success = await useAuthStore.getState().refresh()
        if (success) {
          const newToken = useAuthStore.getState().accessToken!
          processRefreshQueue(newToken, null)
          isRefreshing = false

          // Retry original request with new token
          headers['Authorization'] = `Bearer ${newToken}`
          return fetch(`${API_BASE}${path}`, { ...fetchOptions, headers })
        } else {
          const err = new Error('Session expired')
          processRefreshQueue(null, err)
          isRefreshing = false
          useAuthStore.getState().logout()
          throw new ApiError(401, 'Session expired')
        }
      } catch (err) {
        processRefreshQueue(null, err as Error)
        isRefreshing = false
        useAuthStore.getState().logout()
        throw new ApiError(401, 'Session expired')
      }
    } else {
      // Wait for the ongoing refresh
      return new Promise<Response>((resolve, reject) => {
        refreshQueue.push({
          resolve: (token: string) => {
            headers['Authorization'] = `Bearer ${token}`
            fetch(`${API_BASE}${path}`, { ...fetchOptions, headers }).then(resolve).catch(reject)
          },
          reject: (err: Error) => reject(err),
        })
      })
    }
  }

  if (!response.ok) {
    const body = await response.text().catch(() => response.statusText)
    const retryAfter = response.status === 429
      ? parseInt(response.headers.get('Retry-After') || '0', 10)
      : undefined
    throw new ApiError(response.status, body || response.statusText, retryAfter)
  }

  return response
}
