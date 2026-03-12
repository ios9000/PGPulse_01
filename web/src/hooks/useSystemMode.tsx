import { useState, useEffect, createContext, useContext, type ReactNode } from 'react'

interface SystemMode {
  mode: 'live' | 'persistent'
  retention?: string
  loading: boolean
}

const SystemModeContext = createContext<SystemMode>({
  mode: 'persistent',
  loading: true,
})

export function useSystemMode(): SystemMode {
  return useContext(SystemModeContext)
}

export function SystemModeProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<SystemMode>({ mode: 'persistent', loading: true })

  useEffect(() => {
    fetch('/api/v1/system/mode')
      .then(res => res.json())
      .then(data => setState({ mode: data.mode, retention: data.retention, loading: false }))
      .catch(() => setState({ mode: 'persistent', loading: false }))
  }, [])

  return (
    <SystemModeContext.Provider value={state}>
      {children}
    </SystemModeContext.Provider>
  )
}
