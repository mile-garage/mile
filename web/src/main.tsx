// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createBrowserRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { Root } from './App'
import { Dashboard } from './pages/Dashboard'
import { Vehicles } from './pages/Vehicles'
import { VehicleDetail } from './pages/VehicleDetail'
import { VehicleEdit } from './pages/VehicleEdit'
import { Settings } from './pages/Settings'
import './styles.css'

const router = createBrowserRouter([
  {
    element: <Root />,
    children: [
      { path: '/', element: <Dashboard /> },
      { path: '/vehicles', element: <Vehicles /> },
      { path: '/vehicles/new', element: <VehicleEdit key="new" /> },
      { path: '/vehicles/:id', element: <VehicleDetail /> },
      { path: '/vehicles/:id/edit', element: <VehicleEdit /> },
      { path: '/settings', element: <Settings /> },
      { path: '*', element: <Dashboard /> },
    ],
  },
])

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
)
