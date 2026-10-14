// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createBrowserRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { Root } from './App'
import { Dashboard } from './pages/Dashboard'
import { Vehicles } from './pages/Vehicles'
import { VehicleDetailPage } from './pages/VehicleDetail'
import { VehicleEdit } from './pages/VehicleEdit'
import { Settings } from './pages/Settings'
import { Expenses } from './pages/Expenses'
import '@fontsource/barlow/latin-400.css'
import '@fontsource/barlow/latin-500.css'
import '@fontsource/barlow/latin-600.css'
import '@fontsource/barlow/latin-700.css'
import '@fontsource/barlow-semi-condensed/latin-600.css'
import '@fontsource/barlow-semi-condensed/latin-700.css'
import './styles.css'

const router = createBrowserRouter([
  {
    element: <Root />,
    children: [
      { path: '/', element: <Dashboard /> },
      { path: '/vehicles', element: <Vehicles /> },
      { path: '/vehicles/new', element: <VehicleEdit key="new" /> },
      { path: '/vehicles/:id', element: <VehicleDetailPage /> },
      { path: '/vehicles/:id/edit', element: <VehicleEdit /> },
      { path: '/expenses', element: <Expenses /> },
      { path: '/settings', element: <Settings /> },
      { path: '*', element: <Dashboard /> },
    ],
  },
])

// The service worker keeps the interface for offline use (see public/sw.js).
if ('serviceWorker' in navigator && import.meta.env.PROD) {
  navigator.serviceWorker.register('/sw.js').catch(() => {})
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
)
