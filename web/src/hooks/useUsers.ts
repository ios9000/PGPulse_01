import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'

interface UserResponse {
  id: number
  username: string
  role: string
  active: boolean
  permissions: string[]
}

interface CreateUserPayload {
  username: string
  password: string
  role: string
}

interface UpdateUserPayload {
  role?: string
  active?: boolean
}

interface ChangePasswordPayload {
  currentPassword: string
  newPassword: string
}

export function useUsers() {
  return useQuery({
    queryKey: ['users'],
    queryFn: async () => {
      const res = await apiFetch('/auth/users')
      const data = await res.json()
      return data.users as UserResponse[]
    },
  })
}

export function useCreateUser() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: CreateUserPayload) => {
      const res = await apiFetch('/auth/register', {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      return res.json()
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
    },
  })
}

export function useUpdateUser() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, ...payload }: UpdateUserPayload & { id: number }) => {
      const res = await apiFetch(`/auth/users/${id}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      })
      return res.json()
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
    },
  })
}

export function useChangePassword() {
  return useMutation({
    mutationFn: async (payload: ChangePasswordPayload) => {
      const res = await apiFetch('/auth/me/password', {
        method: 'PUT',
        body: JSON.stringify({
          current_password: payload.currentPassword,
          new_password: payload.newPassword,
        }),
      })
      return res.json()
    },
  })
}
