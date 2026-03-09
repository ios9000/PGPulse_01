import { create } from 'zustand'

export type ToastType = 'success' | 'error' | 'warning'

export interface ToastItem {
  id: number
  message: string
  type: ToastType
}

let nextId = 1

interface ToastState {
  toasts: ToastItem[]
  addToast: (message: string, type: ToastType) => void
  removeToast: (id: number) => void
}

export const useToastStore = create<ToastState>()((set) => ({
  toasts: [],

  addToast: (message: string, type: ToastType) => {
    const id = nextId++
    set((state) => ({ toasts: [...state.toasts, { id, message, type }] }))

    setTimeout(() => {
      set((state) => ({
        toasts: state.toasts.filter((t) => t.id !== id),
      }))
    }, 4000)
  },

  removeToast: (id: number) => {
    set((state) => ({
      toasts: state.toasts.filter((t) => t.id !== id),
    }))
  },
}))

export const toast = {
  success: (message: string) => useToastStore.getState().addToast(message, 'success'),
  error: (message: string) => useToastStore.getState().addToast(message, 'error'),
  warning: (message: string) => useToastStore.getState().addToast(message, 'warning'),
}
