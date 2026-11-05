import { useEffect, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { ToastProvider } from './components/ToastProvider'
import { AuthProvider, useAuth } from './components/AuthProvider'
import { ThemeProvider } from './components/ThemeProvider'
import { FontScaleProvider } from './components/FontScaleProvider'
import { AppShell, type Route } from './components/AppShell'
import { TooltipProvider } from './components/ui/tooltip'
import { LoginPage } from './pages/LoginPage'
import { AccountsPage } from './pages/AccountsPage'
import { FetchPage } from './pages/FetchPage'
import { SettingsPage } from './pages/SettingsPage'

const ROUTES: Route[] = ['accounts', 'fetch', 'settings']

function routeFromHash(): Route {
  const hash = window.location.hash.replace(/^#\/?/, '')
  return (ROUTES as string[]).includes(hash) ? (hash as Route) : 'accounts'
}

function Shell() {
  const { user, loading } = useAuth()
  const [route, setRoute] = useState<Route>(routeFromHash)

  useEffect(() => {
    const onHash = () => setRoute(routeFromHash())
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  const navigate = (next: Route) => {
    window.location.hash = `#/${next}`
    setRoute(next)
  }

  if (loading) {
    return (
      <div className="grid h-screen place-items-center bg-background">
        <Loader2 className="size-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!user) return <LoginPage />

  return (
    <AppShell route={route} onNavigate={navigate}>
      {route === 'accounts' && <AccountsPage />}
      {route === 'fetch' && <FetchPage />}
      {route === 'settings' && <SettingsPage />}
    </AppShell>
  )
}

export default function App() {
  return (
    <ThemeProvider>
      <FontScaleProvider>
        <TooltipProvider>
          <ToastProvider>
            <AuthProvider>
              <Shell />
            </AuthProvider>
          </ToastProvider>
        </TooltipProvider>
      </FontScaleProvider>
    </ThemeProvider>
  )
}
