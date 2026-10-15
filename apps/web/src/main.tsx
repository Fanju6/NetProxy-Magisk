import React from 'react'
import ReactDOM from 'react-dom/client'
import { RouterProvider } from '@tanstack/react-router'
import { ApiClient, unavailableTransport } from '@/api/client'
import { AppProviders } from '@/app/providers'
import { makeRouter } from '@/app/router'
import './styles.css'

// 只有开发环境或显式 demo 构建启用 Mock；普通生产构建不建立真实连接。
const transport =
  import.meta.env.DEV || import.meta.env.VITE_NETPROXY_TRANSPORT === 'mock'
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
