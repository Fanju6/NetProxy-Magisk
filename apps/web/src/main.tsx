import React from 'react'
import ReactDOM from 'react-dom/client'
import { RouterProvider } from '@tanstack/react-router'
import { ApiClient, unavailableTransport } from '@/api/client'
import { AppProviders } from '@/app/providers'
import { makeRouter } from '@/app/router'
import './styles.css'

// Native 必须显式构建并验证 Tauri 环境；普通 Web 生产包不引入 IPC 或 Mock。
const transport =
  import.meta.env.MODE === 'native'
    ? (await import('@/api/native')).createNativeTransport()
    : import.meta.env.DEV || import.meta.env.VITE_NETPROXY_TRANSPORT === 'mock'
      ? new (await import('@/mocks/transport')).MockTransport()
      : unavailableTransport
const api = new ApiClient(transport)
const router = makeRouter()
ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <AppProviders api={api}>
      <RouterProvider router={router} />
    </AppProviders>
  </React.StrictMode>,
)
